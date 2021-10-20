/*
Copyright 2018 The Kubernetes Authors.

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
	"fmt"
	"net/http"
	"net/url"
	"reflect"
	"testing"

	statepb "github.com/GoogleCloudPlatform/testgrid/pb/state"
	"github.com/golang/protobuf/proto"
	"google.golang.org/api/googleapi"
)

func TestIsPreconditionFailed(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "pass",
		},
		{
			name: "normal",
			err:  errors.New("normal"),
		},
		{
			name: "googleapi",
			err: &googleapi.Error{
				Code: 404,
			},
		},
		{
			name: "precondition",
			err: &googleapi.Error{
				Code: http.StatusPreconditionFailed,
			},
			want: true,
		},
		{
			name: "wrapped precondition",
			err: fmt.Errorf("wrap: %w", &googleapi.Error{
				Code: http.StatusPreconditionFailed,
			}),
			want: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsPreconditionFailed(tc.err); got != tc.want {
				t.Errorf("isPreconditionFailed(%v) got %t, want %t", tc.err, got, tc.want)
			}
		})
	}
}

func Test_SetURL(t *testing.T) {
	cases := []struct {
		name   string
		url    string
		err    bool
		bucket string
		object string
	}{
		{
			name:   "only bucket",
			url:    "gs://thisbucket",
			bucket: "thisbucket",
		},
		{
			name:   "bucket and object",
			url:    "gs://first/second",
			bucket: "first",
			object: "second",
		},
		{
			name:   "allow files",
			url:    "/path/to/my/bucket/foo",
			bucket: "",
			object: "path/to/my/bucket/foo",
		},
		{
			name:   "allow file urls",
			url:    "file://path/to/my/bucket/foo",
			bucket: "path",
			object: "to/my/bucket/foo",
		},
		{
			name: "reject unknown scheme",
			url:  "foo://some/path",
			err:  true,
		},
		{
			name: "reject websites",
			url:  "http://example.com/object",
			err:  true,
		},
		{
			name: "reject ports",
			url:  "gs://first:123/second",
			err:  true,
		},
		{
			name: "reject username",
			url:  "gs://erick@first/second",
			err:  true,
		},
		{
			name: "reject queries",
			url:  "gs://first/second?query=true",
			err:  true,
		},
		{
			name: "reject fragments",
			url:  "gs://first/second#fragment",
			err:  true,
		},
	}
	for _, tc := range cases {
		var p Path
		err := p.Set(tc.url)
		switch {
		case err != nil && !tc.err:
			t.Errorf("%s: unexpected error: %v", tc.name, err)
		case err == nil && tc.err:
			t.Errorf("%s: failed to raise an error", tc.name)
		default:
			if p.Bucket() != tc.bucket {
				t.Errorf("%s: bad bucket %s != %s", tc.name, p.Bucket(), tc.bucket)
			}
			if p.Object() != tc.object {
				t.Errorf("%s: bad object %s != %s", tc.name, p.Object(), tc.object)
			}
		}
	}
}

func Test_ResolveReference(t *testing.T) {
	var p Path
	err := p.Set("gs://bucket/path/to/config")
	if err != nil {
		t.Fatalf("bad path: %v", err)
	}
	u, err := url.Parse("testgroup")
	if err != nil {
		t.Fatalf("bad url: %v", err)
	}
	q, err := p.ResolveReference(u)
	if q.Object() != "path/to/testgroup" {
		t.Errorf("bad object: %s", q)
	}
	if q.Bucket() != "bucket" {
		t.Errorf("bad bucket: %s", q)
	}
}

// Ensure that a == b => calcCRC(a) == calcCRC(b)
func Test_calcCRC(t *testing.T) {
	b1 := []byte("hello")
	b2 := []byte("world")
	b1a := []byte("h")
	b1a = append(b1a, []byte("ello")...)
	c1 := calcCRC(b1)
	c2 := calcCRC(b2)
	c1a := calcCRC(b1a)

	switch {
	case c1 == c2:
		t.Errorf("g1 crc %d should not equal g2 crc %d", c1, c2)
	case len(b1) == 0, len(b2) == 0:
		t.Errorf("empty b1 b2 %s %s", b1, b2)
	case len(b1) != len(b1a), c1 != c1a:
		t.Errorf("different results: %s %d != %s %d", b1, c1, b1a, c1a)
	}

}

func TestMarshalGrid(t *testing.T) {
	g1 := statepb.Grid{
		Columns: []*statepb.Column{
			{Build: "alpha"},
			{Build: "second"},
		},
	}
	g2 := statepb.Grid{
		Columns: []*statepb.Column{
			{Build: "first"},
			{Build: "second"},
		},
	}

	b1, e1 := MarshalGrid(&g1)
	b2, e2 := MarshalGrid(&g2)
	uncompressed, e1a := proto.Marshal(&g1)

	switch {
	case e1 != nil, e2 != nil:
		t.Errorf("unexpected error %v %v %v", e1, e2, e1a)
	}

	if reflect.DeepEqual(b1, b2) {
		t.Errorf("unexpected equality %v == %v", b1, b2)
	}

	if reflect.DeepEqual(b1, uncompressed) {
		t.Errorf("should be compressed but is not: %v", b1)
	}
}
