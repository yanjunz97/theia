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
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMonitorMemoryWithDeletion(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	if err != nil {
		t.Fatalf("an error '%s' was not expected when opening a stub database connection", err)
	}
	defer db.Close()

	baseTime := time.Now()
	diskRow := sqlmock.NewRows([]string{"free_space", "total_space"}).AddRow(4, 10)
	partsRow := sqlmock.NewRows([]string{"SUM(bytes)"}).AddRow(5)
	countRow := sqlmock.NewRows([]string{"count"}).AddRow(10)
	timeRow := sqlmock.NewRows([]string{"timeInserted"}).AddRow(baseTime.Add(5 * time.Second))
	mock.ExpectQuery("SELECT free_space, total_space FROM system.disks").WillReturnRows(diskRow)
	mock.ExpectQuery("SELECT SUM(bytes) FROM system.parts").WillReturnRows(partsRow)
	mock.ExpectQuery("SELECT COUNT() FROM flows").WillReturnRows(countRow)
	mock.ExpectQuery("SELECT timeInserted FROM flows LIMIT 1 OFFSET 5").WillReturnRows(timeRow)
	for _, table := range []string{"flows", "flows_pod_view", "flows_node_view", "flows_policy_view"} {
		command := fmt.Sprintf("ALTER TABLE %s DELETE WHERE timeInserted < toDateTime('%v')", table, baseTime.Add(5*time.Second).Format(timeFormat))
		mock.ExpectExec(command).WillReturnResult(sqlmock.NewResult(0, 5))
	}

	tableName = "flows"
	mvNames = []string{"flows_pod_view", "flows_node_view", "flows_policy_view"}
	allocatedSpace = 10
	threshold = 0.5
	deletePercentage = 0.5

	monitorMemory(db)

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("there were unfulfilled expectations: %s", err)
	}

}

func TestMonitorMemoryWithoutDeletion(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	if err != nil {
		t.Fatalf("an error '%s' was not expected when opening a stub database connection", err)
	}
	defer db.Close()

	diskRow := sqlmock.NewRows([]string{"free_space", "total_space"}).AddRow(6, 10)
	partsRow := sqlmock.NewRows([]string{"SUM(bytes)"}).AddRow(5)
	mock.ExpectQuery("SELECT free_space, total_space FROM system.disks").WillReturnRows(diskRow)
	mock.ExpectQuery("SELECT SUM(bytes) FROM system.parts").WillReturnRows(partsRow)

	allocatedSpace = 10
	threshold = 0.5
	deletePercentage = 0.5

	monitorMemory(db)

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("there were unfulfilled expectations: %s", err)
	}
}

func TestParseSize(t *testing.T) {
	size, err := parseSize("500Mi")
	require.NoError(t, err, "Parsing size check failed")
	assert.Equal(t, uint64(500*math.Pow(1024, 2)), size)

	size, err = parseSize("2.5G")
	require.NoError(t, err, "Parsing size check failed")
	assert.Equal(t, uint64(2.5*math.Pow(1000, 3)), size)
}
