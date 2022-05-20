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
		// TODO: setup port forwarder using clientset.
		// TODO: get the recommendation result from ClickHouse using recoID
		fmt.Printf("recoID: %v, clientset: %v\n", recoID, clientset)
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
