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

# TODO(slchase): integrate better with existing bazel script/commands
# note: while the go protoc command runs separately for each file, these must run
#   all together or the imports don't work w.r.t each other

p=$(find . -not '(' -path './node_modules/*' -o -path './web/node_modules/*' -prune ')' -name '*.proto')

echo "FOUND: ${p}"

npx protoc --ts_out "${PWD}/web/src" --proto_path "." ${p}

# yarn run grpc_tools_node_protoc \
#     "--plugin=protoc-gen-ts=./web/node_modules/.bin/protoc-gen-ts" \
#     "--ts_out=grpc:${PWD}/web/src" \
#     ${p} \
#     google/protobuf/timestamp.proto

