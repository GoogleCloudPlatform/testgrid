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

ack=10
bots=()
exp=24h
project=
role=roles/pubsub.subscriber
topic=
prefix=

while getopts "t:p:a:e:b:f:" flag; do
    case "$flag" in
        a) ack=$OPTARG;;
        b) bots+=("$OPTARG");;
        e) exp=$OPTARG;;
        p) project=$OPTARG;;
        t) topic=$OPTARG;;
        f) prefix=$OPTARG;;
    esac
done

shift $((OPTIND -1))

if [[ $# -lt 1 || -z "$topic" ]]; then
    echo "Usage: $(basename "$0") [-a $ack] [-b BOT ] [-e $exp] -c [-p $project] <-t TOPIC> SUBSCRIPTION [EXTRA ...]" >&2
    echo >&2
    echo "  -a ACK  : seconds to allow testgrid to ack a message" >&2
    echo "  -b BIND : add IAM binding for member, such as serviceAccount:foo (repeatable)" >&2
    echo "  -e EXP  : number of s/m/h/d to keep unack'd messages" >&2
    echo "  -p PROJ : project to create subscription" >&2
    echo "  -r ROLE : role to bind" >&2
    echo "  -t TOPIC: projects/TOPIC_PROJ/topics/TOPIC" >&2
    echo "  -f PREFIX: filter on object names with this prefix" >&2
    echo >&2
    echo "More info: gcloud pusbsub subscriptions" >&2
    exit 1
fi

name=$1
shift

log() {
    (
        set -o xtrace
        "$@"
    )
}

# Subscribe to topic

verb=create
create_arg=("--topic=$topic" "$@")
prefix_args=()
prefix_arg=""
if [ -n "$prefix" ];then
   prefix_arg="hasPrefix(attributes.objectId, \"$prefix\")"
   prefix_args=("--message-filter=$prefix_arg")
fi

old=$(gcloud pubsub subscriptions describe "--project=$project" "$name" --format='value(topic)' 2>/dev/null || true)
old_filter=$(gcloud pubsub subscriptions describe "--project=$project" "$name" --format='value(filter)' 2>/dev/null || true)

if [[ -n "$old" ]]; then
    if [[ "$old" == "$topic" && "$old_filter" == "$prefix_arg" ]]; then
        echo "Subscription already exists, updating."
        verb=update
        create_arg=()
        prefix_args=()
    else
        echo "WARNING: $project/$name already subscribed to $old (filter: $old_filter)." >&2
        read -p "Delete and replace with a subscription to $topic (filter: $prefix) [yes/NO]: " answer
        case $answer in
            y*|Y*) ;;
            *) exit 1
        esac
        log gcloud pubsub subscriptions "--project=$project" delete "$name"
    fi
fi

log gcloud pubsub subscriptions "$verb" \
    "--project=$project" "$name" \
    "--ack-deadline=$ack" "--expiration-period=$exp" \
    "${prefix_args[@]}" "${create_arg[@]}"

# Add bindings to subscription

for bot in "${bots[@]}"; do
    log gcloud pubsub subscriptions add-iam-policy-binding \
        "--project=$project" "$name" \
        "--member=$bot" "--role=$role"
done
