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

import "bytes"

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
	return s.ESAPIV0.NewScroll(indexNames,scrollTime,docBufferCount)
}
func (s *ESAPIV5) NextScroll(scrollTime string,scrollId string)(*Scroll,error)  {
	return s.ESAPIV0.NextScroll(scrollId,scrollId)
}

