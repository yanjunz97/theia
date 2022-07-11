#!/usr/bin/env bash

# Copyright 2022 Antrea Authors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#      http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

function checkDataVersion {
    tables=$(clickhouse client -h 127.0.0.1 -q "SHOW TABLES")
    echo $tables
    if [[ $tables == *"migrate_version"* ]]; then
        dataVersion=$(clickhouse client -h 127.0.0.1 -q "SELECT version FROM migrate_version")
    elif [[ $tables == *"flows"* ]]; then
        dataVersion="0.1.0"
    fi
}

function setDataVersion {
    tables=$(clickhouse client -h 127.0.0.1 -q "SHOW TABLES")
    if [[ $tables == *"migrate_version"* ]]; then
        clickhouse client -h 127.0.0.1 -q "ALTER TABLE migrate_version DELETE WHERE version!=''"
    else
        clickhouse client -h 127.0.0.1 -q "CREATE TABLE migrate_version (version String) engine=MergeTree ORDER BY version"
    fi
    clickhouse client -h 127.0.0.1 -q "INSERT INTO migrate_version (*) VALUES ('{{ .Chart.Version }}')"
    echo "=== Set data schema version to {{ .Chart.Version }} ==="
}

function migrate {
    # Define the upgrading/downgrading path
    VERSION_TO_INDEX="0:0.1.0, 1:0.2.0"
    UPGRADING_MIGRATORS=("010_020.sql")
    DOWNGRADING_MIGRATORS=("020_010.sql")

    THIS_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" >/dev/null 2>&1 && pwd )"
    migratorDir=$THIS_DIR/"migrators"

    checkDataVersion
    if [[ -z "$dataVersion" ]]; then
        echo "=== No existed data schema. Migration finished. ==="
        return 0
    fi
    theiaVersion={{ .Chart.Version | quote }}

    dataVersionIndex=$(echo $VERSION_TO_INDEX | tr ', ' '\n' | grep "$dataVersion" | sed 's/:/ /g' | awk '{print $1}')
    theiaVersionIndex=$(echo $VERSION_TO_INDEX | tr ', ' '\n' | grep "$theiaVersion" | sed 's/:/ /g' | awk '{print $1}')

    # Update along the path
    if [[ "$dataVersionIndex" -lt "$theiaVersionIndex" ]]; then
        for i in $(seq $dataVersionIndex $((theiaVersionIndex-1)) );
        do
            clickhouse client -h 127.0.0.1 --queries-file $migratorDir/${UPGRADING_MIGRATORS[$i]}
        done
    # Downgrade along the path
    elif [[ "$dataVersionIndex" -gt "$theiaVersionIndex" ]]; then
        for i in $(seq $((dataVersionIndex-1)) -1 $theiaVersionIndex);
        do
            clickhouse client -h 127.0.0.1 --queries-file $migratorDir/${DOWNGRADING_MIGRATORS[$i]}
        done
    else
        echo "=== Data schema version is the same as ClickHouse version. Migration finished. ==="
    fi
    setDataVersion
}
