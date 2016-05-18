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

import
(
	"encoding/json"
	log "github.com/cihub/seelog"
	"testing"
)



func TestParse(test *testing.T){
	setInitLogging("debug")

	text:= `{ "_scroll_id": "c2NhbjswOzE7dG90YWxfaGl0czoxODY1MjY5Ow==", "took": 1, "timed_out": false, "_shards": { "total": 1, "successful": 0, "failed": 1, "failures": [ { "shard": -1, "index": null } ] }, "hits": { "total": 1865269, "max_score": 0, "hits": [] } }`
	scroll := Scroll{}
	err:=json.Unmarshal([]byte(text),&scroll)
	if err != nil {
		log.Error(err)
		return
	}
	log.Info(scroll.ScrollId)
}
