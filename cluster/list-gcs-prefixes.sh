#!/usr/bin/env bash
# Copyright 2021 The TestGrid Authors.
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


if [[ $# -lt 1 ]]; then
    echo "Usage: $(basename "$0") <gs://path/to/config>" >&2
    exit 1
fi
# Find all the "gcs_prefix": "bucket/path/to/job" and output bucket/path
go run github.com/GoogleCloudPlatform/testgrid/config/print --path="$1" \
    | grep '"gcs_prefix"' \
    | cut -d '"' -f 4 \
    | cut -d / -f 1-2 \
    | sort -u
