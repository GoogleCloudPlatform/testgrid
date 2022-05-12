#!/usr/bin/env bash
# Copyright 2022 The Kubernetes Authors.
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

# TODO: Populate these buckets
SRC_BUCKET=""   # An example state to operate on
WORK_BUCKET=""  # A bucket to work in

if [[ -z ${SRC_BUCKET} || -z ${WORK_BUCKET} ]]; then
    echo "Must edit script to add buckets"
    exit 1
fi

gsutil -m rsync -r ${SRC_BUCKET} ${WORK_BUCKET}
bazel build //cmd/tabulator
/usr/bin/time -v bazel run //cmd/tabulator -- --config="${WORK_BUCKET}/config" --confirm --filter --filter-columns $@
