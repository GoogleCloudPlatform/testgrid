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

package updater

import (
	"regexp"
	"strings"
	"sync"

	"github.com/GoogleCloudPlatform/testgrid/metadata/junit"
	evalpb "github.com/GoogleCloudPlatform/testgrid/pb/custom_evaluator"
	tspb "github.com/GoogleCloudPlatform/testgrid/pb/test_status"
)

var (
	res     = map[string]*regexp.Regexp{} // cache regexps for more speed
	resLock sync.RWMutex
)

func strReg(val, expr string) bool {
	resLock.RLock()
	r, ok := res[expr]
	resLock.RUnlock()
	if !ok {
		var err error
		if r, err = regexp.Compile(expr); err != nil {
			r = nil
		}
		resLock.Lock()
		res[expr] = r
		resLock.Unlock()
	}
	if r == nil {
		return false
	}
	return r.MatchString(val)
}

func strEQ(a, b string) bool { return a == b }
func strNE(a, b string) bool { return a != b }

func strStartsWith(a, b string) bool {
	return strings.HasPrefix(b, a)
}

func strContains(a, b string) bool {
	return strings.Contains(a, b)
}

func numEQ(a, b float64) bool {
	return a == b
}

func numNE(a, b float64) bool {
	return a != b
}

func targetStatusEQ(a, b tspb.TestStatus) bool {
	return a == b
}

// TestResult defines the interface for accessing data about the result.
type TestResult interface {
	// Properties for the test result.
	Properties() map[string][]string
	// Name of the test case
	Name() string
	// The number of errors in the test case.
	Errors() int
	// The number of failures in the test case.
	Failures() int
	// The sequence of exception/error messages in the test case.
	Exceptions() []string
}

// TargetResult defines the interface for accessing data about the target/suite result.
type TargetResult interface {
	TargetStatus() tspb.TestStatus
}

// CustomStatus evaluates the result according to the rules.
//
// Returns nil if no rule matches, otherwise returns the overridden status.
func CustomStatus(rules []*evalpb.Rule, testResult TestResult) *tspb.TestStatus {
	for _, rule := range rules {
		if got := evalProperties(rule, testResult); got != nil {
			return got
		}
	}
	return nil
}

// CustomTargetStatus evaluates the result according to the rules.
//
// Returns nil if no rule matches, otherwise returns the overridden status.
func CustomTargetStatus(rules []*evalpb.Rule, targetResult TargetResult) *tspb.TestStatus {
	for _, rule := range rules {
		if got := evalTargetProperties(rule, targetResult); got != nil {
			return got
		}
	}
	return nil
}

type jUnitTestResult struct {
	Result *junit.Result
}

func (jr jUnitTestResult) Properties() map[string][]string {
	if jr.Result == nil || jr.Result.Properties == nil || jr.Result.Properties.PropertyList == nil {
		return nil
	}
	out := make(map[string][]string, len(jr.Result.Properties.PropertyList))
	for _, p := range jr.Result.Properties.PropertyList {
		out[p.Name] = append(out[p.Name], p.Value)
	}
	return out
}

func (jr jUnitTestResult) Name() string {
	return jr.Result.Name
}

func (jr jUnitTestResult) Errors() int {
	if jr.Result.Errored != nil {
		return 1
	}
	return 0
}

func (jr jUnitTestResult) Failures() int {
	if jr.Result.Failure != nil {
		return 1
	}
	return 0
}

func (jr jUnitTestResult) Exceptions() []string {
	options := make([]string, 0, 2)
	if e := jr.Result.Errored; e != nil {
		options = append(options, e.Value)
	}
	if f := jr.Result.Failure; f != nil {
		options = append(options, f.Value)
	}
	return options
}

func evalProperties(rule *evalpb.Rule, testResult TestResult) *tspb.TestStatus {
	for _, cmp := range rule.TestResultComparisons {
		if cmp.Comparison == nil {
			return nil
		}
		var scmp func(string, string) bool
		var fcmp func(float64, float64) bool
		sval := cmp.Comparison.GetStringValue()
		fval := cmp.Comparison.GetNumericalValue()
		switch cmp.Comparison.Op {
		case evalpb.Comparison_OP_CONTAINS:
			scmp = strContains
		case evalpb.Comparison_OP_REGEX:
			scmp = strReg
		case evalpb.Comparison_OP_STARTS_WITH:
			scmp = strStartsWith
		case evalpb.Comparison_OP_EQ:
			scmp = strEQ
			fcmp = numEQ
		case evalpb.Comparison_OP_NE:
			scmp = strNE
			fcmp = numNE
		}

		if p := cmp.GetPropertyKey(); p != "" {
			if scmp == nil {
				return nil
			}
			var good bool
			props := testResult.Properties()
			if props == nil {
				return nil
			}
			for _, val := range props[p] {
				if !scmp(val, sval) {
					continue
				}
				good = true
				break
			}
			if !good {
				return nil
			}
		} else if f := cmp.GetTestResultField(); f != "" {
			var getNum func() int
			var getStr func() string
			switch f {
			case "name":
				getStr = testResult.Name
			case "error_count":
				getNum = testResult.Errors
			case "failure_count":
				getNum = testResult.Failures
			default: // TODO(fejta): drop or support other fields
				return nil
			}
			switch {
			case getNum != nil:
				n := float64(getNum())
				if !fcmp(n, fval) {
					return nil
				}
			case getStr != nil:
				if !scmp(getStr(), sval) {
					return nil
				}
			}
		} else if ef := cmp.GetTestResultErrorField(); ef != "" {
			if scmp == nil {
				return nil
			}
			if ef != "exception_type" {
				return nil
			}
			var good bool
			for _, got := range testResult.Exceptions() {
				if scmp(got, sval) {
					good = true
					break
				}
			}
			if !good {
				return nil
			}
		} else {
			return nil
		}

	}
	want := rule.GetComputedStatus()
	return &want
}

func evalTargetProperties(rule *evalpb.Rule, targetResult TargetResult) *tspb.TestStatus {
	for _, cmp := range rule.TestResultComparisons {
		if cmp.Comparison == nil {
			return nil
		}
		// Only EQ is supported
		if cmp.Comparison.Op != evalpb.Comparison_OP_EQ {
			return nil
		}
		// Only target_status is supported
		if f := cmp.GetTargetStatusField(); f == true {
			getSts := targetResult.TargetStatus
			stscmp := targetStatusEQ
			stsval := cmp.Comparison.GetTargetStatusValue()
			if !stscmp(getSts(), stsval) {
				return nil
			}
		} else {
			return nil
		}
	}
	want := rule.GetComputedStatus()
	return &want
}
