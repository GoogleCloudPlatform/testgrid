#!/usr/bin/env bash
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


set -o errexit
set -o nounset
set -o pipefail

if [[ -n "${BUILD_WORKSPACE_DIRECTORY:-}" ]]; then # Running inside bazel
  echo "Updating protos..." >&2
else
  bazel=$(command -v bazelisk || command -v bazel || true)
  if [[ -z "$bazel" ]]; then
      echo "Install bazel at https://bazel.build" >&2
      exit 1
  fi
  (
    set -o xtrace
    "$bazel" run //hack:update-protos
  )
  exit 0
fi

protoc=$1
plugin=$2
boiler=$3
grpc=$4
importmap=$5
dest=$BUILD_WORKSPACE_DIRECTORY

if [[ -z "${_virtual_imports:-}" ]]; then
  export _virtual_imports="$0.runfiles/com_google_protobuf/_virtual_imports"
fi

genproto() {
  dir=$(dirname "$1")
  base=$(basename "$1")
  out=$dest/$dir/${base%.proto}.pb.go
  rm -f "$out" # mac will complain otherwise
  (
    # TODO(fejta): this _virtual_imports piece is super fragile
    # Add any extra well-known imports to data and then add a new path
    "$protoc" \
        "--plugin=$plugin" \
        "--proto_path=$dir" \
        "--proto_path=$dest" \
        "--proto_path=$_virtual_imports/timestamp_proto" \
        "--go_out=${grpc},${importmap}:$dest/$dir" \
        "$1"
  )
  tmp=$(mktemp)
  mv "$out" "$tmp"
  cat "$boiler" "$tmp" > "$out"
}

echo -n "Generating protos: " >&2
for p in $(find . -not '(' -path './vendor' -o -path './node_modules' -o -path './external' -prune ')' -name '*.proto'); do
  echo -n "$p "
  genproto "$p"
done
echo

