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
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	hatypes "github.com/n42-gateway/n42-gateway/pkg/haproxy/types"
	"github.com/n42-gateway/n42-gateway/pkg/types/helper_test"
	"github.com/n42-gateway/n42-gateway/pkg/utils"
)

const bogusKeyword = "another-bogus-keyword"

// fakeHAProxyCheck rejects the first line of haproxy.cfg that has bogusKeyword, in the format of `haproxy -c`.
func fakeHAProxyCheck(c *testConfig, calls *int) func() error {
	return func() error {
		*calls++
		cfgFile := filepath.Join(c.tempdir, "haproxy.cfg")
		content, err := os.ReadFile(cfgFile)
		if err != nil {
			return err
		}
		for n, line := range strings.Split(string(content), "\n") {
			if strings.Contains(line, bogusKeyword) {
				return fmt.Errorf("[ALERT]    (1) : config : parsing [%s:%d] : unknown keyword '%s' in 'backend' section", cfgFile, n+1, bogusKeyword)
			}
		}
		return nil
	}
}

func (c *testConfig) addSnippetBackend(name string, snippet ...string) *hatypes.Backend {
	b := c.config.Backends().AcquireBackend("d1", name, "8080")
	b.CustomConfigLate = snippet
	b.CustomConfigSource = "Ingress 'd1/" + name + "'"
	c.httpFrontend(80).AcquireHost(name+".local").AddPath(b, "/", hatypes.MatchBegin)
	return b
}

func TestCheckConfigSnippetsDropsRejected(t *testing.T) {
	c := setup(t)
	defer c.teardown()
	var calls int
	c.instance.options.CheckConfigSnippets = true
	c.instance.checkFn = fakeHAProxyCheck(c, &calls)
	metrics := c.instance.metrics.(*helper_test.MetricsMock)

	invalid := c.addSnippetBackend("app1", bogusKeyword)
	valid := c.addSnippetBackend("app2", "http-response set-header X-Valid 1")
	c.Update()

	assert.Equal(t, 2, calls)
	assert.Nil(t, invalid.CustomConfigLate)
	assert.Equal(t, []string{"http-response set-header X-Valid 1"}, valid.CustomConfigLate)
	cfg := c.readConfig(filepath.Join(c.tempdir, "haproxy.cfg"))
	assert.NotContains(t, cfg, bogusKeyword)
	assert.Contains(t, cfg, "http-response set-header X-Valid 1")
	assert.Equal(t, 1, metrics.ConfigSnippetsRejected)
	require.Len(t, c.logger.Logging, 3)
	assert.Contains(t, c.logger.Logging[0], "WARN skipping configuration snippet on Ingress 'd1/app1': haproxy rejected it: [ALERT]")
	assert.Contains(t, c.logger.Logging[0], "unknown keyword 'another-bogus-keyword'")
	c.logger.Logging = c.logger.Logging[1:]
	c.logger.CompareLogging(defaultLogging)
}

func TestCheckConfigSnippetsSkippedWithoutSnippet(t *testing.T) {
	c := setup(t)
	defer c.teardown()
	var calls int
	c.instance.options.CheckConfigSnippets = true
	c.instance.checkFn = fakeHAProxyCheck(c, &calls)

	b := c.config.Backends().AcquireBackend("d1", "app1", "8080")
	c.httpFrontend(80).AcquireHost("app1.local").AddPath(b, "/", hatypes.MatchBegin)
	c.Update()

	assert.Equal(t, 0, calls)
	c.logger.CompareLogging(defaultLogging)
}

func TestCheckConfigSnippetsDisabled(t *testing.T) {
	c := setup(t)
	defer c.teardown()
	var calls int
	c.instance.checkFn = fakeHAProxyCheck(c, &calls)

	invalid := c.addSnippetBackend("app1", bogusKeyword)
	c.Update()

	assert.Equal(t, 0, calls)
	assert.Equal(t, []string{bogusKeyword}, invalid.CustomConfigLate)
	assert.Contains(t, c.readConfig(filepath.Join(c.tempdir, "haproxy.cfg")), bogusKeyword)
	c.logger.CompareLogging(defaultLogging)
}

func TestCheckConfigSnippetsUnmappedError(t *testing.T) {
	c := setup(t)
	defer c.teardown()
	var calls int
	c.instance.options.CheckConfigSnippets = true
	c.instance.checkFn = func() error {
		calls++
		return fmt.Errorf("[ALERT]    (1) : config : parsing [%s:2] : unknown keyword 'bogus' in 'global' section", filepath.Join(c.tempdir, "haproxy.cfg"))
	}
	metrics := c.instance.metrics.(*helper_test.MetricsMock)

	snippet := c.addSnippetBackend("app1", "http-response set-header X-Valid 1")
	err := c.instance.HAProxyUpdate(utils.NewTimer(nil))

	require.ErrorContains(t, err, "error checking configuration snippets")
	assert.Equal(t, 1, calls)
	assert.Equal(t, []string{"http-response set-header X-Valid 1"}, snippet.CustomConfigLate)
	assert.Equal(t, 0, metrics.ConfigSnippetsRejected)
	c.logger.CompareLogging("")
}

func TestCheckConfigSnippetsStopsAfterMaxRounds(t *testing.T) {
	c := setup(t)
	defer c.teardown()
	var calls int
	c.instance.options.CheckConfigSnippets = true
	c.instance.checkFn = fakeHAProxyCheck(c, &calls)
	metrics := c.instance.metrics.(*helper_test.MetricsMock)

	for _, name := range []string{"app1", "app2", "app3"} {
		c.addSnippetBackend(name, bogusKeyword)
	}
	last := c.addSnippetBackend("app4", bogusKeyword)
	err := c.instance.HAProxyUpdate(utils.NewTimer(nil))

	require.ErrorContains(t, err, "error checking configuration snippets")
	assert.Equal(t, 4, calls)
	assert.Equal(t, 3, metrics.ConfigSnippetsRejected)
	assert.Equal(t, []string{bogusKeyword}, last.CustomConfigLate)
	require.Len(t, c.logger.Logging, 3)
	for i, name := range []string{"app1", "app2", "app3"} {
		assert.Contains(t, c.logger.Logging[i], "WARN skipping configuration snippet on Ingress 'd1/"+name+"'")
	}
	c.logger.Logging = nil
}
