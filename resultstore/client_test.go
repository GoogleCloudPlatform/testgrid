/*
Copyright 2020 The Testgrid Authors.

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

package resultstore

import (
	"testing"
	"time"

	resultstore "google.golang.org/genproto/googleapis/devtools/resultstore/v2"
)

func TestConvertToInvocations(t *testing.T) {
	// Resultstore stores time using the protobuf type. It differs from the
	// Golang's Time and Duration type and requires conversion.
	now := stamp(time.Now())
	duration := dur(time.Second)
	cases := []struct {
		name     string
		response *resultstore.SearchInvocationsResponse
		expected []*Invocation
	}{
		{
			name:     "empty response",
			response: &resultstore.SearchInvocationsResponse{},
			expected: []*Invocation{},
		},
		{
			name: "single response",
			response: &resultstore.SearchInvocationsResponse{
				Invocations: []*resultstore.Invocation{
					{
						Name: "invocations/fakeid-12345",
						Id: &resultstore.Invocation_Id{
							InvocationId: "fakeid-12345",
						},
						StatusAttributes: &resultstore.StatusAttributes{
							Status: resultstore.Status_PASSED,
						},
						Timing: &resultstore.Timing{
							StartTime: now,
							Duration:  duration,
						},
					},
				},
			},
			expected: []*Invocation{
				{
					Name:     "invocations/fakeid-12345",
					Duration: protoDurationToGoDuration(duration),
					Start:    protoTimeToGoTime(now),
					Status:   resultstore.Status_PASSED,
				},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := convertToInvocations(tc.response)
			if !deepEqual(got, tc.expected) {
				t.Errorf(diff(got, tc.expected))
			}
		})
	}
}
