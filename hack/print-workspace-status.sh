#!/bin/bash
# Copyright 2017 The Kubernetes Authors.
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

git_commit="$(git describe --tags --always --dirty)"
build_date="$(date -u '+%Y%m%d')"
docker_tag="v${build_date}-${git_commit}"

cat <<EOF
STABLE_TESTGRID_REPO ${TESTGRID_REPO_OVERRIDE:-gcr.io/k8s-testgrid}
STABLE_TESTGRID_CLUSTER ${TESTGRID_CLUSTER_OVERRIDE:-gke_k8s-testgrid_us-central1_auto}
STABLE_BUILD_GIT_COMMIT ${git_commit}
DOCKER_TAG ${docker_tag}
EOF
