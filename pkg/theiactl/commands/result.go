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

package commands

import (
	"fmt"
	"os/exec"

	"github.com/google/uuid"
	"github.com/spf13/cobra"
)

// resultCmd represents the result command
var resultCmd = &cobra.Command{
	Use:   "result",
	Short: "Get the recommendation result of a policy recommendation Spark job",
	Long: `Get the recommendation result of a policy recommendation Spark job by ID.
It will return the recommended network policies described in yaml.`,
	Example: `
Get the recommendation result with job ID e998433e-accb-4888-9fc8-06563f073e86
$ theiactl policyreco result --id e998433e-accb-4888-9fc8-06563f073e86
`,
	RunE: func(cmd *cobra.Command, args []string) error {
		recoID, err := cmd.Flags().GetString("id")
		if err != nil {
			return err
		}
		_, err = uuid.Parse(recoID)
		if err != nil {
			return fmt.Errorf("failed to decode input id %s into a UUID, err: %v", recoID, err)
		}

		kubeconfig, err := cmd.Flags().GetString("kubeconfig")
		if err != nil {
			return err
		}
		clientset, err := CreateK8sClient(kubeconfig)
		if err != nil {
			return fmt.Errorf("couldn't create k8s client using given kubeconfig, %v", err)
		}
		err = CheckClickHousePod(clientset)
		if err != nil {
			return err
		}
		// Forward the ClickHouse service port
		serviceName := "clickhouse-clickhouse"
		port, err := getServicePort(clientset, serviceName)
		if err != nil {
			return err
		}
		portForwardCmd := exec.Command("kubectl", "port-forward", fmt.Sprintf("service/%s", serviceName), "-n", flowVisibilityNS, fmt.Sprintf("%d:%d", port, port))
		if err := portForwardCmd.Start(); err != nil {
			return fmt.Errorf("fail to forward port for service %s, %v", serviceName, err)
		}
		defer portForwardCmd.Process.Kill()
		// get the recommendation result from ClickHouse with id
		var recoResult string
		databaseURL := fmt.Sprintf("tcp://localhost:%d", port)
		connect, err := ConnectClickHouse(clientset, databaseURL)
		if err != nil {
			return fmt.Errorf("error when connecting to ClickHouse, %v", err)
		}
		query := "SELECT yamls FROM recommendations WHERE id = (?);"
		err = connect.QueryRow(query, recoID).Scan(&recoResult)
		if err != nil {
			return fmt.Errorf("get recommendation result failed of recommendation job with id %s: %v", recoID, err)
		}
		fmt.Print(recoResult)
		return nil
	},
}

func init() {
	policyrecoCmd.AddCommand(resultCmd)
	resultCmd.Flags().StringP(
		"id",
		"i",
		"",
		"ID of the policy recommendation Spark job",
	)
}
