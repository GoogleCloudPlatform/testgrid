/*
Copyright 2019 The Kubernetes Authors.

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

// Package junit describes the test-infra definition of "junit", and provides
// utilities to parse it.
package junit

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"
)

type suiteOrSuites struct {
	suites Suites
}

func (s *suiteOrSuites) UnmarshalXML(d *xml.Decoder, start xml.StartElement) error {
	switch start.Name.Local {
	case "testsuites":
		d.DecodeElement(&s.suites, &start)
	case "testsuite":
		var suite Suite
		d.DecodeElement(&suite, &start)
		s.suites.Suites = append(s.suites.Suites, suite)
	default:
		return fmt.Errorf("bad element name: %q", start.Name)
	}
	s.suites.Truncate(10000)
	return nil
}

// Suites holds a <testsuites/> list of Suite results
type Suites struct {
	XMLName xml.Name `xml:"testsuites"`
	Suites  []Suite  `xml:"testsuite"`
}

// Truncate ensures that strings do not exceed the specified length.
func (s *Suites) Truncate(max int) {
	for i := range s.Suites {
		s.Suites[i].Truncate(max)
	}
}

// Suite holds <testsuite/> results
type Suite struct {
	XMLName  xml.Name `xml:"testsuite"`
	Suites   []Suite  `xml:"testsuite"`
	Name     string   `xml:"name,attr"`
	Time     float64  `xml:"time,attr"` // Seconds
	Failures int      `xml:"failures,attr"`
	Tests    int      `xml:"tests,attr"`
	Results  []Result `xml:"testcase"`
	/*
	* <properties><property name="go.version" value="go1.8.3"/></properties>
	 */
}

// Truncate ensures that strings do not exceed the specified length.
func (s *Suite) Truncate(max int) {
	for i := range s.Suites {
		s.Suites[i].Truncate(max)
	}
	for i := range s.Results {
		s.Results[i].Truncate(max)
	}
}

// Property defines the xml element that stores additional metrics about each benchmark.
type Property struct {
	Name  string `xml:"name,attr"`
	Value string `xml:"value,attr"`
}

// Properties defines the xml element that stores the list of properties that are associated with one benchmark.
type Properties struct {
	PropertyList []Property `xml:"property"`
}

// Result holds <testcase/> results
type Result struct {
	Name       string      `xml:"name,attr"`
	Time       float64     `xml:"time,attr"`
	ClassName  string      `xml:"classname,attr"`
	Output     *string     `xml:"system-out,omitempty"`
	Error      *string     `xml:"system-err,omitempty"`
	Errored    *Errored    `xml:"error,omitempty"`
	Failure    *Failure    `xml:"failure,omitempty"`
	Skipped    *Skipped    `xml:"skipped,omitempty"`
	Properties *Properties `xml:"properties,omitempty"`
}

// Errored holds <error/> elements.
type Errored struct {
	Message string `xml:"message,attr"`
	Type    string `xml:"type,attr"`
	Value   string `xml:",chardata"`
}

// Failure holds <failure/> elements.
type Failure struct {
	Message string `xml:"message,attr"`
	Type    string `xml:"type,attr"`
	Value   string `xml:",chardata"`
}

// Skipped holds <skipped/> elements.
type Skipped struct {
	Message string `xml:"message,attr"`
	Value   string `xml:",chardata"`
}

// SetProperty adds the specified property to the Result or replaces the
// existing value if a property with that name already exists.
func (r *Result) SetProperty(name, value string) {
	if r.Properties == nil {
		r.Properties = &Properties{}
	}
	for i, existing := range r.Properties.PropertyList {
		if existing.Name == name {
			r.Properties.PropertyList[i].Value = value
			return
		}
	}
	// Didn't find an existing property. Add a new one.
	r.Properties.PropertyList = append(
		r.Properties.PropertyList,
		Property{
			Name:  name,
			Value: value,
		},
	)
}

// Message extracts the message for the junit test case.
//
// Will use the first non-empty <error/>, <failure/>, <skipped/>, <system-err/>, <system-out/> value.
func (r Result) Message(max int) string {
	var msg string
	switch {
	case r.Errored != nil && (r.Errored.Message != "" || r.Errored.Value != ""):
		msg = composeMessage(r.Errored.Message, r.Errored.Value)
	case r.Failure != nil && (r.Failure.Message != "" || r.Failure.Value != ""):
		msg = composeMessage(r.Failure.Message, r.Failure.Value)
	case r.Skipped != nil && (r.Skipped.Message != "" || r.Skipped.Value != ""):
		msg = composeMessage(r.Skipped.Message, r.Skipped.Value)
	case r.Error != nil && *r.Error != "":
		msg = *r.Error
	case r.Output != nil && *r.Output != "":
		msg = *r.Output
	}
	msg = truncate(msg, max)
	if utf8.ValidString(msg) {
		return msg
	}
	return fmt.Sprintf("invalid utf8: %s", strings.ToValidUTF8(msg, "?"))
}

func composeMessage(messages ...string) string {
	nonEmptyMessages := []string{}
	for _, m := range messages {
		if m != "" {
			nonEmptyMessages = append(nonEmptyMessages, m)
		}
	}
	messageBuilder := strings.Builder{}
	for i, m := range nonEmptyMessages {
		messageBuilder.WriteString(m)
		if i+1 < len(nonEmptyMessages) {
			messageBuilder.WriteRune('\n')
		}
	}
	return messageBuilder.String()
}

func truncate(s string, max int) string {
	if max <= 0 {
		return s
	}
	l := len(s)
	if l < max {
		return s
	}
	h := max / 2
	return s[:h] + "..." + s[l-h:]
}

func truncatePointer(str *string, max int) {
	if str == nil {
		return
	}
	s := truncate(*str, max)
	str = &s
}

// Truncate ensures that strings do not exceed the specified length.
func (r Result) Truncate(max int) {
	var errorVal, failureVal, skippedVal string
	if r.Errored != nil {
		errorVal = r.Errored.Value
	}
	if r.Failure != nil {
		failureVal = r.Failure.Value
	}
	if r.Skipped != nil {
		skippedVal = r.Skipped.Value
	}
	for _, s := range []*string{&errorVal, &failureVal, &skippedVal, r.Error, r.Output} {
		truncatePointer(s, max)
	}
}

func unmarshalXML(reader io.Reader, i interface{}) error {
	dec := xml.NewDecoder(reader)
	dec.CharsetReader = func(charset string, input io.Reader) (io.Reader, error) {
		switch charset {
		case "UTF-8", "utf8", "":
			// utf8 is not recognized by golang, but our coalesce.py writes a utf8 doc, which python accepts.
			return input, nil
		default:
			return nil, fmt.Errorf("unknown charset: %s", charset)
		}
	}
	return dec.Decode(i)
}

// Parse returns the Suites representation of these XML bytes.
func Parse(buf []byte) (*Suites, error) {
	if len(buf) == 0 {
		return &Suites{}, nil
	}
	reader := bytes.NewReader(buf)
	return ParseStream(reader)
}

// ParseStream reads bytes into a Suites object.
func ParseStream(reader io.Reader) (*Suites, error) {
	// Try to parse it as a <testsuites/> object
	var s suiteOrSuites
	err := unmarshalXML(reader, &s)
	if err != nil && err != io.EOF {
		return nil, err
	}
	return &s.suites, nil
}
