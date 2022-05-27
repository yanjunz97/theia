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

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

func CreateK8sClient(kubeconfig string) (kubernetes.Interface, error) {
	var err error
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		return nil, err
	}
	// creates the clientset
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, err
	}
	return clientset, nil
}

func PolicyRecoPreCheck(clientset kubernetes.Interface) error {
	// Check the deployment of Spark Operator in flow-visibility ns
	pods, err := clientset.CoreV1().Pods(flowVisibilityNS).List(context.TODO(), metav1.ListOptions{
		LabelSelector: "app.kubernetes.io/instance=policy-reco,app.kubernetes.io/name=spark-operator",
	})
	if err != nil {
		return fmt.Errorf("error %v when finding the policy-reco-spark-operator Pod, please check the deployment of the Spark Operator", err)
	}
	if len(pods.Items) < 1 {
		return fmt.Errorf("can't find the policy-reco-spark-operator Pod, please check the deployment of the Spark Operator")
	}
	CheckClickHousePod(clientset)
	return nil
}

func CheckClickHousePod(clientset kubernetes.Interface) error {
	// Check the ClickHouse deployment in flow-visibility namespace
	pods, err := clientset.CoreV1().Pods(flowVisibilityNS).List(context.TODO(), metav1.ListOptions{
		LabelSelector: "app=clickhouse",
	})
	if err != nil {
		return fmt.Errorf("error %v when finding the ClickHouse Pod, please check the deployment of the ClickHouse", err)
	}
	if len(pods.Items) < 1 {
		return fmt.Errorf("can't find the ClickHouse Pod, please check the deployment of ClickHouse")
	}
	hasRunningPod := false
	for _, pod := range pods.Items {
		if pod.Status.Phase == "Running" {
			hasRunningPod = true
			break
		}
	}
	if !hasRunningPod {
		return fmt.Errorf("can't find a running ClickHouse Pod, please check the deployment of ClickHouse")
	}
	return nil
}

func ConstStrToPointer(constStr string) *string {
	return &constStr
}
