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

goog_buckets=(
    gs://oss-prow
    gs://istio-prow
)

goog_topic=projects/k8s-testgrid/topics/prow-updates

k8s_topic=projects/kubernetes-jenkins/topics/prow-updates

k8s_buckets=(
    gs://kubernetes-jenkins
)

dir=$(dirname "$0")
list=$dir/list-gcs-prefixes.sh
create_topic=$dir/create-topic.sh
create_sub=$dir/create-subscription.sh


log() {
    (
        set -o xtrace
        "$@"
    )
}

apply-subscription() {
    topic=$1
    project=$2
    sub=$3
    canary=$4
    # Prod
    log "$create_sub" -t "$topic" \
        -b serviceAccount:updater@k8s-testgrid.iam.gserviceaccount.com \
        -p "$project" "$sub"
    # Canary
    log "$create_sub" -t "$topic" \
        -b serviceAccount:testgrid-canary@k8s-testgrid.iam.gserviceaccount.com \
        -p "$project" "$canary"
}

apply-topic() {
    topic=$1
    shift
    buckets=("$@")
    log "$create_topic" -t "$topic" -p logs/ -p pr-logs/ "${buckets[@]}"
}

do-list() {
    "$list" gs://k8s-testgrid-canary/config
    echo "NOTICE: edit this file ($(basename "$0")) to add any additional paths" >&2
}

something=
while getopts "lst" flag; do
    case "$flag" in
        s)
            apply-subscription "$goog_topic" k8s-testgrid testgrid testgrid-canary
            apply-subscription "$k8s_topic" kubernetes-jenkins testgrid testgrid-canary
            something=yes
            ;;
        t)
            apply-topic "$goog_topic" "${goog_buckets[@]}"
            apply-topic "$k8s_topic" "${k8s_buckets[@]}"
            something=yes
            ;;
        l)
            do-list
            something=yes
            ;;
    esac
done

if [[ -z "$something" ]]; then
    echo "Usage: $(basename "$0") [-u] [-s] [-l]" >&2
    echo >&2
    echo "  -l: list buckets in use" >&2
    echo "  -t: configure topics for ${buckets[@]}" >&2
    echo "  -s: configure subscriptions" >&2
    exit 1
fi
