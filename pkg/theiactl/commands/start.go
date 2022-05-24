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
	"regexp"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/spf13/cobra"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	sparkv1 "antrea.io/theia/third_party/sparkoperator/v1beta2"
)

const (
	flowVisibilityNS     = "flow-visibility"
	k8sQuantitiesReg     = "^([+-]?[0-9.]+)([eEinumkKMGTP]*[-+]?[0-9]*)$"
	sparkImage           = "aurorazhou/theia-policy-recommendation:latest"
	sparkImagePullPolicy = "IfNotPresent"
	sparkAppFile         = "local:///opt/spark/work-dir/policy_recommendation_job.py"
	sparkServiceAccount  = "policy-reco-spark"
	sparkVersion         = "3.1.1"
)

type SparkResourceArgs struct {
	executorInstances   int32
	driverCoreRequest   string
	driverMemory        string
	executorCoreRequest string
	executorMemory      string
}

// startCmd represents the start command
var startCmd = &cobra.Command{
	Use:   "start",
	Short: "Start a new policy recommendation Spark job",
	Long: `Start a new policy recommendation Spark job.
Network policies will be recommended based on the flow records sent by Flow Aggregator.
Must finish the deployment of Theia first, please follow the steps in 
https://github.com/antrea-io/theia/blob/main/docs/network-policy-recommendation.md`,
	Example: `Start a policy recommendation spark job with default configuration
$ theiactl policyreco start
Start an initial policy recommendation spark job with network isolation option 1 and limit on last 10k flow records
$ theiactl policyreco start --type initial --option 1 --limit 10000
Start an initial policy recommendation spark job with network isolation option 1 and limit on flow records from 2022-01-01 00:00:00 to 2022-01-31 23:59:59.
$ theiactl policyreco start --type initial --option 1 --start_time '2022-01-01 00:00:00' --end_time '2022-01-31 23:59:59'
Start a policy recommendation spark job with default configuration but doesn't recommend toServices ANPs
$ theiactl policyreco start --to_services=false
`,
	RunE: func(cmd *cobra.Command, args []string) error {
		var recoJobArgs []string
		sparkResourceArgs := SparkResourceArgs{}

		recoType, err := cmd.Flags().GetString("type")
		if err != nil {
			return err
		}
		if recoType != "initial" && recoType != "subsequent" {
			return fmt.Errorf("recommendation type should be 'initial' or 'subsequent'")
		}
		recoJobArgs = append(recoJobArgs, "--type", recoType)

		limit, err := cmd.Flags().GetInt("limit")
		if err != nil {
			return err
		}
		if limit < 0 {
			return fmt.Errorf("limit should be an integer >= 0")
		}
		recoJobArgs = append(recoJobArgs, "--limit", strconv.Itoa(limit))

		option, err := cmd.Flags().GetInt("option")
		if err != nil {
			return err
		}
		if option < 1 || option > 3 {
			return fmt.Errorf("option of network isolation preference should be 1 or 2 or 3")
		}
		recoJobArgs = append(recoJobArgs, "--option", strconv.Itoa(option))

		startTime, err := cmd.Flags().GetString("start_time")
		if err != nil {
			return err
		}
		if startTime != "" {
			_, err = time.Parse("2006-01-02 15:04:05", startTime)
			if err != nil {
				return fmt.Errorf(`parsing start_time: %v, start_time should be in 
'YYYY-MM-DD hh:mm:ss' format, for example: 2006-01-02 15:04:05`, err)
			}
			recoJobArgs = append(recoJobArgs, "--start_time", startTime)
		}

		endTime, err := cmd.Flags().GetString("end_time")
		if err != nil {
			return err
		}
		if endTime != "" {
			_, err = time.Parse("2006-01-02 15:04:05", endTime)
			if err != nil {
				return fmt.Errorf(`parsing end_time: %v, end_time should be in 
'YYYY-MM-DD hh:mm:ss' format, for example: 2006-01-02 15:04:05`, err)
			}
			recoJobArgs = append(recoJobArgs, "--end_time", endTime)
		}

		nsAllowList, err := cmd.Flags().GetString("ns_allow_list")
		if err != nil {
			return err
		}
		if nsAllowList != "" {
			var parsedNsAllowList []string
			err := json.Unmarshal([]byte(nsAllowList), &parsedNsAllowList)
			if err != nil {
				return fmt.Errorf(`parsing ns_allow_list: %v, ns_allow_list should 
be a list of namespace string, for example: '["kube-system","flow-aggregator","flow-visibility"]'`, err)
			}
			recoJobArgs = append(recoJobArgs, "--ns_allow_list", nsAllowList)
		}

		rmLabels, err := cmd.Flags().GetBool("rm_labels")
		if err != nil {
			return err
		}
		recoJobArgs = append(recoJobArgs, "--rm_labels", strconv.FormatBool(rmLabels))

		toServices, err := cmd.Flags().GetBool("to_services")
		if err != nil {
			return err
		}
		recoJobArgs = append(recoJobArgs, "--to_services", strconv.FormatBool(toServices))

		executorInstances, err := cmd.Flags().GetInt32("executor_instances")
		if err != nil {
			return err
		}
		if executorInstances < 0 {
			return fmt.Errorf("executor_instances should be an integer >= 0")
		}
		sparkResourceArgs.executorInstances = executorInstances

		driverCoreRequest, err := cmd.Flags().GetString("driver_core_request")
		if err != nil {
			return err
		}
		matchResult, err := regexp.MatchString(k8sQuantitiesReg, driverCoreRequest)
		if err != nil || !matchResult {
			return fmt.Errorf("driver_core_request should conform to the Kubernetes convention")
		}
		sparkResourceArgs.driverCoreRequest = driverCoreRequest

		driverMemory, err := cmd.Flags().GetString("driver_memory")
		if err != nil {
			return err
		}
		matchResult, err = regexp.MatchString(k8sQuantitiesReg, driverMemory)
		if err != nil || !matchResult {
			return fmt.Errorf("driver_memory should conform to the Kubernetes convention")
		}
		sparkResourceArgs.driverMemory = driverMemory

		executorCoreRequest, err := cmd.Flags().GetString("executor_core_request")
		if err != nil {
			return err
		}
		matchResult, err = regexp.MatchString(k8sQuantitiesReg, executorCoreRequest)
		if err != nil || !matchResult {
			return fmt.Errorf("executor_core_request should conform to the Kubernetes convention")
		}
		sparkResourceArgs.executorCoreRequest = executorCoreRequest

		executorMemory, err := cmd.Flags().GetString("executor_memory")
		if err != nil {
			return err
		}
		matchResult, err = regexp.MatchString(k8sQuantitiesReg, executorMemory)
		if err != nil || !matchResult {
			return fmt.Errorf("executor_memory should conform to the Kubernetes convention")
		}
		sparkResourceArgs.executorMemory = executorMemory

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

		recommendationID := uuid.New().String()
		recoJobArgs = append(recoJobArgs, "--id", recommendationID)
		recommendationApplication := &sparkv1.SparkApplication{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "sparkoperator.k8s.io/v1beta2",
				Kind:       "SparkApplication",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      "policy-reco-" + recommendationID,
				Namespace: flowVisibilityNS,
			},
			Spec: sparkv1.SparkApplicationSpec{
				Type:                "Python",
				SparkVersion:        sparkVersion,
				Mode:                "cluster",
				Image:               ConstStrToPointer(sparkImage),
				ImagePullPolicy:     ConstStrToPointer(sparkImagePullPolicy),
				MainApplicationFile: ConstStrToPointer(sparkAppFile),
				Arguments:           recoJobArgs,
				Driver: sparkv1.DriverSpec{
					CoreRequest: &driverCoreRequest,
					SparkPodSpec: sparkv1.SparkPodSpec{
						Memory: &driverMemory,
						Labels: map[string]string{
							"version": sparkVersion,
						},
						EnvSecretKeyRefs: map[string]sparkv1.NameKey{
							"CH_USERNAME": {
								Name: "clickhouse-secret",
								Key:  "username",
							},
							"CH_PASSWORD": {
								Name: "clickhouse-secret",
								Key:  "password",
							},
						},
						ServiceAccount: ConstStrToPointer(sparkServiceAccount),
					},
				},
				Executor: sparkv1.ExecutorSpec{
					CoreRequest: &executorCoreRequest,
					SparkPodSpec: sparkv1.SparkPodSpec{
						Memory: &executorMemory,
						Labels: map[string]string{
							"version": sparkVersion,
						},
						EnvSecretKeyRefs: map[string]sparkv1.NameKey{
							"CH_USERNAME": {
								Name: "clickhouse-secret",
								Key:  "username",
							},
							"CH_PASSWORD": {
								Name: "clickhouse-secret",
								Key:  "password",
							},
						},
					},
					Instances: &sparkResourceArgs.executorInstances,
				},
			},
		}
		response := &sparkv1.SparkApplication{}
		err = clientset.CoreV1().RESTClient().
			Post().
			AbsPath("/apis/sparkoperator.k8s.io/v1beta2").
			Namespace(flowVisibilityNS).
			Resource("sparkapplications").
			Body(recommendationApplication).
			Do(context.TODO()).
			Into(response)
		if err != nil {
			return err
		}
		fmt.Printf("A new policy recommendation job is created successfully, id is %s\n", recommendationID)
		return nil
	},
}

func init() {
	policyrecoCmd.AddCommand(startCmd)
	startCmd.Flags().StringP(
		"type",
		"t",
		"initial",
		"{initial|subsequent} Indicates this recommendation is an initial recommendion or a subsequent recommendation job.",
	)
	startCmd.Flags().IntP(
		"limit",
		"l",
		0,
		"The limit on the number of flow records read from the database. 0 means no limit.",
	)
	startCmd.Flags().IntP(
		"option",
		"o",
		1,
		`{1|2|3} Option of network isolation preference in policy recommendation.
Currently we support 3 options:
1: Recommending allow ANP/ACNP policies, with default deny rules only on applied to Pod labels which have allow rules recommended.
2: Recommending allow ANP/ACNP policies, with default deny rules for whole cluster.
3: Recommending allow K8s network policies, with no deny rules at all`,
	)
	startCmd.Flags().StringP(
		"start_time",
		"s",
		"",
		`The start time of the flow records considered for the policy recommendation.
Format is YYYY-MM-DD hh:mm:ss in UTC timezone. No limit of the start time of flow records by default.`,
	)
	startCmd.Flags().StringP(
		"end_time",
		"e",
		"",
		`The end time of the flow records considered for the policy recommendation.
Format is YYYY-MM-DD hh:mm:ss in UTC timezone. No limit of the end time of flow records by default.`,
	)
	startCmd.Flags().StringP(
		"ns_allow_list",
		"n",
		"",
		`List of default traffic allow namespaces.
If no namespaces provided, Traffic inside Antrea CNI related namespaces: ['kube-system', 'flow-aggregator',
'flow-visibility'] will be allowed by default.`,
	)
	startCmd.Flags().Bool(
		"rm_labels",
		true,
		`Enable this option will remove automatically generated Pod labels including 'pod-template-hash',
'controller-revision-hash', 'pod-template-generation'.`,
	)
	startCmd.Flags().Bool(
		"to_services",
		true,
		`Use the toServices feature in ANP and recommendation toServices rules for Pod-to-Service flows,
only works when option is 1 or 2.`,
	)
	startCmd.Flags().Int32(
		"executor_instances",
		1,
		"Specify the number of executors for the spark application. Example values include 1, 2, 8, etc.",
	)
	startCmd.Flags().String(
		"driver_core_request",
		"200m",
		`Specify the cpu request for the driver Pod. Values conform to the Kubernetes convention.
Example values include 0.1, 500m, 1.5, 5, etc.`,
	)
	startCmd.Flags().String(
		"driver_memory",
		"512M",
		`Specify the memory request for the driver Pod. Values conform to the Kubernetes convention.
Example values include 512M, 1G, 8G, etc.`,
	)
	startCmd.Flags().String(
		"executor_core_request",
		"200m",
		`Specify the cpu request for the executor Pod. Values conform to the Kubernetes convention.
Example values include 0.1, 500m, 1.5, 5, etc.`,
	)
	startCmd.Flags().String(
		"executor_memory",
		"512M",
		`Specify the memory request for the executor Pod. Values conform to the Kubernetes convention.
Example values include 512M, 1G, 8G, etc.`,
	)
}
