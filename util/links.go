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

package util

import (
	"net/url"
	"regexp"
	"strings"

	configpb "github.com/GoogleCloudPlatform/testgrid/pb/config"
)

// Utility functions to correctly parse link templates.

var (
	// All tokens.
	tokenRe = regexp.MustCompile(`<[^<>]*>`)

	// Find the raw encode token: `<encode:[text]>`.
	encodeTokenRe = regexp.MustCompile(`<encode:[^<>]*>`)
	encodeRe      = regexp.MustCompile(`<encode:([^<>]*)>`)

	// Substitute a particular custom column value for this token.
	// TODO: Handle start/end columns (for custom columns and build IDs).
	CustomColumnRe = regexp.MustCompile(`<custom-\d+>`)

	// Simple tokens.
	TestStatus      = "<test-status>"
	TestID          = "<test-id>"
	WorkflowID      = "<workflow-id>"
	WorkflowName    = "<workflow-name>"
	TestName        = "<test-name>"
	DisplayName     = "<display-name>"
	MethodName      = "<method-name>"
	TestURL         = "<test-url>"
	BuildID         = "<build-id>"
	BugComponent    = "<bug-component>"
	Owner           = "<owner>"
	Cc              = "<cc>"
	GcsPrefix       = "<gcs-prefix>"
	Environment     = "<environment>" // dashboard tab name
	ResultsExplorer = "<results-explorer>"
	CodeSearchPath  = "<cs-path>"
)

// Tokens returns the unique list of all Tokens that could be replaced in this template.
func Tokens(template *configpb.LinkTemplate) []string {
	allTokens := map[string]bool{}
	for _, token := range tokenRe.FindAllString(template.GetUrl(), -1) {
		allTokens[token] = true
	}
	for _, option := range template.GetOptions() {
		for _, token := range tokenRe.FindAllString(option.GetKey(), -1) {
			allTokens[token] = true
		}
		for _, token := range tokenRe.FindAllString(option.GetValue(), -1) {
			allTokens[token] = true
		}
	}
	var keys []string
	for k := range allTokens {
		keys = append(keys, k)
	}
	return keys
}

// ExpandTemplate expands the given link template with given parameters.
func ExpandTemplate(template *configpb.LinkTemplate, parameters map[string]string) (string, error) {
	rawUrl := expandTemplateString(template.GetUrl(), parameters, false)
	baseUrl, err := url.Parse(rawUrl)
	if err != nil {
		return "", err
	}
	options := url.Values{}
	for _, optionTemplate := range template.GetOptions() {
		k := expandTemplateString(optionTemplate.GetKey(), parameters, true)
		v := expandTemplateString(optionTemplate.GetValue(), parameters, true)
		options.Add(k, v)
	}
	baseUrl.RawQuery = options.Encode()
	return baseUrl.String(), nil
}

// expandTemplateString substitutes given parameters into the given link template.
func expandTemplateString(template string, parameters map[string]string, isQuery bool) string {
	// TODO: Handle start/end tokens.
	// Substitute parameters for simple tokens.
	for token, value := range parameters {
		template = strings.ReplaceAll(template, token, value)
	}
	escape := url.PathEscape
	if isQuery {
		// Don't encode here; options.Encode() will do that.
		escape = func(s string) string { return s }
	}
	// Encode specified parameters.
	return encodeTokenRe.ReplaceAllStringFunc(
		template,
		func(token string) string {
			submatches := encodeRe.FindStringSubmatch(token)
			if len(submatches) <= 1 {
				return ""
			}
			return escape(submatches[1])
		},
	)
}
