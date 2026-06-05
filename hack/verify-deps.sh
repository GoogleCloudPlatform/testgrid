#!/usr/bin/env bash
# Copyright 2026 The Kubernetes Authors.
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
set -o pipefail

fail() {
  echo "ERROR: $1. Fix with:" >&2
  echo "  bazel run @io_k8s_repo_infra//hack:update-deps" >&2
  exit 1
}

if [[ -z "${TEST_WORKSPACE:-}" ]]; then
  echo "This script must be run inside bazel test" >&2
  exit 1
fi

tmpfiles=$TEST_TMPDIR/files
mkdir -p "$tmpfiles"
rm -f bazel-*
cp -aL "." "$tmpfiles"
rm -rf "$tmpfiles/external"

(
  export BUILD_WORKSPACE_DIRECTORY=$tmpfiles
  export HOME=$(realpath "$TEST_TMPDIR/home")
  unset GOPATH
  
  go=$(realpath "$2")
  # Explicitly set GOROOT to resolve Go 1.21+ trimmed binary issues in sandboxes
  export GOROOT=$(dirname $(dirname "$go"))
  export PATH=$(dirname "$go"):$PATH
  
  # Run update-deps command passed in arguments
  "$@"
)

(
  # Remove the platform/binary for gazelle and kazel
  gazelle=$(dirname "$3")
  kazel=$(dirname "$4")
  rm -rf {.,"$tmpfiles"}/{"$gazelle","$kazel"}
)

# Avoid diff -N so we handle empty files correctly
diff=$(diff -upr \
  -x ".git" \
  -x "bazel-*" \
  -x "_output" \
  -x "external" \
  "." "$tmpfiles" 2>/dev/null || true)

if [[ -n "${diff}" ]]; then
  echo "${diff}" >&2
  echo >&2
  fail "modules changed"
fi
echo "SUCCESS: modules up-to-date"
