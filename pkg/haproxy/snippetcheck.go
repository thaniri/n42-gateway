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

	hatypes "github.com/n42-gateway/n42-gateway/pkg/haproxy/types"
)

const maxSnippetCheckRounds = 3

func hasSnippet(backend *hatypes.Backend) bool {
	return len(backend.CustomConfigEarly) > 0 || len(backend.CustomConfigLate) > 0
}

func (i *instance) hasChangedSnippets() bool {
	for _, backend := range i.config.Backends().ItemsAdd() {
		if hasSnippet(backend) {
			return true
		}
	}
	return false
}

// checkConfigSnippets drops the backend snippets that `haproxy -c` rejects, and writes the configuration again.
func (i *instance) checkConfigSnippets() error {
	for round := 0; ; round++ {
		err := i.checkFn()
		if err == nil {
			return nil
		}
		if round == maxSnippetCheckRounds {
			return fmt.Errorf("configuration is still invalid after dropping snippets: %w", err)
		}
		dropped := 0
		for _, alert := range parseCheckOutput(err.Error(), os.ReadFile) {
			backend := i.config.Backends().Items()[alert.backendID]
			if backend == nil || !hasSnippet(backend) {
				continue
			}
			backend.CustomConfigEarly = nil
			backend.CustomConfigLate = nil
			// an unchanged backend's shard file would not be written again otherwise
			i.config.Backends().BackendChanged(backend)
			i.logger.Warn("skipping configuration snippet on %s: haproxy rejected it: %s", backend.CustomConfigSource, alert.message)
			i.metrics.IncConfigSnippetsRejected()
			dropped++
		}
		if dropped == 0 {
			return fmt.Errorf("configuration is invalid and no snippet can be dropped: %w", err)
		}
		if err := i.writeConfig(); err != nil {
			return fmt.Errorf("error writing configuration: %w", err)
		}
	}
}
