/*
Copyright 2021 The TestGrid Authors.

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

// A script to quickly check two TestGrid state.protos do not wildly differ.
// Assume that if the column and row names are approx. equivalent, the state
// is probably reasonable.

package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/GoogleCloudPlatform/testgrid/config"
	"github.com/GoogleCloudPlatform/testgrid/util/gcs"
	"github.com/google/go-cmp/cmp"
	"github.com/sirupsen/logrus"
	"google.golang.org/api/iterator"

	statepb "github.com/GoogleCloudPlatform/testgrid/pb/state"
)

type options struct {
	first, second gcs.Path
	configPath    gcs.Path
	creds         string
	diffRatioOK   float64
	debug, trace  bool
	testGroupURL  string
}

// validate ensures reasonable options
func (o *options) validate() error {
	if o.first.String() == "" {
		return errors.New("unset: --first")
	}
	if o.second.String() == "" {
		return errors.New("unset: --second")
	}
	if o.diffRatioOK < 0.0 || o.diffRatioOK > 1.0 {
		return fmt.Errorf("--diff-ratio-ok must be a ratio between 0.0 and 1.0: %f", o.diffRatioOK)
	}
	if o.debug && o.trace {
		return fmt.Errorf("set only one of --debug or --trace log levels")
	}
	if !strings.HasSuffix(o.first.String(), "/") {
		o.first.Set(o.first.String() + "/")
	}
	if !strings.HasSuffix(o.second.String(), "/") {
		o.second.Set(o.second.String() + "/")
	}
	if o.testGroupURL != "" && !strings.HasSuffix(o.testGroupURL, "/") {
		o.testGroupURL += "/"
	}

	return nil
}

// gatherOptions reads options from flags
func gatherFlagOptions(fs *flag.FlagSet, args ...string) options {
	var o options
	fs.Var(&o.first, "first", "First directory of state files to compare.")
	fs.Var(&o.second, "second", "Second directory of state files to compare.")
	fs.Var(&o.configPath, "config", "Path to configuration file (e.g. gs://path/to/config)")
	fs.StringVar(&o.creds, "gcp-service-account", "", "/path/to/gcp/creds (use local creds if empty)")
	fs.Float64Var(&o.diffRatioOK, "diff-ratio-ok", 0.0, "Ratio between 0.0 and 1.0. Only count as different if ratio of differences / total is higher than this.")
	fs.BoolVar(&o.debug, "debug", false, "If true, print detailed info like full diffs.")
	fs.BoolVar(&o.trace, "trace", false, "If true, print extremely detailed info.")
	fs.StringVar(&o.testGroupURL, "test-group-url", "", "Provide a TestGrid URL for viewing test group links (e.g. 'http://k8s.testgrid.io/q/testgroup/')")
	fs.Parse(args)
	return o
}

// gatherOptions reads options from flags
func gatherOptions() options {
	return gatherFlagOptions(flag.CommandLine, os.Args[1:]...)
}

var tgDupRegEx = regexp.MustCompile(` \<TESTGRID:\d+\>`)
var dupRegEx = regexp.MustCompile(` \[\d+\]`)

func rowsAndColumns(ctx context.Context, grid *statepb.Grid, numColumnsRecent int32) (map[string]int, map[string]int, map[string]int) {
	rows := make(map[string]int)
	issues := make(map[string]int)
	for _, row := range grid.GetRows() {
		// Ignore stale rows.
		if numColumnsRecent != 0 && len(row.GetResults()) >= 2 {
			if row.GetResults()[0] == 0 && row.GetResults()[1] >= numColumnsRecent {
				continue
			}
		}
		name := row.GetName()
		// Ignore test methods.
		if strings.Contains(name, "@TESTGRID@") {
			continue
		}
		// Equate duplicate-named rows.
		name = tgDupRegEx.ReplaceAllString(name, "")
		name = dupRegEx.ReplaceAllString(name, "")
		rows[name]++
		for _, issueID := range row.GetIssues() {
			issues[issueID]++
		}
	}

	columns := make(map[string]int)
	for _, column := range grid.GetColumns() {
		// We know times and other data will differ, so ignore them for now.
		key := fmt.Sprintf("%s|%s", column.GetBuild(), strings.Join(column.GetExtra(), "|"))
		columns[key]++
	}

	return rows, columns, issues
}

func numDiff(diff string) int {
	var plus, minus int
	for _, line := range strings.Split(diff, "\n") {
		if strings.HasPrefix(line, "+") {
			plus++
		}
		if strings.HasPrefix(line, "-") {
			minus++
		}
	}
	if plus > minus {
		return plus
	}
	return minus
}

type diffReasons struct {
	firstHasDuplicates  bool
	secondHasDuplicates bool
	other               bool
}

func compareKeys(first, second map[string]int, diffRatioOK float64) (diffed bool) {
	total := len(first)
	if len(second) > len(first) {
		total = len(second)
	}
	firstKeys := make(map[string]bool)
	secondKeys := make(map[string]bool)
	for k := range first {
		firstKeys[k] = true
	}
	for k := range second {
		secondKeys[k] = true
	}
	if diff := cmp.Diff(firstKeys, secondKeys); diff != "" {
		n := numDiff(diff)
		diffRatio := float64(n) / float64(total)
		if diffRatio > diffRatioOK {
			diffed = true
		}
	}
	return
}

func reportDiff(first, second map[string]int, identifier string, diffRatioOK float64) (diffed bool, reasons diffReasons) {
	total := len(second)
	if len(first) > len(second) {
		total = len(first)
	}
	if diff := cmp.Diff(first, second); diff != "" {
		n := numDiff(diff)
		diffRatio := float64(n) / float64(total)
		if diffRatio > diffRatioOK {
			diffed = true
			keysDiffed := compareKeys(first, second, diffRatioOK)
			logrus.Infof("\t❌ %d / %d %ss differ (%.2f) (keys diffed = %t)", n, total, identifier, diffRatio, keysDiffed)
			if keysDiffed {
				reasons.other = true
				logrus.Debugf("\t(-first, +second): %s", diff)
			} else {
				// Guess where the duplicates are based on totals.
				var firstTotal, secondTotal int
				for _, v := range first {
					firstTotal += v
				}
				for _, v := range second {
					secondTotal += v
				}
				if firstTotal > secondTotal {
					reasons.firstHasDuplicates = true
				} else {
					reasons.secondHasDuplicates = true
				}
			}
		}
	} else {
		logrus.Tracef("\t✅ All %d %ss match!", len(first), identifier)
	}
	return
}

func compare(ctx context.Context, first, second *statepb.Grid, diffRatioOK float64, numColumnsRecent int32) (diffed bool, rowDiffReasons, colDiffReasons diffReasons) {
	logrus.Tracef("*****************************")
	firstRows, firstColumns, _ := rowsAndColumns(ctx, first, numColumnsRecent)
	secondRows, secondColumns, _ := rowsAndColumns(ctx, second, numColumnsRecent)
	// both grids have no results, ignore
	if (len(firstRows) == 0 && len(secondRows) == 0) || (len(firstColumns) == 0 && len(secondColumns) == 0) {
		return
	}
	// first has no results, second keeps one column
	if len(firstColumns) == 0 && len(secondColumns) == 1 {
		return
	}

	if rowsDiffed, reasons := reportDiff(firstRows, secondRows, "row", diffRatioOK); rowsDiffed {
		diffed = true
		rowDiffReasons = reasons
	}
	if colsDiffed, reasons := reportDiff(firstColumns, secondColumns, "column", diffRatioOK); colsDiffed {
		diffed = true
		colDiffReasons = reasons
	}
	if diffed {
		logrus.Infof("Compared %q and %q...", first.GetConfig().GetName(), second.GetConfig().GetName())
	}
	return
}

func filenames(ctx context.Context, dir gcs.Path, client gcs.Client) ([]string, error) {
	stats := client.Objects(ctx, dir, "/", "")
	var filenames []string
	for {
		stat, err := stats.Next()
		if errors.Is(err, iterator.Done) {
			break
		}
		if err != nil {
			return nil, err
		}
		filename := dir.String()
		if !strings.HasSuffix(filename, "/") {
			filename += "/"
		}
		filename += filepath.Base(stat.Name)
		filenames = append(filenames, filename)
	}
	return filenames, nil
}

func main() {
	opt := gatherOptions()
	if err := opt.validate(); err != nil {
		logrus.Fatalf("Invalid options %v: %v", opt, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if opt.debug {
		logrus.SetLevel(logrus.DebugLevel)
	} else if opt.trace {
		logrus.SetLevel(logrus.TraceLevel)
	}

	storageClient, err := gcs.ClientWithCreds(ctx, opt.creds)
	if err != nil {
		logrus.Fatalf("Failed to create storage client: %v", err)
	}
	defer storageClient.Close()

	client := gcs.NewClient(storageClient)

	cfg, err := config.Read(ctx, opt.configPath.String(), storageClient)
	if err != nil {
		logrus.WithError(err).WithField("path", opt.configPath.String()).Error("Failed to read configuration, proceeding without config info.")
	}

	firstFiles, err := filenames(ctx, opt.first, client)
	if err != nil {
		logrus.Fatalf("Failed to list files in %q: %v", opt.first.String(), err)
	}
	var diffedMsgs, errorMsgs []string
	var total, notFound int
	rowFirstDups := make(map[string]bool)  // Good; second deduplicates.
	rowSecondDups := make(map[string]bool) // Bad; second adds duplicates.
	colFirstDups := make(map[string]bool)  // Good; second deduplicates.
	colSecondDups := make(map[string]bool) // Bad; second adds duplicates.
	otherDiffed := make(map[string]bool)   // Bad; found unknown differences.
	for _, firstP := range firstFiles {
		tgName := filepath.Base(firstP)
		secondP := opt.second.String()
		if !strings.HasSuffix(secondP, "/") {
			secondP += "/"
		}
		secondP += tgName
		firstPath, err := gcs.NewPath(firstP)
		if err != nil {
			errorMsgs = append(errorMsgs, fmt.Sprintf("gcs.NewPath(%q): %v", firstP, err))
			continue
		}
		// Optionally skip processing some groups.
		tg := config.FindTestGroup(tgName, cfg)
		if tg == nil {
			logrus.Tracef("Did not find test group %q in config", tgName)
			notFound++
			continue
		}

		firstGrid, _, err := gcs.DownloadGrid(ctx, client, *firstPath)
		if err != nil {
			errorMsgs = append(errorMsgs, fmt.Sprintf("gcs.DownloadGrid(%q): %v", firstP, err))
			continue
		}
		secondPath, err := gcs.NewPath(secondP)
		if err != nil {
			errorMsgs = append(errorMsgs, fmt.Sprintf("gcs.NewPath(%q): %v", secondP, err))
			continue
		}
		secondGrid, _, err := gcs.DownloadGrid(ctx, client, *secondPath)
		if err != nil {
			errorMsgs = append(errorMsgs, fmt.Sprintf("gcs.DownloadGrid(%q): %v", secondP, err))
			continue
		}

		if diffed, rowReasons, colReasons := compare(ctx, firstGrid, secondGrid, opt.diffRatioOK, tg.GetNumColumnsRecent()); diffed {
			msg := fmt.Sprintf("%q vs. %q", firstP, secondP)
			if opt.testGroupURL != "" {
				parts := strings.Split(firstP, "/")
				msg = opt.testGroupURL + parts[len(parts)-1]
			}
			if rowReasons.secondHasDuplicates {
				rowSecondDups[msg] = true
			} else if colReasons.secondHasDuplicates {
				colSecondDups[msg] = true
			} else if rowReasons.firstHasDuplicates {
				rowFirstDups[msg] = true
			} else if colReasons.firstHasDuplicates {
				colFirstDups[msg] = true
			} else {
				otherDiffed[msg] = true
			}
			diffedMsgs = append(diffedMsgs, msg)
		}
		total++
	}
	logrus.Infof("Found diffs for %d of %d pairs (%d not found):", len(diffedMsgs), total, notFound)
	report := func(diffs map[string]bool, name string) {
		logrus.Infof("found %d %q:", len(diffs), name)
		for msg := range diffs {
			logrus.Infof("\t* %s", msg)
		}
	}
	report(rowFirstDups, "✅ rows get deduplicated")
	report(colFirstDups, "✅ columns get deduplicated")
	report(rowSecondDups, "❌ rows get duplicated")
	report(colSecondDups, "❌ columns get duplicated")
	report(otherDiffed, "❌ other diffs")
	if n := len(errorMsgs); n > 0 {
		logrus.WithField("count", n).WithField("errors", errorMsgs).Fatal("Errors when diffing directories.")
	}
}
