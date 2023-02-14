/*
Copyright 2021 The TestGrid Authors.

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

package v1

import (
	"net/http"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

// writeJSON will write obj to w as JSON, or will write the JSON marshalling error
// Includes headers that are universal to all API responses
func (s Server) writeJSON(w http.ResponseWriter, msg proto.Message) {
	resp, err := protojson.Marshal(msg)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if s.AccessControlAllowOrigin != "" {
		w.Header().Set("Access-Control-Allow-Origin", s.AccessControlAllowOrigin)
		if s.AccessControlAllowOrigin != "*" {
			w.Header().Set("Vary", "Origin")
		}
	}
	w.Write(resp)
}
