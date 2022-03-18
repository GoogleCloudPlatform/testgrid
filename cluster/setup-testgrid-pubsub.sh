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


set -o nounset
set -o errexit


dir=$(dirname "$0")
create_topic=$dir/create-topic.sh
create_sub=$dir/create-subscription.sh

log() {
    (
        set -o xtrace
        "$@"
    )
}


apply() {
    log "$create_topic" -t "$topic" -p '' "$bucket"

    # TODO(fejta): filter to correct prefixes (/grid /tabs)
    log "$create_sub" -t "$topic" -b "$bot" -p "$project" "$group_sub"
    log "$create_sub" -t "$topic" -b "$bot" -p "$project" "$tab_sub"
}

project=k8s-testgrid

bucket=gs://k8s-testgrid
topic="projects/$project/topics/testgrid"
group_sub=test-group-updates
tab_sub=tab-updates
bot=serviceAccount:updater@k8s-testgrid.iam.gserviceaccount.com
apply


bucket=gs://k8s-testgrid-canary
topic="projects/$project/topics/canary-testgrid"
group_sub=canary-test-group-updates
tab_sub=canary-tab-updates
bot=serviceAccount:testgrid-canary@k8s-testgrid.iam.gserviceaccount.com
apply
