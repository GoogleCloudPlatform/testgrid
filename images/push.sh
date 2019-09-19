#!/bin/bash
# Copyright 2019 The Kubernetes Authors.
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

# bump.sh will
# * ensure there are no pending changes
# * activate GOOGLE_APPLICATION_CREDENTIALS and configure-docker if set
# * run //images:push, retrying if necessary

set -o errexit
set -o nounset
set -o pipefail

# Coloring Macros
# See https://misc.flogisoft.com/bash/tip_colors_and_formatting

color-version() { # Bold blue
  echo -e "\x1B[1;34m${@}\x1B[0m"
}

color-error() { # Light red
  echo -e "\x1B[91m${@}\x1B[0m"
}

color-target() { # Bold cyan
  echo -e "\x1B[1;33m${@}\x1B[0m"
}

# Authenticate with Google Cloud
if [[ -n "${GOOGLE_APPLICATION_CREDENTIALS:-}" ]]; then
  echo "Detected GOOGLE_APPLICATION_CREDENTIALS, activating..." >&2
  gcloud auth activate-service-account --key-file="${GOOGLE_APPLICATION_CREDENTIALS}"
  gcloud auth configure-docker
fi

# Build and push the current commit, failing on any uncommitted changes.
new_version="v$(date -u '+%Y%m%d')-$(git describe --tags --always --dirty)"
echo -e "version: $(color-version ${new_version})" >&2
if [[ "${new_version}" == *-dirty ]]; then
  echo -e "$(color-error ERROR): uncommitted changes to repo" >&2
  echo "  Fix with git commit" >&2
  exit 1
fi

echo -e "Pushing $(color-version ${new_version})" >&2
# Remove retries after https://github.com/bazelbuild/rules_docker/issues/673
for i in {1..3}; do
  if bazel run //images:push; then
    exit 0
  elif [[ "$i" == 3 ]]; then
    echo "Failed"
    exit 1
  fi
  echo "Failed attempt $i, retrying..."
  sleep 5
done
