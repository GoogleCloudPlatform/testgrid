#!/usr/bin/env bash
# Copyright 2023 The TestGrid Authors.
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

WORKDIR="${HOME}/github/testgrid" # The root directory of your repository
PROTO_DEST="${WORKDIR}/web/src/gen"

cd "${WORKDIR}/web"

# See https://github.com/timostamm/protobuf-ts/blob/master/MANUAL.md
npx protoc --ts_out ${PROTO_DEST} --proto_path ${WORKDIR} --ts_opt long_type_string \
  ${WORKDIR}/pb/custom_evaluator/custom_evaluator.proto \
  ${WORKDIR}/pb/state/state.proto \
  ${WORKDIR}/pb/config/config.proto \
  ${WORKDIR}/pb/test_status/test_status.proto \
  ${WORKDIR}/pb/api/v1/data.proto
