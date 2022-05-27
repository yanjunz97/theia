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
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os/exec"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/spf13/cobra"
	"k8s.io/apimachinery/pkg/util/wait"

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
		if state == "RUNNING" {
			// Check the working progress of running recommendation job
			// Forward the policy recommendation service port
			portForwardCmd := exec.Command("kubectl", "port-forward", fmt.Sprintf("service/policy-reco-%s-ui-svc", recoID), "-n", flowVisibilityNS, "4040:4040")
			if err := portForwardCmd.Start(); err != nil {
				return fmt.Errorf("failed to forward port for policy recommendation service, %v", err)
			}
			defer portForwardCmd.Process.Kill()
			stateProgress, err := getPolicyRecommendationProgress(recoID)
			if err != nil {
				return err
			}
			state += stateProgress
		}
		fmt.Printf("Status of this policy recommendation job is %s\n", state)
		return nil
	},
}

func getPolicyRecommendationProgress(id string) (string, error) {
	// Get the id of current spark application
	url := "http://localhost:4040/api/v1/applications"
	response, err := getResponseFromSparkMonitoringSvc(url)
	if err != nil {
		return "", fmt.Errorf("failed to get response from Spark Monitoring service with id %s, %v", id, err)
	}
	var getAppsResult []map[string]interface{}
	json.Unmarshal([]byte(response), &getAppsResult)
	if len(getAppsResult) != 1 {
		return "", fmt.Errorf("wrong number of Spark Application with id %s, expected 1, got %d", id, len(getAppsResult))
	}
	sparkAppID := getAppsResult[0]["id"]
	// Check the percentage of completed stages
	url = fmt.Sprintf("http://localhost:4040/api/v1/applications/%s/stages", sparkAppID)
	response, err = getResponseFromSparkMonitoringSvc(url)
	if err != nil {
		return "", fmt.Errorf("failed to get response from Spark Monitoring service at %s, %v", url, err)
	}
	var getStagesResult []map[string]interface{}
	json.Unmarshal([]byte(response), &getStagesResult)
	if len(getStagesResult) < 1 {
		return "", fmt.Errorf("wrong number of Spark Application stages, expected more than 1, got %d", len(getStagesResult))
	}
	completedStages := 0
	for _, stage := range getStagesResult {
		if stage["status"] == "COMPLETE" || stage["status"] == "SKIPPED" {
			completedStages++
		}
	}
	return fmt.Sprintf(": %d/%d (%d%%) stages completed", completedStages, len(getStagesResult), completedStages*100/len(getStagesResult)), nil
}

func getResponseFromSparkMonitoringSvc(url string) ([]byte, error) {
	sparkMonitoringClient := http.Client{}
	request, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	var res *http.Response
	var getErr error
	connRetryInterval := 1 * time.Second
	connTimeout := 10 * time.Second
	if err := wait.PollImmediate(connRetryInterval, connTimeout, func() (bool, error) {
		res, err = sparkMonitoringClient.Do(request)
		if err != nil {
			getErr = err
			return false, nil
		}
		return true, nil
	}); err != nil {
		return nil, getErr
	}
	if res.Body != nil {
		defer res.Body.Close()
	}
	body, readErr := ioutil.ReadAll(res.Body)
	if readErr != nil {
		return nil, readErr
	}
	return body, nil
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
