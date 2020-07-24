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
	"bytes"
	"errors"
	"io"
	"reflect"
	"testing"

	"cloud.google.com/go/storage"
)

func TestParseSuitesMeta(t *testing.T) {
	cases := []struct {
		name      string
		input     string
		context   string
		timestamp string
		thread    string
		empty     bool
	}{

		{
			name:  "not junit",
			input: "./started.json",
			empty: true,
		},
		{
			name:  "forgot suffix",
			input: "./junit",
			empty: true,
		},
		{
			name:  "basic",
			input: "./junit.xml",
		},
		{
			name:    "context",
			input:   "./junit_hello world isn't-this exciting!.xml",
			context: "hello world isn't-this exciting!",
		},
		{
			name:    "numeric context",
			input:   "./junit_12345.xml",
			context: "12345",
		},
		{
			name:    "context and thread",
			input:   "./junit_context_12345.xml",
			context: "context",
			thread:  "12345",
		},
		{
			name:      "context and timestamp",
			input:     "./junit_context_20180102-1234.xml",
			context:   "context",
			timestamp: "20180102-1234",
		},
		{
			name:      "context thread timestamp",
			input:     "./junit_context_20180102-1234_5555.xml",
			context:   "context",
			timestamp: "20180102-1234",
			thread:    "5555",
		},
	}

	for _, tc := range cases {
		actual := parseSuitesMeta(tc.input)
		switch {
		case actual == nil && !tc.empty:
			t.Errorf("%s: unexpected nil map", tc.name)
		case actual != nil && tc.empty:
			t.Errorf("%s: should not have returned a map: %v", tc.name, actual)
		case actual != nil:
			for k, expected := range map[string]string{
				"Context":   tc.context,
				"Thread":    tc.thread,
				"Timestamp": tc.timestamp,
			} {
				if a, ok := actual[k]; !ok {
					t.Errorf("%s: missing key %s", tc.name, k)
				} else if a != expected {
					t.Errorf("%s: %s actual %s != expected %s", tc.name, k, a, expected)
				}
			}
		}
	}

}

type fakeReader struct {
	buf      *bytes.Buffer
	readErr  error
	closeErr error
}

func (fr *fakeReader) Read(p []byte) (int, error) {
	if fr.readErr != nil {
		return 0, fr.readErr
	}
	return fr.buf.Read(p)
}

func (fr *fakeReader) Close() error {
	if fr.closeErr != nil {
		return fr.closeErr
	}
	fr.readErr = errors.New("already closed")
	fr.closeErr = fr.readErr
	return nil
}

func TestReadJSON(t *testing.T) {
	cases := []struct {
		name     string
		reader   fakeReader
		openErr  error
		actual   interface{}
		expected interface{}
		is       error
	}{
		{
			name:     "basically works",
			reader:   fakeReader{buf: bytes.NewBufferString("{}")},
			actual:   &Started{},
			expected: &Started{},
		},
		{
			name:   "read a json object",
			reader: fakeReader{buf: bytes.NewBufferString("{\"hello\": 5}")},
			actual: &struct {
				Hello int `json:"hello"`
			}{},
			expected: &struct {
				Hello int `json:"hello"`
			}{5},
		},
		{
			name:    "ErrObjectNotExist on open returns an ErrObjectNotExist error",
			openErr: storage.ErrObjectNotExist,
			is:      storage.ErrObjectNotExist,
		},
		{
			name:    "other open errors also error",
			openErr: errors.New("injected open error"),
		},
		{
			name: "read error errors",
			reader: fakeReader{
				buf:     bytes.NewBufferString("{}"),
				readErr: errors.New("injected read error"),
			},
		},
		{
			name: "close error errors",
			reader: fakeReader{
				buf:      bytes.NewBufferString("{}"),
				closeErr: errors.New("injected close error"),
			},
		},
		{
			name: "invalid json errors",
			reader: fakeReader{
				buf:      bytes.NewBufferString("{\"json\": \"hates trailing commas\",}"),
				closeErr: errors.New("injected close error"),
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fakeOpen := func() (io.ReadCloser, error) {
				if tc.openErr != nil {
					return nil, tc.openErr
				}
				return &tc.reader, nil
			}
			err := readJSON(fakeOpen, tc.actual)
			switch {
			case err != nil:
				if tc.expected != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if tc.is != nil && !errors.Is(err, tc.is) {
					t.Errorf("bad error: %v, wanted %v", err, tc.is)
				}
			case tc.expected == nil:
				t.Error("failed to receive expected error")
			default:
				if !reflect.DeepEqual(tc.actual, tc.expected) {
					t.Errorf("got %v, want %v", tc.actual, tc.expected)
				}
			}
		})
	}
}

// TODO(fejta): TestStarted
// TODO(fejta): TestFinished
// TODO(fejta): TestSuites
