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
	"encoding/json"
	"fmt"
	log "github.com/cihub/seelog"
	"bytes"
	"strings"
	"regexp"
	"net/http"
	"io/ioutil"
	"errors"
)

type ESAPIV0 struct{
	Host string //eg: http://localhost:9200
	Auth *Auth //eg: user:pass
}

func (s *ESAPIV0) ClusterHealth() *ClusterHealth {

	url := fmt.Sprintf("%s/_cluster/health", s.Host)
	_, body, errs := Get(url,s.Auth)

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

func (s *ESAPIV0) Bulk(data *bytes.Buffer){
	if data == nil || data.Len() == 0 {
		return
	}
	data.WriteRune('\n')
	url:=fmt.Sprintf("%s/_bulk", s.Host)

	client := &http.Client{}
	reqest, _ := http.NewRequest("POST", url, data)
	if(s.Auth!=nil){
		reqest.SetBasicAuth(s.Auth.User,s.Auth.Pass)
	}
	resp,errs := client.Do(reqest)
	if errs != nil {
		log.Error(errs)
		return
	}

	body,err:=ioutil.ReadAll(resp.Body)
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

	url:=fmt.Sprintf("%s/%s/_settings", s.Host,indexNames)
	resp,body, errs := Get(url,s.Auth)
	if errs != nil {
		return nil,errs[0]
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil,errors.New(body)
	}

	log.Debug(body)

	err := json.Unmarshal([]byte(body),allSettings)
	if(err!=nil){
		return nil,err
	}

	//for name, index := range *idxs {
	//	//TODO 验证 analyzer等setting是否生效
	//	if settings, ok := allSettings[name]; !ok {
	//		return log.Errorf("couldnt find index %s", name)
	//	} else {
	//		// omg XXX
	//		index.(map[string]interface{})["settings"] = map[string]interface{}{}
	//		var shards string
	//		if _, ok := settings.(map[string]interface{})["settings"].(map[string]interface{})["index"]; ok {
	//			// try the new style syntax first, which has an index object
	//			shards = settings.(map[string]interface{})["settings"].(map[string]interface{})["index"].(map[string]interface{})["number_of_shards"].(string)
	//		} else {
	//			// if not, could be running from old es, try the old style index.number_of_shards
	//			shards = settings.(map[string]interface{})["settings"].(map[string]interface{})["index.number_of_shards"].(string)
	//		}
	//		index.(map[string]interface{})["settings"].(map[string]interface{})["index"] = map[string]interface{}{
	//			"number_of_shards": shards,
	//		}
	//	}
	//}

	return allSettings,nil
}

func (s *ESAPIV0) GetIndexMappings(copyAllIndexes bool,indexNames string)(string,int,*Indexes,error){
	url:=fmt.Sprintf("%s/%s/_mapping", s.Host, indexNames)
	resp,body, errs := Get(url,s.Auth)
	if errs != nil {
		log.Error(errs)
		return "",0,nil,errs[0]
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return "",0,nil,errors.New(body)
	}

	idxs := Indexes{}
	er := json.Unmarshal([]byte(body),&idxs)

	if er != nil {
		log.Error(body)
		return "",0,nil,er
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

	i:=0
	// wrap in mappings if moving from super old es
	for name, idx := range idxs {
		i++
		if _, ok := idx.(map[string]interface{})["mappings"]; !ok {
			(idxs)[name] = map[string]interface{}{
				"mappings": idx,
			}
		}
	}

	return indexNames,i,&idxs,nil
}



func (s *ESAPIV0) UpdateIndexSettings(){}


// CreateIndexes on remodeleted ES instance
func (s *ESAPIV0) CreateIndexes(idxs *Indexes) (err error) {

	for name, idx := range *idxs {
		body := bytes.Buffer{}
		enc := json.NewEncoder(&body)
		enc.Encode(idx)

		log.Debug("start create index: ",name)


		url:=fmt.Sprintf("%s/%s", s.Host, name)
		client := &http.Client{}
		reqest, _ := http.NewRequest("POST", url, &body)
		if(s.Auth!=nil){
			reqest.SetBasicAuth(s.Auth.User,s.Auth.Pass)
		}
		resp,errs := client.Do(reqest)
		if errs != nil {
			log.Error(errs)
			return errs
		}

		if resp.StatusCode != 200 {
			b, _ := ioutil.ReadAll(resp.Body)
			return errors.New("failed creating index: "+string(b))
		}

		respBody,err:=ioutil.ReadAll(resp.Body)
		if err != nil {
			log.Error(err)
			return err
		}

		log.Debug(name,string(respBody))

		if err != nil {
			return err
		}
		defer resp.Body.Close()
		log.Info("created index: ", name)
	}

	return nil
}

func (s *ESAPIV0) NewScroll(indexNames string,scrollTime string,docBufferCount int)(scroll *Scroll, err error){

	// curl -XGET 'http://es-0.9:9200/_search?search_type=scan&scroll=10m&size=50'
	url := fmt.Sprintf("%s/%s/_search?search_type=scan&scroll=%s&size=%d", s.Host, indexNames, scrollTime, docBufferCount)
	resp,body, errs := Get(url,s.Auth)
	if err != nil {
		log.Error(errs)
		return nil,errs[0]
	}
	defer resp.Body.Close()

	log.Trace("new scroll,",body)

	if err != nil {
		log.Error(err)
		return nil,err
	}


	if resp.StatusCode != 200 {
		return nil,errors.New(body)
	}

	scroll = &Scroll{}
	err = json.Unmarshal([]byte(body),scroll)
	if err != nil {
		log.Error(err)
		return nil,err
	}

	return scroll,err
}

func (s *ESAPIV0) NextScroll(scrollTime string,scrollId string)(*Scroll,error)  {
	//  curl -XGET 'http://es-0.9:9200/_search/scroll?scroll=5m'
	id := bytes.NewBufferString(scrollId)
	url:=fmt.Sprintf("%s/_search/scroll?scroll=%s&scroll_id=%s", s.Host, scrollTime, id)
	resp,body, errs := Get(url,s.Auth)
	if errs != nil {
		log.Error(errs)
		return nil,errs[0]
	}

	if resp.StatusCode != 200 {
		return nil,errors.New(body)
	}

	defer resp.Body.Close()

	// decode elasticsearch scroll response
	scroll := &Scroll{}
	err := json.Unmarshal([]byte(body), &scroll)
	if err != nil {
		log.Error(body)
		log.Error(err)
		return nil,err
	}

	return scroll,nil
}
