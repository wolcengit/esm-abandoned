/*
Copyright 2016 Medcl (m AT medcl.net)

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

   http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	log "github.com/cihub/seelog"
	"io/ioutil"
	"net/http"
	"regexp"
	"strings"
)

type ESAPIV0 struct {
	Host      string //eg: http://localhost:9200
	Auth      *Auth  //eg: user:pass
	HttpProxy string //eg: http://proxyIp:proxyPort
}

func (s *ESAPIV0) ClusterHealth() *ClusterHealth {

	url := fmt.Sprintf("%s/_cluster/health", s.Host)
	_, body, errs := Get(url, s.Auth)

	if errs != nil {
		return &ClusterHealth{Name: s.Host, Status: "unreachable"}
	}

	log.Debug(body)

	health := &ClusterHealth{}
	err := json.Unmarshal([]byte(body), health)

	if err != nil {
		log.Error(body)
		return &ClusterHealth{Name: s.Host, Status: "unreachable"}
	}
	return health
}

func (s *ESAPIV0) Bulk(data *bytes.Buffer) {
	if data == nil || data.Len() == 0 {
		return
	}
	data.WriteRune('\n')
	url := fmt.Sprintf("%s/_bulk", s.Host)

	client := &http.Client{}
	reqest, _ := http.NewRequest("POST", url, data)
	if s.Auth != nil {
		reqest.SetBasicAuth(s.Auth.User, s.Auth.Pass)
	}
	resp, errs := client.Do(reqest)
	if errs != nil {
		log.Error(errs)
		return
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Error(err)
		return
	}

	defer resp.Body.Close()
	defer data.Reset()
	if resp.StatusCode != 200 {
		log.Errorf("bad bulk response: %s %s", body, resp.StatusCode)
		return
	}
}

func (s *ESAPIV0) GetIndexSettings(indexNames string) (*Indexes, error) {

	// get all settings
	allSettings := &Indexes{}

	url := fmt.Sprintf("%s/%s/_settings", s.Host, indexNames)
	resp, body, errs := Get(url, s.Auth)
	if errs != nil {
		return nil, errs[0]
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, errors.New(body)
	}

	log.Debug(body)

	err := json.Unmarshal([]byte(body), allSettings)
	if err != nil {
		return nil, err
	}

	return allSettings, nil
}

func (s *ESAPIV0) GetIndexMappings(copyAllIndexes bool, indexNames string) (string, int, *Indexes, error) {
	url := fmt.Sprintf("%s/%s/_mapping", s.Host, indexNames)
	resp, body, errs := Get(url, s.Auth)
	if errs != nil {
		log.Error(errs)
		return "", 0, nil, errs[0]
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return "", 0, nil, errors.New(body)
	}

	idxs := Indexes{}
	er := json.Unmarshal([]byte(body), &idxs)

	if er != nil {
		log.Error(body)
		return "", 0, nil, er
	}

	// remove indexes that start with . if user asked for it
	if copyAllIndexes == false {
		for name := range idxs {
			switch name[0] {
			case '.':
				delete(idxs, name)
			case '_':
				delete(idxs, name)

			}
		}
	}

	// if _all indexes limit the list of indexes to only these that we kept
	// after looking at mappings
	if indexNames == "_all" {

		var newIndexes []string
		for name := range idxs {
			newIndexes = append(newIndexes, name)
		}
		indexNames = strings.Join(newIndexes, ",")

	} else if strings.Contains(indexNames, "*") || strings.Contains(indexNames, "?") {

		r, _ := regexp.Compile(indexNames)

		//check index patterns
		var newIndexes []string
		for name := range idxs {
			matched := r.MatchString(name)
			if matched {
				newIndexes = append(newIndexes, name)
			}
		}
		indexNames = strings.Join(newIndexes, ",")

	}

	i := 0
	// wrap in mappings if moving from super old es
	for name, idx := range idxs {
		i++
		if _, ok := idx.(map[string]interface{})["mappings"]; !ok {
			(idxs)[name] = map[string]interface{}{
				"mappings": idx,
			}
		}
	}

	return indexNames, i, &idxs, nil
}

func getIndexEmptySettings() map[string]interface{} {
	tempIndexSettings := map[string]interface{}{}
	tempIndexSettings["settings"] = map[string]interface{}{}
	tempIndexSettings["settings"].(map[string]interface{})["index"] = map[string]interface{}{}
	return tempIndexSettings
}

func cleanSettings(settings map[string]interface{}) {
	//clean up settings
	delete(settings["settings"].(map[string]interface{})["index"].(map[string]interface{}), "creation_date")
	delete(settings["settings"].(map[string]interface{})["index"].(map[string]interface{}), "uuid")
	delete(settings["settings"].(map[string]interface{})["index"].(map[string]interface{}), "version")
}

func (s *ESAPIV0) UpdateIndexSettings(name string, settings map[string]interface{}) error {

	log.Debug("start update index: ", name, settings)

	url := fmt.Sprintf("%s/%s/_settings", s.Host, name)

	if _, ok := settings["settings"].(map[string]interface{})["index"]; ok {
		if set, ok := settings["settings"].(map[string]interface{})["index"].(map[string]interface{})["analysis"]; ok {
			log.Debug("update static index settings: ", name)
			staticIndexSettings := getIndexEmptySettings()
			staticIndexSettings["settings"].(map[string]interface{})["index"].(map[string]interface{})["analysis"] = set
			Post(fmt.Sprintf("%s/%s/_close", s.Host, name), s.Auth, "")
			body := bytes.Buffer{}
			enc := json.NewEncoder(&body)
			enc.Encode(staticIndexSettings)
			bodyStr, err := Request("PUT", url, s.Auth, &body)
			if err != nil {
				log.Error(bodyStr, err)
				return err
			}
			delete(settings["settings"].(map[string]interface{})["index"].(map[string]interface{}), "analysis")
			Post(fmt.Sprintf("%s/%s/_open", s.Host, name), s.Auth, "")
		}
	}

	log.Debug("update dynamic index settings: ", name)

	body := bytes.Buffer{}
	enc := json.NewEncoder(&body)
	enc.Encode(settings)
	_, err := Request("PUT", url, s.Auth, &body)

	return err
}

func (s *ESAPIV0) UpdateIndexMapping(indexName string, settings map[string]interface{}) error {

	log.Debug("start update mapping: ", indexName,settings)

	for name, mapping := range settings {

		log.Debug("start update mapping: ", indexName,name,mapping)

		url := fmt.Sprintf("%s/%s/%s/_mapping", s.Host, indexName, name)

		body := bytes.Buffer{}
		enc := json.NewEncoder(&body)
		enc.Encode(mapping)
		res, err := Request("POST", url, s.Auth, &body)
		if(err!=nil){
			log.Error(err,res)
		}
	}
	return nil
}

func (s *ESAPIV0) DeleteIndex(name string) (err error) {

	log.Debug("start delete index: ", name)

	url := fmt.Sprintf("%s/%s", s.Host, name)

	Request("DELETE", url, s.Auth, nil)

	log.Debug("delete index: ", name)

	return nil
}

func (s *ESAPIV0) CreateIndex(name string, settings map[string]interface{}) (err error) {
	cleanSettings(settings)

	body := bytes.Buffer{}
	enc := json.NewEncoder(&body)
	enc.Encode(settings)
	log.Debug("start create index: ", name, settings)

	url := fmt.Sprintf("%s/%s", s.Host, name)

	resp, err := Request("POST", url, s.Auth, &body)
	log.Debug(resp)

	return err
}

func (s *ESAPIV0) NewScroll(indexNames string, scrollTime string, docBufferCount int) (scroll *Scroll, err error) {

	// curl -XGET 'http://es-0.9:9200/_search?search_type=scan&scroll=10m&size=50'
	url := fmt.Sprintf("%s/%s/_search?search_type=scan&scroll=%s&size=%d", s.Host, indexNames, scrollTime, docBufferCount)
	resp, body, errs := Get(url, s.Auth)
	if err != nil {
		log.Error(errs)
		return nil, errs[0]
	}
	defer resp.Body.Close()

	log.Trace("new scroll,", body)

	if err != nil {
		log.Error(err)
		return nil, err
	}

	if resp.StatusCode != 200 {
		return nil, errors.New(body)
	}

	scroll = &Scroll{}
	err = json.Unmarshal([]byte(body), scroll)
	if err != nil {
		log.Error(err)
		return nil, err
	}

	return scroll, err
}

func (s *ESAPIV0) NextScroll(scrollTime string, scrollId string) (*Scroll, error) {
	//  curl -XGET 'http://es-0.9:9200/_search/scroll?scroll=5m'
	id := bytes.NewBufferString(scrollId)
	url := fmt.Sprintf("%s/_search/scroll?scroll=%s&scroll_id=%s", s.Host, scrollTime, id)
	resp, body, errs := Get(url, s.Auth)
	if errs != nil {
		log.Error(errs)
		return nil, errs[0]
	}

	if resp.StatusCode != 200 {
		return nil, errors.New(body)
	}

	defer resp.Body.Close()

	// decode elasticsearch scroll response
	scroll := &Scroll{}
	err := json.Unmarshal([]byte(body), &scroll)
	if err != nil {
		log.Error(body)
		log.Error(err)
		return nil, err
	}

	return scroll, nil
}


