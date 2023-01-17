/*
Copyright 2023 The TestGrid Authors.

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

// Package api provides code to host an API displaying TestGrid data
package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHealth(t *testing.T) {
	tests := []struct {
		name         string
		path         string
		expectedCode int
	}{
		{
			name:         "Returns a healthiness check at '/'",
			path:         "/",
			expectedCode: http.StatusOK,
		},
		{
			name:         "Return 404 to nonsense",
			path:         "/ipa/v1/derp",
			expectedCode: http.StatusNotFound,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			router, _, err := GetRouters(RouterOptions{}, nil)
			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}
			request, err := http.NewRequest("GET", test.path, nil)
			if err != nil {
				t.Fatalf("Can't form request: %v", err)
			}
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if response.Code != test.expectedCode {
				t.Errorf("Expected %d, but got %d", test.expectedCode, response.Code)
			}
		})
	}
}
