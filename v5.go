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
	log "github.com/cihub/seelog"
	"encoding/json"
	"io/ioutil"
	"fmt"
	"net/http"
)

type ESAPIV5 struct{
	ESAPIV0
}


func (s *ESAPIV5) ClusterHealth() *ClusterHealth {
	return s.ESAPIV0.ClusterHealth()
}

func (s *ESAPIV5) Bulk(data *bytes.Buffer){
	s.ESAPIV0.Bulk(data)
}
func (s *ESAPIV5) GetIndexSettings(copyAllIndexes bool,indexNames string)(string,*Indexes,error){
	return s.ESAPIV0.GetIndexSettings(copyAllIndexes,indexNames)
}
func (s *ESAPIV5) UpdateIndexSettings(){}
func (s *ESAPIV5) NewScroll(indexNames string,scrollTime string,docBufferCount int)(scroll *Scroll, err error){
	url := fmt.Sprintf("%s/%s/_search?scroll=%s&size=%d", s.Host, indexNames, scrollTime,docBufferCount)
	resp, err := http.Get(url)
	if err != nil {
		log.Error(err)
		return nil,err
	}
	defer resp.Body.Close()

	body,err:=ioutil.ReadAll(resp.Body)

	log.Debug("new scroll,",string(body))

	if err != nil {
		log.Error(err)
		return nil,err
	}

	scroll = &Scroll{}
	err = json.Unmarshal(body,scroll)
	if err != nil {
		log.Error(err)
		return nil,err
	}

	return scroll,err
}
func (s *ESAPIV5) NextScroll(scrollTime string,scrollId string)(*Scroll,error)  {
	id := bytes.NewBufferString(scrollId)

	req, err := http.NewRequest("GET", fmt.Sprintf("%s/_search/scroll?scroll=%s&scroll_id=%s", s.Host, scrollTime, id), nil)
	if err != nil {
		log.Error(err)
		return nil,err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Error(err)
		return nil,err
	}
	defer resp.Body.Close()

	data, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Error(err)
		return nil,err
	}

	// decode elasticsearch scroll response
	scroll := &Scroll{}
	err = json.Unmarshal(data, &scroll)
	if err != nil {
		log.Error(string(data))
		log.Error(err)
		return nil,err
	}

	return scroll,nil
}

