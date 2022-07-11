// Copyright 2022 Antrea Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package e2e

import (
	"flag"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	upgradeToAntreaYML         = "antrea-new.yml"
	upgradeToFlowAggregatorYML = "flow-aggregator-new.yml"
	upgradeToFlowVisibilityYML = "flow-visibility-new.yml"
	upgradeToVersion           = flag.String("upgrade.toVersion", "", "Version updated to")
)

func skipIfNotUpgradeTest(t *testing.T) {
	if *upgradeToVersion == "" {
		t.Skipf("Skipping test as we are not testing for upgrade")
	}
}

// TestUpgrade tests that some basic functionalities are not broken when
// upgrading from one version of Antrea to another. At the moment it checks
// that:
//  * connectivity (intra and inter Node) is not broken
//  * NetworkPolicy can take effect
//  * namespaces can be deleted
//  * Pod deletion leads to correct resource cleanup
// To run the test, provide the -upgrade.toVersion flag.
func TestUpgrade(t *testing.T) {
	skipIfNotUpgradeTest(t)

	data, _, _, err := setupTestForFlowVisibility(t, false, true)
	if err != nil {
		t.Fatalf("Error when setting up test: %v", err)
	}
	defer func() {
		teardownTest(t, data)
		teardownFlowVisibility(t, data, false)
	}()
	t.Logf("Upgrading Antrea YAML to %s", upgradeToAntreaYML)
	// Do not wait for agent rollout as its updateStrategy is set to OnDelete for upgrade test.
	if err := data.deployAntreaCommon(upgradeToAntreaYML, "", false); err != nil {
		t.Fatalf("Error upgrading Antrea: %v", err)
	}
	t.Logf("Restarting all Antrea DaemonSet Pods")
	if err := data.restartAntreaAgentPods(defaultTimeout); err != nil {
		t.Fatalf("Error when restarting Antrea: %v", err)
	}

	t.Logf("Upgrading Flow Visibility YAML to %s", upgradeToFlowVisibilityYML)
	if err := data.deployFlowVisibilityCommon(upgradeToFlowVisibilityYML); err != nil {
		t.Fatalf("Error upgrading Flow Visibility: %v", err)
	}
	t.Logf("Restarting all Flow Visibility Pods")
	if err := data.restartFlowVisibilityPods(); err != nil {
		t.Fatalf("Error when restarting Flow Visibility Pods: %v", err)
	}

	t.Logf("Upgrading Flow Aggregator YAML to %s", upgradeToFlowAggregatorYML)
	if err := data.deployFlowAggregatorCommon(upgradeToFlowAggregatorYML); err != nil {
		t.Fatalf("Error upgrading Flow Aggregator: %v", err)
	}
	t.Logf("Restarting all Flow Aggregator Pods")
	if err := data.restartFlowAggregatorPods(); err != nil {
		t.Fatalf("Error when restarting Flow Aggregator Pods: %v", err)
	}
	checkClickHouseDataSchema(t, data)
}

func checkClickHouseDataSchema(t *testing.T, data *TestData) {
	query := "SELECT version FROM migrate_version"
	queryOutput, _, err := data.RunCommandFromPod(flowVisibilityNamespace, clickHousePodName, "clickhouse", []string{"clickhouse-client", query})
	require.NoErrorf(t, err, "Fail to get version from ClickHouse: %v", queryOutput)
	assert.Contains(t, queryOutput, *upgradeToVersion)
}
