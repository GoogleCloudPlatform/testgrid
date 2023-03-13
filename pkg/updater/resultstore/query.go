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

// Package resultstore reads results from ResultStore.
package resultstore

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/sirupsen/logrus"
	rspb "google.golang.org/genproto/googleapis/devtools/resultstore/v2"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

// InvocationClient searches for invocations from ResultStore.
type InvocationClient interface {
	SearchInvocations(context.Context, *rspb.SearchInvocationsRequest, ...grpc.CallOption) (*rspb.SearchInvocationsResponse, error)
	SearchConfiguredTargets(context.Context, *rspb.SearchConfiguredTargetsRequest, ...grpc.CallOption) (*rspb.SearchConfiguredTargetsResponse, error)
	ExportInvocation(context.Context, *rspb.ExportInvocationRequest, ...grpc.CallOption) (*rspb.ExportInvocationResponse, error)
}

// ConvertReason includes reasons a conversion might fail.
type ConvertReason int64

const (
	// Used to handle parentheses in regex matching
	startParen = "@STARTPAREN@"
	endParen   = "@ENDPAREN@"
	// Unknown why conversion failed.
	Unknown ConvertReason = iota
	// BeforeAfter is not allowed (`before:<date>` or `after:<date>`).
	BeforeAfter
	// Bare tokens are not allowed.
	Bare
)

// QueryError is an error for query conversion.
type QueryError struct {
	Reason ConvertReason
	Err    error
}

func (qe *QueryError) Error() string {
	return qe.Err.Error()
}

func (qe *QueryError) String() string {
	switch r := qe.Reason; r {
	case BeforeAfter:
		return "Before/After"
	case Bare:
		return "Bare"
	default:
		return "Unknown"
	}
}

func isTargetToken(token string) bool {
	parts := strings.SplitN(token, ":", 2)
	key := strings.TrimSpace(parts[0])

	if k := strings.ToLower(key); k == "target" || k == "exact_target" {
		return true
	}
	return false
}

func complexToken(simpleToken string, isTarget bool) (string, error) {
	logic := map[string]string{
		"and": "AND",
		"or":  "OR",
		"not": "NOT",
	}
	parts := strings.SplitN(simpleToken, ":", 2)
	if len(parts) == 0 {
		return "", nil
	}
	key := strings.TrimSpace(parts[0])
	if key == "" {
		return "", nil
	}
	var negative bool // true if the token is negative, e.g. "-label:<label>"
	if strings.HasPrefix(key, "-") {
		negative = true
		key = key[1:]
	}
	if len(parts) == 1 {
		// Could be a logical connector...
		if token, ok := logic[strings.ToLower(key)]; ok {
			return token, nil
		}
		// ...or a parenthesis...
		if key == "(" || key == ")" {
			return key, nil
		}
		// ...or a bare token, which is disallowed.
		return "", &QueryError{
			Reason: Bare,
			Err:    fmt.Errorf("bare tokens are not allowed: got %s", key),
		}
	}

	// Don't allow 'before:', 'after:'...
	if key == "before" || key == "after" {
		return "", &QueryError{
			Reason: BeforeAfter,
			Err:    errors.New("'before:', 'after:' are not allowed in TestGrid queries"),
		}
	}
	// Ignore 'status' tokens, they aren't useful for TestGrid queries.
	if key == "status" {
		return "", nil
	}

	var token string
	k := strings.ToLower(key)
	switch {
	case k == "user" && isTarget:
		token = "invocation.invocation_attributes.users:\"%s\""
	case k == "user":
		token = "invocation_attributes.users:\"%s\""
	case k == "label" && isTarget:
		token = "invocation.invocation_attributes.labels:\"%s\""
	case k == "label":
		token = "invocation_attributes.labels:\"%s\""
	case k == "target" || k == "exact_target":
		token = "id.target_id=\"%s\""
	default:
		// Assume this is a property, ex. "<property_name>:<value>".
		token = `propertyEquals(` + key + `, "%s")`
		if isTarget {
			token = `(invocationPropertyEquals(` + key + `, "%[1]s") OR configurationPropertyEquals(` + key + `, "%[1]s"))`
		}
	}

	value := strings.TrimSpace(parts[1])
	value = strings.Trim(value, `"`)
	if value == "" {
		return "", nil
	}
	if negative {
		token = "NOT " + token
	}
	return fmt.Sprintf(token, value), nil
}

var tokenRegex = regexp.MustCompile(`(?i)(?P<token>(?P<key>\S+):(?P<value>".*?"|'.*?'|\S+))|(?P<logic>or|and|not)|(?P<bare>".*?"|'.*?'|\S+)`)

// ComplexQuery attempts to translate a given simple query into a valid ResultStore query.
func ComplexQuery(simpleQuery string) (string, error) {
	simpleQuery = strings.ReplaceAll(simpleQuery, "'", "\"")
	// Temporarily replace parentheses with a special token, then re-substitute them later.
	simpleQuery = strings.ReplaceAll(simpleQuery, "(", " "+startParen+" ")
	simpleQuery = strings.ReplaceAll(simpleQuery, ")", " "+endParen+" ")

	var simpleTokens, complexTokens []string
	matches := tokenRegex.FindAllStringSubmatch(simpleQuery, -1)
	for _, match := range matches {
		// Should be exactly 6 entries; full match, token, key, value, logic, bare.
		if len(match) != 6 {
			return "", fmt.Errorf("wrong number of submatches on match %v; want 6, got %d", match, len(match))
		}
		if match[0] == "" {
			continue
		}
		token := match[0]
		// Bare tokens are not allowed, auto-transform to a label unless it's a parenthesis.
		switch match[5] {
		case startParen:
			token = "("
		case endParen:
			token = ")"
		case "":
			// do nothing
		default:
			// Automatically transform a bare token to a label. (Keep negation if present).
			bare := match[5]
			var negated bool
			if strings.HasPrefix(bare, "-") {
				negated = true
				bare = bare[1:]
			}
			token = fmt.Sprintf("label:%s", bare)
			if negated {
				token = fmt.Sprintf("-%s", token)
			}
		}
		simpleTokens = append(simpleTokens, token)
	}

	// First, check if this is a target search.
	var searchTargets bool
	for _, simpleToken := range simpleTokens {
		// Reverse the parentheses conversion if needed (e.g. if a value contained quoted parentheses).
		simpleToken = strings.ReplaceAll(simpleToken, " "+startParen+" ", "(")
		simpleToken = strings.ReplaceAll(simpleToken, " "+endParen+" ", ")")
		if isTargetToken(simpleToken) {
			searchTargets = true
			break
		}
	}

	// Then translate the query.
	for _, simpleToken := range simpleTokens {
		if simpleToken == "" {
			continue
		}
		// Reverse the parentheses conversion if needed (e.g. if a value contained quoted parentheses).
		simpleToken = strings.ReplaceAll(simpleToken, " "+startParen+" ", "(")
		simpleToken = strings.ReplaceAll(simpleToken, " "+endParen+" ", ")")
		token, err := complexToken(simpleToken, searchTargets)
		if err != nil {
			return "", err
		}
		if token == "" {
			continue
		}
		complexTokens = append(complexTokens, token)
	}
	return strings.Join(complexTokens, " "), nil
}

// fieldMaskCtx returns a context with the specified field masks for gRPC calls.
func fieldMaskCtx(ctx context.Context, fields []string) context.Context {
	return metadata.AppendToOutgoingContext(ctx, "X-Goog-FieldMask", strings.Join(fields, ","))
}

func targetSearch(ctx context.Context, query string, client InvocationClient, projectID string, pageSize int, pageToken string) ([]string, string, error) {
	req := &rspb.SearchConfiguredTargetsRequest{
		Query:     query,
		PageSize:  int32(pageSize),
		ProjectId: projectID,
		Parent:    "invocations/-/targets/-",
	}
	if pageSize < 0 {
		return nil, "", errors.New("pageSize must be at least 0")
	}
	if pageToken != "" {
		req.PageStart = &rspb.SearchConfiguredTargetsRequest_PageToken{PageToken: pageToken}
	}
	ctx = fieldMaskCtx(ctx, []string{
		"next_page_token",
		"configured_targets.id",
	})
	response, err := client.SearchConfiguredTargets(ctx, req)
	var ids []string
	if err != nil {
		return ids, "", err
	}
	for _, target := range response.GetConfiguredTargets() {
		ids = append(ids, target.GetId().GetInvocationId())
	}
	return ids, response.GetNextPageToken(), nil
}

func invocationSearch(ctx context.Context, query string, client InvocationClient, projectID string, pageSize int, pageToken string) ([]string, bool, string, error) {
	req := &rspb.SearchInvocationsRequest{
		Query:     query,
		PageSize:  int32(pageSize),
		ProjectId: projectID,
	}
	if pageSize < 0 {
		return nil, false, "", errors.New("pageSize must be at least 0")
	}
	if pageToken != "" {
		req.PageStart = &rspb.SearchInvocationsRequest_PageToken{PageToken: pageToken}
	}
	logrus.WithField("query", query).WithField("req", req).Debug("Searching ResultStore")
	ctx = fieldMaskCtx(ctx, []string{
		"next_page_token",
		"invocations.id",
		"invocations.invocation_attributes.labels",
	})
	response, err := client.SearchInvocations(ctx, req)
	var ids []string
	if err != nil {
		return ids, false, "", err
	}
	var overview bool
	for _, inv := range response.GetInvocations() {
		ids = append(ids, inv.GetId().GetInvocationId())
		labels := inv.GetInvocationAttributes().GetLabels()
		if !overview {
			for _, label := range labels {
				if label == "overview" {
					overview = true
				}
			}
		}
	}
	return ids, overview, response.GetNextPageToken(), nil
}

// search searches for invocation IDs from ResultStore.
func search(ctx context.Context, query string, client InvocationClient, projectID string, max int) ([]string, bool, error) {
	if query == "" {
		return nil, false, errors.New("query cannot be empty")
	}
	if max < 0 {
		return nil, false, errors.New("max must be 0 (unlimited) or positive")
	}
	if projectID == "" {
		return nil, false, errors.New("projectID cannot be empty")
	}
	var invIDs []string
	var pageToken string
	pageSize := 0 // Unlimited
	searchTargets := strings.Contains(query, "id.target_id=")
	for {
		var ids []string
		var overview bool
		var nextPageToken string
		var err error
		if searchTargets {
			ids, nextPageToken, err = targetSearch(ctx, query, client, projectID, pageSize, pageToken)
		} else {
			ids, overview, nextPageToken, err = invocationSearch(ctx, query, client, projectID, pageSize, pageToken)
		}
		if err != nil {
			return invIDs, false, err
		}
		invIDs = append(invIDs, ids...)

		// Stop if there's no next_page_token, or we've hit max results.
		if nextPageToken == "" || (max > 0 && len(invIDs) >= max) {
			return invIDs, overview, nil
		}
		pageToken = nextPageToken
	}
}
