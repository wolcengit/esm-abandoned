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
	"net/http"
	"github.com/parnurzeal/gorequest"
)

func Get(url string,auth *Auth) (*http.Response, string, []error) {
	request := gorequest.New()
	if(auth!=nil){
		request.SetBasicAuth(auth.User,auth.Pass)
	}

	resp, body, errs := request.Get(url).End()
	return resp, body, errs

}

func Post(url string,auth *Auth, body string)(*http.Response, string, []error)  {
	request := gorequest.New()
	if(auth!=nil){
		request.SetBasicAuth(auth.User,auth.Pass)
	}
	return request.Post(url).Send(body).End()
}


