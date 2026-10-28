// Command gitniblet-codeql-check reads a CodeQL SARIF file and fails on any
// result that isn't on the accepted list.
//
// Every result in our own code counts. In vendor/ only the security queries
// count, since the quality ones flag third-party style we'd never change. An
// accepted entry that matches nothing fails too, so the list can't go stale.
//
//	gitniblet-codeql-check <results.sarif> <accepted.txt>
//
// accepted.txt has one "<rule id> <path>" per line, the path relative to
// source/, then "##" and why. Blank lines and lines starting with # are skipped.
//
// Copyright © 2026 Jim Collier [ID: 2უNაɘ«҂թȹɤξπ๙¿ձϖ]
// SPDX-License-Identifier: MIT
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"strings"
)

type sarif struct {
	Runs []struct {
		Tool struct {
			Driver     component   `json:"driver"`
			Extensions []component `json:"extensions"`
		} `json:"tool"`
		Results []struct {
			RuleID  string `json:"ruleId"`
			Message struct {
				Text string `json:"text"`
			} `json:"message"`
			Locations []struct {
				PhysicalLocation struct {
					ArtifactLocation struct {
						URI string `json:"uri"`
					} `json:"artifactLocation"`
					Region struct {
						StartLine int `json:"startLine"`
					} `json:"region"`
				} `json:"physicalLocation"`
			} `json:"locations"`
		} `json:"results"`
	} `json:"runs"`
}

type component struct {
	Rules []struct {
		ID         string `json:"id"`
		Properties struct {
			Tags []string `json:"tags"`
		} `json:"properties"`
	} `json:"rules"`
}

func main() {
	if len(os.Args) != 3 {
		fmt.Fprintln(os.Stderr, "usage: gitniblet-codeql-check <results.sarif> <accepted.txt>")
		os.Exit(2)
	}
	n, err := check(os.Args[1], os.Args[2])
	if err != nil {
		fmt.Fprintln(os.Stderr, "codeql-check:", err)
		os.Exit(2)
	}
	if n > 0 {
		os.Exit(1)
	}
}

// check prints a line per new result or stale entry, then a summary, and
// returns how many of those there were.
func check(sarifPath, acceptedPath string) (int, error) {
	accepted, err := readAccepted(acceptedPath)
	if err != nil {
		return 0, err
	}
	b, err := os.ReadFile(sarifPath)
	if err != nil {
		return 0, err
	}
	var s sarif
	if err := json.Unmarshal(b, &s); err != nil {
		return 0, fmt.Errorf("%s: %w", sarifPath, err)
	}
	if len(s.Runs) == 0 {
		return 0, fmt.Errorf("%s: no runs", sarifPath)
	}

	used := map[string]bool{}
	var total, counted, bad int
	for _, run := range s.Runs {
		security := map[string]bool{}
		for _, c := range append([]component{run.Tool.Driver}, run.Tool.Extensions...) {
			for _, r := range c.Rules {
				security[r.ID] = slices.Contains(r.Properties.Tags, "security")
			}
		}
		for _, r := range run.Results {
			total++
			path, line := "?", 0
			if len(r.Locations) > 0 {
				path = r.Locations[0].PhysicalLocation.ArtifactLocation.URI
				line = r.Locations[0].PhysicalLocation.Region.StartLine
			}
			if strings.HasPrefix(path, "vendor/") && !security[r.RuleID] {
				continue
			}
			counted++
			key := r.RuleID + " " + path
			if _, ok := accepted[key]; ok {
				used[key] = true
				continue
			}
			bad++
			msg, _, _ := strings.Cut(r.Message.Text, "\n")
			fmt.Printf("FAIL %s %s:%d: %s\n", r.RuleID, path, line, msg)
		}
	}
	keys := make([]string, 0, len(accepted))
	for k := range accepted {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	for _, k := range keys {
		if !used[k] {
			bad++
			fmt.Printf("STALE %s: accepted at line %d, but nothing matches it now\n", k, accepted[k])
		}
	}
	fmt.Printf("CodeQL: %d results, %d in own code or security in vendor/, %d accepted, %d new or stale\n",
		total, counted, len(used), bad)
	return bad, nil
}

// readAccepted maps "<rule id> <path>" to its line number.
func readAccepted(path string) (map[string]int, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	out := map[string]int{}
	sc := bufio.NewScanner(f)
	for n := 1; sc.Scan(); n++ {
		line, _, _ := strings.Cut(sc.Text(), "##")
		fields := strings.Fields(line)
		if len(fields) == 0 || strings.HasPrefix(fields[0], "#") {
			continue
		}
		if len(fields) != 2 {
			return nil, fmt.Errorf("%s:%d: want \"<rule id> <path> ## why\"", path, n)
		}
		out[fields[0]+" "+fields[1]] = n
	}
	return out, sc.Err()
}
