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
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/spf13/cobra"

	sparkv1 "antrea.io/theia/third_party/sparkoperator/v1beta2"
)

// checkCmd represents the check command
var checkCmd = &cobra.Command{
	Use:   "check",
	Short: "Check the status of a policy recommendation Spark job",
	Long: `Check the current status of a policy recommendation Spark job by ID.
It will return the status of this Spark application like SUBMITTED, RUNNING, COMPLETED, or FAILED.`,
	Example: `
Check the current status of job with ID e998433e-accb-4888-9fc8-06563f073e86
$ theiactl policyreco check --id e998433e-accb-4888-9fc8-06563f073e86
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

		err = PolicyRecoPreCheck(clientset)
		if err != nil {
			return err
		}

		sparkApplication := &sparkv1.SparkApplication{}
		err = clientset.CoreV1().RESTClient().
			Get().
			AbsPath("/apis/sparkoperator.k8s.io/v1beta2").
			Namespace(flowVisibilityNS).
			Resource("sparkapplications").
			Name("policy-reco-" + recoID).
			Do(context.TODO()).
			Into(sparkApplication)
		if err != nil {
			return err
		}
		state := strings.TrimSpace(string(sparkApplication.Status.AppState.State))
		fmt.Printf("Status of this policy recommendation job is %s\n", state)
		return nil
		// TODO: add implementation of checking work progress through Spark Monitoring Service after port forwarder finished
	},
}

func init() {
	policyrecoCmd.AddCommand(checkCmd)
	checkCmd.Flags().StringP(
		"id",
		"i",
		"",
		"ID of the policy recommendation Spark job",
	)
}
