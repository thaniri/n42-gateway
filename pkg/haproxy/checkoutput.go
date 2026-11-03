/*
Copyright 2026 The HAProxy Ingress Controller Authors.

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

package haproxy

import (
	"regexp"
	"strconv"
	"strings"
)

// checkAlert is an error of `haproxy -c` that points to a backend.
type checkAlert struct {
	backendID string
	message   string
}

var (
	checkParsingRegex = regexp.MustCompile(`parsing \[([^\]]+):([0-9]+)\]`)
	checkProxyRegex   = regexp.MustCompile(`(?i)\b(?:backend|proxy) '([^']+)'`)
)

// parseCheckOutput returns one alert per backend that the errors of `haproxy -c` point to.
func parseCheckOutput(output string, readFile func(name string) ([]byte, error)) []checkAlert {
	var alerts []checkAlert
	seen := map[string]bool{}
	files := map[string][]string{}
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		// alerts without the "config" context, e.g. "has no server available", are not config errors
		if !strings.HasPrefix(line, "[ALERT]") || !strings.Contains(line, ": config :") {
			continue
		}
		var backendID string
		if match := checkParsingRegex.FindStringSubmatch(line); match != nil {
			lines, found := files[match[1]]
			if !found {
				if content, err := readFile(match[1]); err == nil {
					lines = strings.Split(string(content), "\n")
				}
				files[match[1]] = lines
			}
			lineNumber, _ := strconv.Atoi(match[2])
			backendID = backendAtLine(lines, lineNumber)
		} else if match := checkProxyRegex.FindStringSubmatch(line); match != nil {
			backendID = match[1]
		}
		if backendID != "" && !seen[backendID] {
			seen[backendID] = true
			alerts = append(alerts, checkAlert{backendID: backendID, message: line})
		}
	}
	return alerts
}

// backendAtLine returns the ID of the backend section that contains the 1-based lineNumber.
func backendAtLine(lines []string, lineNumber int) string {
	for i := min(lineNumber, len(lines)) - 1; i >= 0; i-- {
		line := lines[i]
		if line == "" || line[0] == ' ' || line[0] == '\t' || line[0] == '#' {
			continue
		}
		if fields := strings.Fields(line); len(fields) > 1 && fields[0] == "backend" {
			return fields[1]
		}
		return ""
	}
	return ""
}
