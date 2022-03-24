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


gcloud=$(command -v gcloud)

role=roles/iam.workloadIdentityUser

bind-service-accounts() {
  gsa=$1
  gsa_project=$(extract-project "$gsa")
  k8s_project=k8s-testgrid
  k8s_ns=$2
  shift 2
  members=($(existing-members "$gsa" "$role"))
  rolemsg=${role#*iam.}
  for k8s_sa in "$@"; do
    member="serviceAccount:$k8s_project.svc.id.goog[$k8s_ns/$k8s_sa]"
    membermsg=$k8s_ns/$k8s_sa
    add=y
    for m in "${members[@]}"; do
      if [[ "$m" == "$member" ]]; then
        add=
        break
      fi
    done
    if [[ "$add" == y ]]; then
      echo "${members[@]} in $member"
      read -p "Grant $member $role access to $gsa? [y/N] " add
    else
      echo "NOOP: $membermsg has $rolemsg access to $gsa"
      continue
    fi

    case "$add" in
      y*|Y*)
      (
        set -o xtrace
        "$gcloud" iam service-accounts \
          --project "$gsa_project" \
          add-iam-policy-binding "$gsa" \
          --role "$role" \
          --member "$member"
      )
      ;;
    esac
    echo "DONE: gave $membermsg $rolemsg access to $gsa"
  done
}

extract-project() {
  gp=${1#*@} # someone@proj.svc.id.goog[whatever] => proj.svc.id.goog[whatever]
  gp=${gp%.iam*} # proj.svc.id.goog[whatever] => proj
  echo $gp
}

existing-members() {
  local gsa=$1
  local proj=$(extract-project "$1")
  local role=$2
  gcloud iam service-accounts \
    --project "$proj" \
    get-iam-policy "$gsa" \
    --filter="bindings.role=$role" \
    --flatten=bindings --format='value[delimiter=" "](bindings.members)'
}



dir=$(dirname "$0")

echo "Service accounts:"
grep -R -E "iam.gke.io/gcp-service-account|serviceAccountName:|namespace:" "$dir" | grep -v grep | sort -u


canary=(
  config-merger
  summarizer
  tabulator
  updater
)
bind-service-accounts testgrid-canary@k8s-testgrid.iam.gserviceaccount.com testgrid-canary "${canary[@]}"

knative=(
  summarizer
  tabulator
  updater
)
bind-service-accounts testgrid-updater@knative-tests.iam.gserviceaccount.com knative "${knative[@]}"

prod=(
  config-merger
  summarizer
  tabulator
  updater
)
bind-service-accounts updater@k8s-testgrid.iam.gserviceaccount.com testgrid "${prod[@]}"
