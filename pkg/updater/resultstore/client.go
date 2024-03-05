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

// Package resultstore fetches and process results from ResultStore.
package resultstore

import (
	"context"
	"crypto/x509"
	"fmt"
	"strings"

	"github.com/sirupsen/logrus"
	"google.golang.org/genproto/googleapis/devtools/resultstore/v2"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/oauth"
	"google.golang.org/grpc/metadata"
)

type resultStoreClient interface {
	SearchInvocations(context.Context, *resultstore.SearchInvocationsRequest, ...grpc.CallOption) (*resultstore.SearchInvocationsResponse, error)
	ExportInvocation(context.Context, *resultstore.ExportInvocationRequest, ...grpc.CallOption) (*resultstore.ExportInvocationResponse, error)
}

// DownloadClient provides a client to download ResultStore results from.
type DownloadClient struct {
	client resultStoreClient
	token  string
}

// NewClient uses the specified gRPC connection to connect to ResultStore.
func NewClient(conn *grpc.ClientConn) *DownloadClient {
	return &DownloadClient{
		client: resultstore.NewResultStoreDownloadClient(conn),
	}
}

// Connect returns a secure gRPC connection.
//
// Authenticates as the service account if specified otherwise the default user.
func Connect(ctx context.Context, serviceAccountPath string) (*grpc.ClientConn, error) {
	pool, err := x509.SystemCertPool()
	if err != nil {
		return nil, fmt.Errorf("system cert pool: %v", err)
	}
	creds := credentials.NewClientTLSFromCert(pool, "")
	const scope = "https://www.googleapis.com/auth/cloud-platform"
	var perRPC credentials.PerRPCCredentials
	if serviceAccountPath != "" {
		perRPC, err = oauth.NewServiceAccountFromFile(serviceAccountPath, scope)
	} else {
		perRPC, err = oauth.NewApplicationDefault(ctx, scope)
	}
	if err != nil {
		return nil, fmt.Errorf("create oauth: %v", err)
	}
	conn, err := grpc.Dial(
		"resultstore.googleapis.com:443",
		grpc.WithTransportCredentials(creds),
		grpc.WithPerRPCCredentials(perRPC),
	)
	if err != nil {
		return nil, fmt.Errorf("dial: %v", err)
	}

	return conn, nil
}

// Search finds all the invocations that satisfies the query condition within a project.
func (c *DownloadClient) Search(ctx context.Context, log logrus.FieldLogger, query, projectID string, fields ...string) ([]string, error) {
	var ids []string
	nextPageToken := ""
	fieldMaskCtx := fieldMask(
		ctx,
		"next_page_token",
		"invocations.id",
	)
	for {
		req := &resultstore.SearchInvocationsRequest{
			Query:     query,
			ProjectId: projectID,
			PageStart: &resultstore.SearchInvocationsRequest_PageToken{
				PageToken: nextPageToken,
			},
		}
		resp, err := c.client.SearchInvocations(fieldMaskCtx, req)
		if err != nil {
			return nil, err
		}
		for _, inv := range resp.GetInvocations() {
			ids = append(ids, inv.Id.GetInvocationId())
		}
		if resp.GetNextPageToken() == "" {
			break
		}
		nextPageToken = resp.GetNextPageToken()
	}
	return ids, nil
}

// FetchResult provides a interface to store Resultstore invocation data.
type FetchResult struct {
	Invocation        *resultstore.Invocation
	Actions           []*resultstore.Action
	ConfiguredTargets []*resultstore.ConfiguredTarget
	Targets           []*resultstore.Target
}

// fieldMask is required by gRPC for GET methods.
func fieldMask(ctx context.Context, fields ...string) context.Context {
	return metadata.AppendToOutgoingContext(ctx, "X-Goog-FieldMask", strings.Join(fields, ","))
}

// FetchInvocation returns all details for a given invocation.
func (c *DownloadClient) FetchInvocation(ctx context.Context, log logrus.FieldLogger, invocationID string) (*FetchResult, error) {
	name := fmt.Sprintf("invocations/%s", invocationID)
	nextPageToken := ""
	result := &FetchResult{}
	fieldMaskCtx := fieldMask(
		ctx,
		"next_page_token",
		"invocation.id",
		"invocation.timing",
		"invocation.status_attributes",
		"invocation.properties",
		"invocation.invocation_attributes",
		"targets.id",
		"targets.timing",
		"targets.status_attributes",
		"targets.properties",
		"actions.id",
		"actions.timing",
		"actions.properties",
		"actions.status_attributes",
		"actions.test_action",
		"configured_targets.id",
		"configured_targets.status_attributes",
		"configured_targets.test_attributes",
		"configured_targets.timing",
	)
	for {
		req := &resultstore.ExportInvocationRequest{
			Name: name,
			PageStart: &resultstore.ExportInvocationRequest_PageToken{
				PageToken: nextPageToken,
			},
		}
		resp, err := c.client.ExportInvocation(fieldMaskCtx, req)
		if err != nil {
			return nil, err
		}
		if result.Invocation == nil {
			result.Invocation = resp.GetInvocation()
		}
		result.Actions = append(result.Actions, resp.GetActions()...)
		result.ConfiguredTargets = append(result.ConfiguredTargets, resp.GetConfiguredTargets()...)
		result.Targets = append(result.Targets, resp.GetTargets()...)
		if resp.GetNextPageToken() == "" {
			break
		}
		nextPageToken = resp.GetNextPageToken()
	}
	return result, nil
}
