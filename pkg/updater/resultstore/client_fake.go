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

package resultstore

import (
	"context"
	"errors"

	rspb "google.golang.org/genproto/googleapis/devtools/resultstore/v2"
	"google.golang.org/grpc"
)

type fakePage struct {
	nextPageToken string
	invocationIDs []string
}

// fakeResultStoreClient returns fake pages of results, based on init values.
type fakeResultStoreClient struct {
	invocations map[string]fakePage
}

func (c fakeResultStoreClient) SearchInvocations(_ context.Context, req *rspb.SearchInvocationsRequest, _ ...grpc.CallOption) (*rspb.SearchInvocationsResponse, error) {
	page, ok := c.invocations[req.GetPageToken()]
	if !ok {
		return nil, errors.New("results not found")
	}
	var invocs []*rspb.Invocation
	for _, invocID := range page.invocationIDs {
		invocs = append(invocs, &rspb.Invocation{Id: &rspb.Invocation_Id{InvocationId: invocID}})
	}
	resp := &rspb.SearchInvocationsResponse{
		NextPageToken: page.nextPageToken,
		Invocations:   invocs,
	}
	return resp, nil
}

func (c fakeResultStoreClient) SearchConfiguredTargets(_ context.Context, req *rspb.SearchConfiguredTargetsRequest, _ ...grpc.CallOption) (*rspb.SearchConfiguredTargetsResponse, error) {
	page, ok := c.invocations[req.GetPageToken()]
	if !ok {
		return nil, errors.New("results not found")
	}
	var targets []*rspb.ConfiguredTarget
	for _, invocID := range page.invocationIDs {
		targets = append(targets, &rspb.ConfiguredTarget{
			Id: &rspb.ConfiguredTarget_Id{
				InvocationId: invocID,
				TargetId:     "fake-target",
			},
		})
	}
	resp := &rspb.SearchConfiguredTargetsResponse{
		NextPageToken:     page.nextPageToken,
		ConfiguredTargets: targets,
	}
	return resp, nil
}

func (c fakeResultStoreClient) ExportInvocation(_ context.Context, req *rspb.ExportInvocationRequest, _ ...grpc.CallOption) (*rspb.ExportInvocationResponse, error) {
	return nil, errors.New("ExportInvocation() not implemented")
}
