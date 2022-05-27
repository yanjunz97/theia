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
	"net/url"
	"os/exec"
	"time"

	"github.com/ClickHouse/clickhouse-go"
	"github.com/google/uuid"
	"github.com/spf13/cobra"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
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
		// Parse the flags
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
		endpoint, err := cmd.Flags().GetString("endpoint")
		if err != nil {
			return err
		}
		if endpoint != "" {
			_, err := url.ParseRequestURI(endpoint)
			if err != nil {
				return fmt.Errorf("failed to decode input endpoint %s into a url, err: %v", endpoint, err)
			}
		}
		// Verify Clickhouse is running
		clientset, err := CreateK8sClient(kubeconfig)
		if err != nil {
			return fmt.Errorf("couldn't create k8s client using given kubeconfig: %v", err)
		}
		if err := CheckClickHousePod(clientset); err != nil {
			return err
		}

		if endpoint == "" {
			port, err := getClickHouseServicePort(clientset)
			if err != nil {
				return err
			}
			// Forward the ClickHouse service port
			// TODO: use Theia port forwarding instead of kubectl port-forward
			portForwardCmd := exec.Command("kubectl", "port-forward", "service/clickhouse-clickhouse", "-n", flowVisibilityNS, fmt.Sprintf("%d:%d", port, port))
			if err := portForwardCmd.Start(); err != nil {
				return fmt.Errorf("failed to forward port for the ClickHouse Service: %v", err)
			}
			defer portForwardCmd.Process.Kill()
			endpoint = fmt.Sprintf("tcp://localhost:%d", port)
		}

		// Connect to ClickHouse and get the result
		username, password, err := getClickHouseSecret(clientset)
		if err != nil {
			return err
		}
		url := fmt.Sprintf("%s?debug=false&username=%s&password=%s", endpoint, username, password)
		connect, err := connectClickHouse(clientset, url)
		if err != nil {
			return fmt.Errorf("error when connecting to ClickHouse, %v", err)
		}
		recoResult, err := getResultFromClickHouse(connect, recoID)
		if err != nil {
			return fmt.Errorf("error when connecting to ClickHouse, %v", err)
		}
		fmt.Print(recoResult)
		return nil
	},
}

func getClickHouseServicePort(clientset kubernetes.Interface) (int32, error) {
	var servicePort int32
	service, err := clientset.CoreV1().Services(flowVisibilityNS).Get(context.TODO(), "clickhouse-clickhouse", metav1.GetOptions{})
	if err != nil {
		return servicePort, fmt.Errorf("error %v when finding the ClickHouse Service, please check the deployment of the ClickHouse", err)
	}
	for _, port := range service.Spec.Ports {
		if port.Name == "tcp" {
			servicePort = port.Port
		}
	}
	if servicePort == 0 {
		return servicePort, fmt.Errorf("error %v when finding the ClickHouse Service, please check the deployment of the ClickHouse", err)
	}
	return servicePort, nil
}

func getClickHouseSecret(clientset kubernetes.Interface) (username []byte, password []byte, err error) {
	secret, err := clientset.CoreV1().Secrets("flow-visibility").Get(context.TODO(), "clickhouse-secret", metav1.GetOptions{})
	if err != nil {
		return username, password, fmt.Errorf("error %v when finding the ClickHouse secret, please check the deployment of ClickHouse", err)
	}
	username, ok := secret.Data["username"]
	if !ok {
		return username, password, fmt.Errorf("error when getting the ClickHouse username")
	}
	password, ok = secret.Data["password"]
	if !ok {
		return username, password, fmt.Errorf("error when getting the ClickHouse password")
	}
	return username, password, nil
}

func connectClickHouse(clientset kubernetes.Interface, url string) (*sql.DB, error) {
	var connect *sql.DB
	var connErr error
	connRetryInterval := 1 * time.Second
	connTimeout := 10 * time.Second

	// Connect to ClickHouse in a loop
	if err := wait.PollImmediate(connRetryInterval, connTimeout, func() (bool, error) {
		// Open the database and ping it
		var err error
		connect, err = sql.Open("clickhouse", url)
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

func getResultFromClickHouse(connect *sql.DB, id string) (string, error) {
	var recoResult string
	query := "SELECT yamls FROM recommendations WHERE id = (?);"
	err := connect.QueryRow(query, id).Scan(&recoResult)
	if err != nil {
		return recoResult, fmt.Errorf("failed to get recommendation result with id %s: %v", id, err)
	}
	return recoResult, nil
}

func init() {
	policyrecoCmd.AddCommand(resultCmd)
	resultCmd.Flags().StringP(
		"id",
		"i",
		"",
		"ID of the policy recommendation Spark job",
	)
	resultCmd.Flags().StringP(
		"endpoint",
		"e",
		"",
		"The ClickHouse service endpoint",
	)
}
