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
	"database/sql"
	"fmt"
	"time"

	"github.com/ClickHouse/clickhouse-go"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

func CreateK8sClient(kubeconfig string) (*kubernetes.Clientset, error) {
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

func PolicyRecoPreCheck(clientset *kubernetes.Clientset) error {
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

func CheckClickHousePod(clientset *kubernetes.Clientset) error {
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
	return nil
}

func getServicePort(clientset *kubernetes.Clientset, name string) (int32, error) {
	var servicePort int32
	service, err := clientset.CoreV1().Services(flowVisibilityNS).Get(context.TODO(), name, metav1.GetOptions{})
	if err != nil {
		return servicePort, fmt.Errorf("error %v when finding Service %s, please check the deployment of the ClickHouse", err, name)
	}
	for _, port := range service.Spec.Ports {
		if port.Name == "tcp" {
			servicePort = port.Port
		}
	}
	if servicePort == 0 {
		return servicePort, fmt.Errorf("error %v when finding Service %s, please check the deployment of the ClickHouse", err, name)
	}
	return servicePort, nil
}

// Connect to ClickHouse in a loop
func ConnectClickHouse(clientset *kubernetes.Clientset, url string) (*sql.DB, error) {
	// Get the ClickHouse connection secret
	secret, err := clientset.CoreV1().Secrets("flow-visibility").Get(context.TODO(), "clickhouse-secret", metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("error %v when finding the ClickHouse secret, please check the deployment of ClickHouse", err)
	}
	userName, ok := secret.Data["username"]
	if !ok {
		return nil, fmt.Errorf("error %v when getting the ClickHouse username", err)
	}
	password, ok := secret.Data["password"]
	if !ok {
		return nil, fmt.Errorf("error %v when getting the ClickHouse password", err)
	}
	var connect *sql.DB
	var connErr error
	connRetryInterval := 1 * time.Second
	connTimeout := 10 * time.Second
	if err := wait.PollImmediate(connRetryInterval, connTimeout, func() (bool, error) {
		// Open the database and ping it
		dataSourceName := fmt.Sprintf("%s?debug=false&username=%s&password=%s", url, userName, password)
		var err error
		connect, err = sql.Open("clickhouse", dataSourceName)
		if err != nil {
			connErr = fmt.Errorf("failed to ping ClickHouse: %v", err)
			return false, nil
		}
		if err := connect.Ping(); err != nil {
			if exception, ok := err.(*clickhouse.Exception); ok {
				connErr = fmt.Errorf("failed to ping ClickHouse: %v", exception.Message)
			} else {
				connErr = fmt.Errorf("failed to ping ClickHouse: %v", err)
			}
			return false, nil
		} else {
			return true, nil
		}
	}); err != nil {
		return nil, fmt.Errorf("failed to connect to ClickHouse after %s: %v", connTimeout, connErr)
	}
	return connect, nil
}

func ConstStrToPointer(constStr string) *string {
	return &constStr
}
