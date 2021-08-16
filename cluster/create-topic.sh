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

# See gsutil notification --help

args=()
topic=
prefixes=()

while getopts "e:t:p:b:" flag; do
    case "$flag" in
        e) args+=(-e "$OPTARG");;
        t) topic=$OPTARG;;
        p) prefixes+=("$OPTARG");;
    esac
done

shift $((OPTIND -1))

if [[ -z "$topic" || $# == 0 ]]; then
    echo "Usage: $(basename "$0") [-e EVENT [-e ...]] [-p PREFIX] <-t TOPIC> <BUCKET ...>" >&2
    echo >&2
    echo "  -e EVENT: OBJECT_FINALIZE|OBJECT_METADATA_UPDATE|OBJECT_DELETE|OBJECT_ARCHIVE (repeatable)" >&2
    echo "  -p PREFIX: only publish messages for objects that start with this name, such as logs/" >&2
    echo "  -t TOPIC: foo or projects/proj/topics/foo" >&2
    echo >&2
    echo "   More info: gsutil notification --help" >&2
    exit 1
fi

log() {
    (
        set -o xtrace
        "$@"
    )
}

for bucket in "$@"; do
    existing=( $(gsutil notification list "$bucket" 2>/dev/null | grep -B 1 "$topic" | grep notificationConfigs || true) )
    for prefix in "${prefixes[@]}"; do
        log gsutil notification create -f json -t "$topic" -p "$prefix" "${args[@]}" "$bucket"
    done
    if [[ ${#existing[@]} -gt 0 ]]; then
        log gsutil notification delete "${existing[@]}"
    fi
done
