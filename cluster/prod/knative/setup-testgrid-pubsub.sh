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

#############
# Sets up the internal queues that TestGrid uses to speak to other parts of itself
#############
set -o nounset
set -o errexit


dir=$(dirname "$0")/../..
create_topic=$dir/create-topic.sh
create_sub=$dir/create-subscription.sh

log() {
    (
        set -o xtrace
        "$@"
    )
}


apply() {
    log "$create_topic" -t "${topic_prefix}-grid" -p 'grid/' "$bucket"
    log "$create_topic" -t "${topic_prefix}-tabs" -p 'tabs/' "$bucket"

    log "$create_sub" -t "${topic_prefix}-grid" -b "$bot" -p "${project}" "$group_sub"
    log "$create_sub" -t "${topic_prefix}-tabs" -b "$bot" -p "${project}" "$tab_sub"
}

project=knative-tests
bucket=gs://knative-own-testgrid
topic_prefix="projects/$project/topics/knative-own-testgrid"
group_sub=test-group-updates
tab_sub=tab-updates
bot=serviceAccount:testgrid-updater@knative-tests.iam.gserviceaccount.com
apply
