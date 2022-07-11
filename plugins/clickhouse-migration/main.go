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

package main

import (
	"database/sql"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/ClickHouse/clickhouse-go"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"
)

type Migrator func(*sql.DB) error

var (
	versionToIndex = map[string]int{
		"v0.1.0": 0,
		"v0.2.0": 1,
	}
	upgradingMigrators = []Migrator{
		migrateV010ToV020,
	}
	downgradingMigrators = []Migrator{
		migrateV020ToV010,
	}
)

const (
	// Connection to ClickHouse times out if it fails for 1 minute.
	connTimeout = time.Minute
	// Retry connection to ClickHouse every 10 seconds if it fails.
	connRetryInterval = 10 * time.Second
	// Query to ClickHouse time out if it fails for 10 seconds.
	queryTimeout = 10 * time.Second
	// Retry query to ClickHouse every second if it fails.
	queryRetryInterval = 1 * time.Second
)

func migrateV010ToV020(*sql.DB) error {
	return nil
}

func migrateV020ToV010(*sql.DB) error {
	return nil
}

func main() {
	expectedNumMigrators := len(versionToIndex) - 1
	if len(upgradingMigrators) != expectedNumMigrators {
		klog.ErrorS(nil, "No enough migrators to upgrade the data schema version", "actualNumMigrators", len(upgradingMigrators), "expectedNumMigrators", expectedNumMigrators)
	}
	if len(downgradingMigrators) != expectedNumMigrators {
		klog.ErrorS(nil, "No enough migrators to downgrade the data schema version", "actualNumMigrators", len(downgradingMigrators), "expectedNumMigrators", expectedNumMigrators)
	}
	connect, err := connectLoop()
	if err != nil {
		klog.ErrorS(err, "Error when connecting to ClickHouse")
		return
	}
	dataVersion, err := getDataVersion(connect)
	if err != nil {
		klog.ErrorS(err, "Failed to get the data schema version")
		return
	}
	if dataVersion == "not found" {
		klog.InfoS("No data schema exists. Data schema migration finished.")
		return
	}
	theiaVersion, err := getTheiaVerstion()
	if err != nil {
		klog.ErrorS(err, "Failed to get the Theia version")
		return
	}
	dataVersionIndex, ok := versionToIndex[dataVersion]
	if !ok {
		klog.ErrorS(nil, "Cannot recognize the data schema version", "dataVersion", dataVersion)
		return
	}
	theiaVersionIndex, ok := versionToIndex[theiaVersion]
	if !ok {
		klog.ErrorS(nil, "Cannot recognize the Theia version", "theiaVersion", theiaVersion)
		return
	}
	if dataVersionIndex < theiaVersionIndex {
		for i := dataVersionIndex; i < theiaVersionIndex; i++ {
			upgradingMigrators[i](connect)
		}
		klog.InfoS("Data schema upgrading finished.", "from", dataVersion, "to", theiaVersion)
	} else if dataVersionIndex > theiaVersionIndex {
		for i := theiaVersionIndex - 1; i >= dataVersionIndex; i-- {
			downgradingMigrators[i](connect)
		}
		klog.InfoS("Data schema downgrading finished.", "from", dataVersion, "to", theiaVersion)
	} else {
		klog.InfoS("Data schema version is the same as ClickHouse version. Data schema migration finished.")
	}
	err = setDataVersion(connect, theiaVersion)
	if err != nil {
		klog.ErrorS(err, "Failed to update the data schema version")
	}
}

func setDataVersion(connect *sql.DB, version string) error {
	command := "INSERT INTO migrate_version (*) VALUES (?) ;"
	if err := wait.PollImmediate(queryRetryInterval, queryTimeout, func() (bool, error) {
		if _, err := connect.Exec(command, version); err != nil {
			return false, nil
		} else {
			return true, nil
		}
	}); err != nil {
		return err
	}
	return nil
}

func getDataVersion(connect *sql.DB) (string, error) {
	var version string
	command := "SELECT version FROM migrate_version"
	if err := wait.PollImmediate(queryRetryInterval, queryTimeout, func() (bool, error) {
		if err := connect.QueryRow(command).Scan(&version); err != nil {
			if strings.Contains(err.Error(), "Table default.migrate_version doesn't exist") {
				version = "not found"
				return true, nil
			} else {
				return false, nil
			}
		} else {
			return true, nil
		}
	}); err != nil {
		return version, err
	}
	// v0.1.0 does not have a version table
	// Check the existense of flows table to distinguish v0.1.0 from empty data schema
	if version == "not found" {
		var count uint64
		command = "SELECT COUNT() FROM flows"
		if err := wait.PollImmediate(queryRetryInterval, queryTimeout, func() (bool, error) {
			if err := connect.QueryRow(command).Scan(&count); err != nil {
				return false, nil
			} else {
				version = "v0.1.0"
				return true, nil
			}
		}); err != nil {
			return version, err
		}
	}
	return version, nil
}

func getTheiaVerstion() (string, error) {
	theiaVersion := os.Getenv("THEIA_VERSION")
	if len(theiaVersion) == 0 {
		return theiaVersion, fmt.Errorf("unable to load environment variables, THEIA_VERSION must be defined")
	}
	return theiaVersion, nil
}

// Connects to ClickHouse in a loop
func connectLoop() (*sql.DB, error) {
	// ClickHouse configuration
	userName := os.Getenv("CLICKHOUSE_USERNAME")
	password := os.Getenv("CLICKHOUSE_PASSWORD")
	databaseURL := os.Getenv("DB_URL")
	if len(userName) == 0 || len(password) == 0 || len(databaseURL) == 0 {
		return nil, fmt.Errorf("unable to load environment variables, CLICKHOUSE_USERNAME, CLICKHOUSE_PASSWORD and DB_URL must be defined")
	}
	var connect *sql.DB
	if err := wait.PollImmediate(connRetryInterval, connTimeout, func() (bool, error) {
		// Open the database and ping it
		dataSourceName := fmt.Sprintf("%s?debug=true&username=%s&password=%s", databaseURL, userName, password)
		var err error
		connect, err = sql.Open("clickhouse", dataSourceName)
		if err != nil {
			klog.ErrorS(err, "Failed to connect to ClickHouse")
			return false, nil
		}
		if err := connect.Ping(); err != nil {
			if exception, ok := err.(*clickhouse.Exception); ok {
				klog.ErrorS(nil, "Failed to ping ClickHouse", "message", exception.Message)
			} else {
				klog.ErrorS(err, "Failed to ping ClickHouse")
			}
			return false, nil
		} else {
			return true, nil
		}
	}); err != nil {
		return nil, fmt.Errorf("failed to connect to ClickHouse after %s", connTimeout)
	}
	return connect, nil
}
