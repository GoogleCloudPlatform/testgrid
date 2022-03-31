/*
Copyright 2022 The TestGrid Authors.

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

package gcs

import (
	"errors"
	"net/http"
	"testing"

	"cloud.google.com/go/storage"
	"github.com/google/go-cmp/cmp"
	"google.golang.org/api/googleapi"
)

func TestWrapGoogleAPIError(t *testing.T) {
	create := func(code int) error {
		var e googleapi.Error
		e.Code = code
		return &e
	}
	cases := []struct {
		name string
		err  error
		want string
		is   error
	}{
		{
			name: "nil",
		},
		{
			name: "basic",
			err:  errors.New("hi"),
			want: errors.New("hi").Error(),
		},
		{
			name: "random code",
			err:  create(http.StatusInternalServerError),
			want: create(http.StatusInternalServerError).Error(),
		},
		{
			name: "404",
			err:  create(http.StatusNotFound),
			want: create(http.StatusNotFound).Error(),
			is:   storage.ErrObjectNotExist,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gotErr := wrapGoogleAPIError(tc.err)
			var got string
			if gotErr != nil {
				got = gotErr.Error()
			}
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Errorf("wrapGoogleAPIError() got unexpected diff (-want +got):\n%s", diff)
			}
			if tc.is != nil && !errors.Is(gotErr, tc.is) {
				t.Errorf("errors.Is(%v, %v) returned false", gotErr, tc.is)
			}
		})
	}
}
