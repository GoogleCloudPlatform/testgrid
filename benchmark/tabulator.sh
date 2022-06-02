#!/usr/bin/env bash
# Copyright 2022 The TestGrid Authors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

set -o errexit
set -o nounset
set -o pipefail

# Time the tabultor to compare algorithm differences.
# Runs across the entire configuration once; ignoring events.

SRC_BUCKET="gs://testgrid-canary"   # An example state to operate on
WORK_BUCKET="/tmp/testgrid-state"  # A bucket to work in. Can be GCS or Local

# Tabulator uses config and testgroup state as inputs
gsutil -m cp ${SRC_BUCKET}/config ${WORK_BUCKET}/config
gsutil -m rsync -r ${SRC_BUCKET}/grid ${WORK_BUCKET}/grid
go test --bench="BenchmarkTabulator" --benchmem --cpuprofile cpu.out --memprofile mem.out --config="${WORK_BUCKET}/config" --confirm
