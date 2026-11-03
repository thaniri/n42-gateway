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
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseCheckOutput(t *testing.T) {
	mainCfg := strings.Join([]string{
		"global",                      // 1
		"    daemon",                  // 2
		"backend d1_app1_8080",        // 3
		"    mode http",               // 4
		"    another-bogus-keyword",   // 5
		"    http-request also-bogus", // 6
		"",                            // 7
		"frontend _front_http",        // 8
		"    bind :80",                // 9
	}, "\n")
	shardCfg := strings.Join([]string{
		"backend d1_app2_8080",                   // 1
		"    mode http",                          // 2
		"    http-request this-is-not-an-action", // 3
	}, "\n")
	files := map[string]string{
		"/etc/haproxy/haproxy.cfg":             mainCfg,
		"/etc/haproxy/haproxy5-backend003.cfg": shardCfg,
	}
	readFile := func(name string) ([]byte, error) {
		if content, found := files[name]; found {
			return []byte(content), nil
		}
		return nil, fmt.Errorf("file not found: %s", name)
	}
	const unknownKeyword = "[ALERT]    (35) : config : parsing [/etc/haproxy/haproxy.cfg:5] : unknown keyword 'another-bogus-keyword' in 'backend' section"
	testCases := map[string]struct {
		output   string
		expected []checkAlert
	}{
		"unknown keyword in the main file": {
			output: unknownKeyword + `
[ALERT]    (35) : config : Error(s) found in configuration file : /etc/haproxy/haproxy.cfg
[ALERT]    (35) : config : Fatal errors found in configuration.`,
			expected: []checkAlert{{backendID: "d1_app1_8080", message: unknownKeyword}},
		},
		"invalid arguments in a shard file": {
			output: "[ALERT]    (35) : config : parsing [/etc/haproxy/haproxy5-backend003.cfg:3]: 'http-request' expects 'deny', but got 'this-is-not-an-action'.",
			expected: []checkAlert{{
				backendID: "d1_app2_8080",
				message:   "[ALERT]    (35) : config : parsing [/etc/haproxy/haproxy5-backend003.cfg:3]: 'http-request' expects 'deny', but got 'this-is-not-an-action'.",
			}},
		},
		"error in a frontend section": {
			output: "[ALERT]    (35) : config : parsing [/etc/haproxy/haproxy.cfg:9] : unknown keyword 'bogus' in 'frontend' section",
		},
		"post-parse error naming the proxy": {
			output: "[ALERT]    (35) : config : Proxy 'd1_app3_8080': unable to find required use_backend: 'missing'.",
			expected: []checkAlert{{
				backendID: "d1_app3_8080",
				message:   "[ALERT]    (35) : config : Proxy 'd1_app3_8080': unable to find required use_backend: 'missing'.",
			}},
		},
		"no server available is not an error": {
			output: "[ALERT]    (55) : backend 'd1_app1_8080' has no server available!",
		},
		"two errors in the same backend": {
			output: unknownKeyword + "\n" +
				"[ALERT]    (35) : config : parsing [/etc/haproxy/haproxy.cfg:6] : 'http-request' expects 'deny', but got 'also-bogus'.",
			expected: []checkAlert{{backendID: "d1_app1_8080", message: unknownKeyword}},
		},
		"file cannot be read": {
			output: "[ALERT]    (35) : config : parsing [/etc/haproxy/missing.cfg:3] : unknown keyword 'bogus' in 'backend' section",
		},
		"empty output": {},
	}
	for name, test := range testCases {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, test.expected, parseCheckOutput(test.output, readFile))
		})
	}
}
