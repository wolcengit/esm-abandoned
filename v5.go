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
	"fmt"
	"errors"
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

func (s *ESAPIV5) GetIndexSettings(indexNames string) (*Indexes,error){
	return s.ESAPIV0.GetIndexSettings(indexNames)
}
func (s *ESAPIV5) GetIndexMappings(copyAllIndexes bool,indexNames string)(string,int,*Indexes,error){
	return s.ESAPIV0.GetIndexMappings(copyAllIndexes,indexNames)
}

func (s *ESAPIV5) UpdateIndexSettings(){}

func (s *ESAPIV5) CreateIndexes(idxs *Indexes) (err error) {
	return s.ESAPIV0.CreateIndexes(idxs)
}

func (s *ESAPIV5) NewScroll(indexNames string,scrollTime string,docBufferCount int)(scroll *Scroll, err error){
	url := fmt.Sprintf("%s/%s/_search?scroll=%s&size=%d", s.Host, indexNames, scrollTime,docBufferCount)
	resp,body, errs := Get(url,s.Auth)
	if errs != nil {
		log.Error(errs)
		return nil,errs[0]
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil,errors.New(body)
	}

	log.Trace("new scroll,",body)

	if err != nil {
		log.Error(err)
		return nil,err
	}

	scroll = &Scroll{}
	err = json.Unmarshal([]byte(body),scroll)
	if err != nil {
		log.Error(err)
		return nil,err
	}

	return scroll,err
}

func (s *ESAPIV5) NextScroll(scrollTime string,scrollId string)(*Scroll,error)  {
	id := bytes.NewBufferString(scrollId)

	url:=fmt.Sprintf("%s/_search/scroll?scroll=%s&scroll_id=%s", s.Host, scrollTime, id)
	resp,body, errs := Get(url,s.Auth)
	if errs != nil {
		log.Error(errs)
		return nil,errs[0]
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil,errors.New(body)
	}

	// decode elasticsearch scroll response
	scroll := &Scroll{}
	err:= json.Unmarshal([]byte(body), &scroll)
	if err != nil {
		log.Error(body)
		log.Error(err)
		return nil,err
	}

	return scroll,nil
}

