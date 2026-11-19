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

load("@bazel_gazelle//:deps.bzl", "go_repository")

def go_repositories():
    go_repository(
        name = "co_honnef_go_tools",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "honnef.co/go/tools",
        sum = "h1:/hemPrYIhOhy8zYrNj+069zDB68us2sMGsfkFJO0iZs=",
        version = "v0.0.0-20190523083050-ea95bdfd59fc",
    )
    go_repository(
        name = "com_github_alecthomas_template",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "github.com/alecthomas/template",
        sum = "h1:JYp7IbQjafoB+tBA3gMyHYHrpOtNuDiK/uB5uXxq5wM=",
        version = "v0.0.0-20190718012654-fb15b899a751",
    )
    go_repository(
        name = "com_github_alecthomas_units",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "github.com/alecthomas/units",
        sum = "h1:UQZhZ2O0vMHr2cI+DC1Mbh0TJxzA3RcLoMsFw+aXw7E=",
        version = "v0.0.0-20190924025748-f65c72e2690d",
    )
    go_repository(
        name = "com_github_armon_go_socks5",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "github.com/armon/go-socks5",
        sum = "h1:0CwZNZbxp69SHPdPJAN/hZIm0C4OItdklCFmMRWYpio=",
        version = "v0.0.0-20160902184237-e75332964ef5",
    )
    go_repository(
        name = "com_github_beorn7_perks",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "github.com/beorn7/perks",
        sum = "h1:VlbKKnNfV8bJzeqoa4cOKqO6bYr3WgKZxO8Z16+hsOM=",
        version = "v1.0.1",
    )
    go_repository(
        name = "com_github_burntsushi_toml",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "github.com/BurntSushi/toml",
        sum = "h1:WXkYYl6Yr3qBf1K79EBnL4mak0OimBfB0XUf9Vl28OQ=",
        version = "v0.3.1",
    )
    go_repository(
        name = "com_github_census_instrumentation_opencensus_proto",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "github.com/census-instrumentation/opencensus-proto",
        sum = "h1:glEXhBS5PSLLv4IXzLA5yPRVX4bilULVyxxbrfOtDAk=",
        version = "v0.2.1",
    )
    go_repository(
        name = "com_github_cespare_xxhash_v2",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "github.com/cespare/xxhash/v2",
        sum = "h1:UL815xU9SqsFlibzuggzjXhog7bL6oX9BbNZnL2UFvs=",
        version = "v2.3.0",
    )
    go_repository(
        name = "com_github_client9_misspell",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "github.com/client9/misspell",
        sum = "h1:ta993UF76GwbvJcIo3Y68y/M3WxlpEHPWIGDkJYwzJI=",
        version = "v0.3.4",
    )
    go_repository(
        name = "com_github_cncf_udpa_go",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "github.com/cncf/udpa/go",
        sum = "h1:WBZRG4aNOuI15bLRrCgN8fCq8E5Xuty6jGbmSNEvSsU=",
        version = "v0.0.0-20191209042840-269d4d468f6f",
    )
    go_repository(
        name = "com_github_cncf_xds_go",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "github.com/cncf/xds/go",
        sum = "h1:aBangftG7EVZoUb69Os8IaYg++6uMOdKK83QtkkvJik=",
        version = "v0.0.0-20260202195803-dba9d589def2",
    )
    go_repository(
        name = "com_github_davecgh_go_spew",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "github.com/davecgh/go-spew",
        sum = "h1:U9qPSI2PIWSS1VwoXQT9A3Wy9MM3WgvqSxFWenqJduM=",
        version = "v1.1.2-0.20180830191138-d8f796af33cc",
    )
    go_repository(
        name = "com_github_envoyproxy_go_control_plane",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "github.com/envoyproxy/go-control-plane",
        sum = "h1:hbG2kr4RuFj222B6+7T83thSPqLjwBIfQawTkC++2HA=",
        version = "v0.14.0",
    )
    go_repository(
        name = "com_github_envoyproxy_go_control_plane_envoy",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "github.com/envoyproxy/go-control-plane/envoy",
        sum = "h1:u3riX6BoYRfF4Dr7dwSOroNfdSbEPe9Yyl09/B6wBrQ=",
        version = "v1.37.0",
    )
    go_repository(
        name = "com_github_envoyproxy_go_control_plane_ratelimit",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "github.com/envoyproxy/go-control-plane/ratelimit",
        sum = "h1:/G9QYbddjL25KvtKTv3an9lx6VBE2cnb8wp1vEGNYGI=",
        version = "v0.1.0",
    )
    go_repository(
        name = "com_github_envoyproxy_protoc_gen_validate",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "github.com/envoyproxy/protoc-gen-validate",
        sum = "h1:MVQghNeW+LZcmXe7SY1V36Z+WFMDjpqGAGacLe2T0ds=",
        version = "v1.3.3",
    )
    go_repository(
        name = "com_github_evanphx_json_patch",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "github.com/evanphx/json-patch",
        sum = "h1:4onqiflcdA9EOZ4RxV643DvftH5pOlLGNtQ5lPWQu84=",
        version = "v4.12.0+incompatible",
    )
    go_repository(
        name = "com_github_felixge_httpsnoop",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "github.com/felixge/httpsnoop",
        sum = "h1:NFTV2Zj1bL4mc9sqWACXbQFVBBg2W3GPvqp8/ESS2Wg=",
        version = "v1.0.4",
    )
    go_repository(
        name = "com_github_fvbommel_sortorder",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "github.com/fvbommel/sortorder",
        sum = "h1:fUmoe+HLsBTctBDoaBwpQo5N+nrCp8g/BjKb/6ZQmYw=",
        version = "v1.1.0",
    )
    go_repository(
        name = "com_github_go_chi_chi",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "github.com/go-chi/chi",
        sum = "h1:QHdzF2szwjqVV4wmByUnTcsbIg7UGaQ0tPF2t5GcAIs=",
        version = "v1.5.4",
    )
    go_repository(
        name = "com_github_go_jose_go_jose_v4",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "github.com/go-jose/go-jose/v4",
        sum = "h1:moDMcTHmvE6Groj34emNPLs/qtYXRVcd6S7NHbHz3kA=",
        version = "v4.1.4",
    )
    go_repository(
        name = "com_github_go_kit_kit",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "github.com/go-kit/kit",
        sum = "h1:wDJmvq38kDhkVxi50ni9ykkdUr1PKgqKOoi01fa0Mdk=",
        version = "v0.9.0",
    )
    go_repository(
        name = "com_github_go_kit_log",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "github.com/go-kit/log",
        sum = "h1:DGJh0Sm43HbOeYDNnVZFl8BvcYVvjD5bqYJvp0REbwQ=",
        version = "v0.1.0",
    )
    go_repository(
        name = "com_github_go_logfmt_logfmt",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "github.com/go-logfmt/logfmt",
        sum = "h1:TrB8swr/68K7m9CcGut2g3UOihhbcbiMAYiuTXdEih4=",
        version = "v0.5.0",
    )
    go_repository(
        name = "com_github_go_logr_logr",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "github.com/go-logr/logr",
        sum = "h1:CjnDlHq8ikf6E492q6eKboGOC0T8CDaOvkHCIg8idEI=",
        version = "v1.4.3",
    )
    go_repository(
        name = "com_github_go_logr_stdr",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "github.com/go-logr/stdr",
        sum = "h1:hSWxHoqTgW2S2qGc0LTAI563KZ5YKYRhT3MFKZMbjag=",
        version = "v1.2.2",
    )
    go_repository(
        name = "com_github_go_openapi_jsonpointer",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "github.com/go-openapi/jsonpointer",
        sum = "h1:eCs3fxoIi3Wh6vtgmLTOjdhSpiqphQ+DaPn38N2ZdrE=",
        version = "v0.19.6",
    )
    go_repository(
        name = "com_github_go_openapi_jsonreference",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "github.com/go-openapi/jsonreference",
        sum = "h1:FBLnyygC4/IZZr893oiomc9XaghoveYTrLC1F86HID8=",
        version = "v0.20.1",
    )
    go_repository(
        name = "com_github_go_openapi_swag",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "github.com/go-openapi/swag",
        sum = "h1:yMBqmnQ0gyZvEb/+KzuWZOXgllrXT4SADYbvDaXHv/g=",
        version = "v0.22.3",
    )
    go_repository(
        name = "com_github_go_stack_stack",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "github.com/go-stack/stack",
        sum = "h1:5SgMzNM5HxrEjV0ww2lTmX6E2Izsfxas4+YHWRs3Lsk=",
        version = "v1.8.0",
    )
    go_repository(
        name = "com_github_go_task_slim_sprig",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "github.com/go-task/slim-sprig",
        sum = "h1:p104kn46Q8WdvHunIJ9dAyjPVtrBPhSr3KT2yUst43I=",
        version = "v0.0.0-20210107165309-348f09dbbbc0",
    )
    go_repository(
        name = "com_github_gogo_protobuf",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "github.com/gogo/protobuf",
        sum = "h1:Ov1cvc58UF3b5XjBnZv7+opcTcQFZebYjWzi34vdm4Q=",
        version = "v1.3.2",
    )
    go_repository(
        name = "com_github_golang_glog",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "github.com/golang/glog",
        sum = "h1:DrW6hGnjIhtvhOIiAKT6Psh/Kd/ldepEa81DKeiRJ5I=",
        version = "v1.2.5",
    )
    go_repository(
        name = "com_github_golang_groupcache",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "github.com/golang/groupcache",
        sum = "h1:oI5xCqsCo564l8iNU+DwB5epxmsaqB+rhGL0m5jtYqE=",
        version = "v0.0.0-20210331224755-41bb18bfe9da",
    )
    go_repository(
        name = "com_github_golang_mock",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "github.com/golang/mock",
        sum = "h1:G5FRp8JnTd7RQH5kemVNlMeyXQAztQ3mOWV95KxsXH8=",
        version = "v1.1.1",
    )
    go_repository(
        name = "com_github_golang_protobuf",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "github.com/golang/protobuf",
        sum = "h1:i7eJL8qZTpSEXOPTxNKhASYpMn+8e5Q6AdndVa1dWek=",
        version = "v1.5.4",
    )
    go_repository(
        name = "com_github_golang_snappy",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "github.com/golang/snappy",
        sum = "h1:yAGX7huGHXlcLOEtBnF4w7FQwA26wojNCwOYAEhLjQM=",
        version = "v0.0.4",
    )
    go_repository(
        name = "com_github_google_gnostic",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "github.com/google/gnostic",
        sum = "h1:FhTMOKj2VhjpouxvWJAV1TL304uMlb9zcDqkl6cEI54=",
        version = "v0.5.7-v3refs",
    )
    go_repository(
        name = "com_github_google_go_cmp",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "github.com/google/go-cmp",
        sum = "h1:wk8382ETsv4JYUZwIsn6YpYiWiBsYLSJiTsyBybVuN8=",
        version = "v0.7.0",
    )
    go_repository(
        name = "com_github_google_go_pkcs11",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "github.com/google/go-pkcs11",
        sum = "h1:PVRnTgtArZ3QQqTGtbtjtnIkzl2iY2kt24yqbrf7td8=",
        version = "v0.3.0",
    )
    go_repository(
        name = "com_github_google_gofuzz",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "github.com/google/gofuzz",
        sum = "h1:xRy4A+RhZaiKjJ1bPfwQ8sedCA+YS2YcCHW6ec7JMi0=",
        version = "v1.2.0",
    )
    go_repository(
        name = "com_github_google_martian_v3",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "github.com/google/martian/v3",
        sum = "h1:DIhPTQrbPkgs2yJYdXU/eNACCG5DVQjySNRNlflZ9Fc=",
        version = "v3.3.3",
    )
    go_repository(
        name = "com_github_google_pprof",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "github.com/google/pprof",
        sum = "h1:K6RDEckDVWvDI9JAJYCmNdQXq6neHJOYx3V6jnqNEec=",
        version = "v0.0.0-20210720184732-4bb14d4b1be1",
    )
    go_repository(
        name = "com_github_google_s2a_go",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "github.com/google/s2a-go",
        sum = "h1:LGD7gtMgezd8a/Xak7mEWL0PjoTQFvpRudN895yqKW0=",
        version = "v0.1.9",
    )
    go_repository(
        name = "com_github_google_uuid",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "github.com/google/uuid",
        sum = "h1:NIvaJDMOsjHA8n1jAhLSgzrAzy1Hgr+hNrb57e+94F0=",
        version = "v1.6.0",
    )
    go_repository(
        name = "com_github_googleapis_enterprise_certificate_proxy",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "github.com/googleapis/enterprise-certificate-proxy",
        sum = "h1:vAe81Msw+8tKUxi2Dqh/NZMz7475yUvmRIkXr4oN2ao=",
        version = "v0.3.11",
    )
    go_repository(
        name = "com_github_googleapis_gax_go_v2",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "github.com/googleapis/gax-go/v2",
        sum = "h1:RksgfBpxqff0EZkDWYuz9q/uWsTVz+kf43LsZ1J6SMc=",
        version = "v2.17.0",
    )
    go_repository(
        name = "com_github_googlecloudplatform_opentelemetry_operations_go_detectors_gcp",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "github.com/GoogleCloudPlatform/opentelemetry-operations-go/detectors/gcp",
        sum = "h1:l7+6kwRMJNwdCvYdDl7Eax+wzEYHSnNY7zrrfbhDdTA=",
        version = "v1.33.0",
    )
    go_repository(
        name = "com_github_googlecloudplatform_opentelemetry_operations_go_exporter_metric",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "github.com/GoogleCloudPlatform/opentelemetry-operations-go/exporter/metric",
        sum = "h1:owcC2UnmsZycprQ5RfRgjydWhuoxg71LUfyiQdijZuM=",
        version = "v0.53.0",
    )
    go_repository(
        name = "com_github_googlecloudplatform_opentelemetry_operations_go_internal_cloudmock",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "github.com/GoogleCloudPlatform/opentelemetry-operations-go/internal/cloudmock",
        sum = "h1:4LP6hvB4I5ouTbGgWtixJhgED6xdf67twf9PoY96Tbg=",
        version = "v0.53.0",
    )
    go_repository(
        name = "com_github_googlecloudplatform_opentelemetry_operations_go_internal_resourcemapping",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "github.com/GoogleCloudPlatform/opentelemetry-operations-go/internal/resourcemapping",
        sum = "h1:Ron4zCA/yk6U7WOBXhTJcDpsUBG9npumK6xw2auFltQ=",
        version = "v0.53.0",
    )
    go_repository(
        name = "com_github_hashicorp_errwrap",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "github.com/hashicorp/errwrap",
        sum = "h1:OxrOeh75EUXMY8TBjag2fzXGZ40LB6IKw45YeGUDY2I=",
        version = "v1.1.0",
    )
    go_repository(
        name = "com_github_hashicorp_go_multierror",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "github.com/hashicorp/go-multierror",
        sum = "h1:H5DkEtf6CXdFp0N0Em5UCwQpXMWke8IA0+lD48awMYo=",
        version = "v1.1.1",
    )
    go_repository(
        name = "com_github_iancoleman_strcase",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "github.com/iancoleman/strcase",
        sum = "h1:nTXanmYxhfFAMjZL34Ov6gkzEsSJZ5DbhxWjvSASxEI=",
        version = "v0.3.0",
    )
    go_repository(
        name = "com_github_josharian_intern",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "github.com/josharian/intern",
        sum = "h1:vlS4z54oSdjm0bgjRigI+G1HpF+tI+9rE5LLzOg8HmY=",
        version = "v1.0.0",
    )
    go_repository(
        name = "com_github_jpillora_backoff",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "github.com/jpillora/backoff",
        sum = "h1:uvFg412JmmHBHw7iwprIxkPMI+sGQ4kzOWsMeHnm2EA=",
        version = "v1.0.0",
    )
    go_repository(
        name = "com_github_json_iterator_go",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "github.com/json-iterator/go",
        sum = "h1:PV8peI4a0ysnczrg+LtxykD8LfKY9ML6u2jnxaEnrnM=",
        version = "v1.1.12",
    )
    go_repository(
        name = "com_github_julienschmidt_httprouter",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "github.com/julienschmidt/httprouter",
        sum = "h1:U0609e9tgbseu3rBINet9P48AI/D3oJs4dN7jwJOQ1U=",
        version = "v1.3.0",
    )
    go_repository(
        name = "com_github_kisielk_errcheck",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "github.com/kisielk/errcheck",
        sum = "h1:e8esj/e4R+SAOwFwN+n3zr0nYeCyeweozKfO23MvHzY=",
        version = "v1.5.0",
    )
    go_repository(
        name = "com_github_kisielk_gotool",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "github.com/kisielk/gotool",
        sum = "h1:AV2c/EiW3KqPNT9ZKl07ehoAGi4C5/01Cfbblndcapg=",
        version = "v1.0.0",
    )
    go_repository(
        name = "com_github_konsorten_go_windows_terminal_sequences",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "github.com/konsorten/go-windows-terminal-sequences",
        sum = "h1:CE8S1cTafDpPvMhIxNJKvHsGVBgn1xWYf1NbHQhywc8=",
        version = "v1.0.3",
    )
    go_repository(
        name = "com_github_kr_logfmt",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "github.com/kr/logfmt",
        sum = "h1:T+h1c/A9Gawja4Y9mFVWj2vyii2bbUNDw3kt9VxK2EY=",
        version = "v0.0.0-20140226030751-b84e30acd515",
    )
    go_repository(
        name = "com_github_kr_pretty",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "github.com/kr/pretty",
        sum = "h1:flRD4NNwYAUpkphVc1HcthR4KEIFJ65n8Mw5qdRn3LE=",
        version = "v0.3.1",
    )
    go_repository(
        name = "com_github_kr_pty",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "github.com/kr/pty",
        sum = "h1:VkoXIwSboBpnk99O/KFauAEILuNHv5DVFKZMBN/gUgw=",
        version = "v1.1.1",
    )
    go_repository(
        name = "com_github_kr_text",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "github.com/kr/text",
        sum = "h1:5Nx0Ya0ZqY2ygV366QzturHI13Jq95ApcVaJBhpS+AY=",
        version = "v0.2.0",
    )
    go_repository(
        name = "com_github_lyft_protoc_gen_star_v2",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "github.com/lyft/protoc-gen-star/v2",
        sum = "h1:JDlNKttNIRd68AAIychs0AqEpO8/I/WYi01OQ7Raw6Q=",
        version = "v2.0.4",
    )
    go_repository(
        name = "com_github_mailru_easyjson",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "github.com/mailru/easyjson",
        sum = "h1:UGYAvKxe3sBsEDzO8ZeWOSlIQfWFlxbzLZe7hwFURr0=",
        version = "v0.7.7",
    )
    go_repository(
        name = "com_github_matttproud_golang_protobuf_extensions",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "github.com/matttproud/golang_protobuf_extensions",
        sum = "h1:4hp9jkHxhMHkqkrB3Ix0jegS5sx/RkqARlsWZ6pIwiU=",
        version = "v1.0.1",
    )
    go_repository(
        name = "com_github_microsoft_go_winio",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "github.com/Microsoft/go-winio",
        sum = "h1:F2VQgta7ecxGYO8k3ZZz3RS8fVIXVxONVUPlNERoyfY=",
        version = "v0.6.2",
    )
    go_repository(
        name = "com_github_moby_spdystream",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "github.com/moby/spdystream",
        sum = "h1:cjW1zVyyoiM0T7b6UoySUFqzXMoqRckQtXwGPiBhOM8=",
        version = "v0.2.0",
    )
    go_repository(
        name = "com_github_modern_go_concurrent",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "github.com/modern-go/concurrent",
        sum = "h1:TRLaZ9cD/w8PVh93nsPXa1VrQ6jlwL5oN8l14QlcNfg=",
        version = "v0.0.0-20180306012644-bacd9c7ef1dd",
    )
    go_repository(
        name = "com_github_modern_go_reflect2",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "github.com/modern-go/reflect2",
        sum = "h1:xBagoLtFs94CBntxluKeaWgTMpvLxC4ur3nMaC9Gz0M=",
        version = "v1.0.2",
    )
    go_repository(
        name = "com_github_mwitkow_go_conntrack",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "github.com/mwitkow/go-conntrack",
        sum = "h1:KUppIJq7/+SVif2QVs3tOP0zanoHgBEVAwHxUSIzRqU=",
        version = "v0.0.0-20190716064945-2f068394615f",
    )
    go_repository(
        name = "com_github_mxk_go_flowrate",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "github.com/mxk/go-flowrate",
        sum = "h1:y5//uYreIhSUg3J1GEMiLbxo1LJaP8RfCpH6pymGZus=",
        version = "v0.0.0-20140419014527-cca7078d478f",
    )
    go_repository(
        name = "com_github_onsi_ginkgo_v2",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "github.com/onsi/ginkgo/v2",
        sum = "h1:zie5Ly042PD3bsCvsSOPvRnFwyo3rKe64TJlD6nu0mk=",
        version = "v2.9.1",
    )
    go_repository(
        name = "com_github_onsi_gomega",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "github.com/onsi/gomega",
        sum = "h1:Z2AnStgsdSayCMDiCU42qIz+HLqEPcgiOCXjAU/w+8E=",
        version = "v1.27.4",
    )
    go_repository(
        name = "com_github_pkg_errors",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "github.com/pkg/errors",
        sum = "h1:FEBLx1zS214owpjy7qsBeixbURkuhQAwrK5UwLGTwt4=",
        version = "v0.9.1",
    )
    go_repository(
        name = "com_github_planetscale_vtprotobuf",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "github.com/planetscale/vtprotobuf",
        sum = "h1:GFCKgmp0tecUJ0sJuv4pzYCqS9+RGSn52M3FUwPs+uo=",
        version = "v0.6.1-0.20240319094008-0393e58bdf10",
    )
    go_repository(
        name = "com_github_pmezard_go_difflib",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "github.com/pmezard/go-difflib",
        sum = "h1:Jamvg5psRIccs7FGNTlIRMkT8wgtp5eCXdBlqhYGL6U=",
        version = "v1.0.1-0.20181226105442-5d4384ee4fb2",
    )
    go_repository(
        name = "com_github_prometheus_client_golang",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "github.com/prometheus/client_golang",
        sum = "h1:+4eQaD7vAZ6DsfsxB15hbE0odUjGI5ARs9yskGu1v4s=",
        version = "v1.11.1",
    )
    go_repository(
        name = "com_github_prometheus_client_model",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "github.com/prometheus/client_model",
        sum = "h1:oBsgwpGs7iVziMvrGhE53c/GrLUsZdHnqNwqPLxwZyk=",
        version = "v0.6.2",
    )
    go_repository(
        name = "com_github_prometheus_common",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "github.com/prometheus/common",
        sum = "h1:iMAkS2TDoNWnKM+Kopnx/8tnEStIfpYA0ur0xQzzhMQ=",
        version = "v0.26.0",
    )
    go_repository(
        name = "com_github_prometheus_procfs",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "github.com/prometheus/procfs",
        sum = "h1:mxy4L2jP6qMonqmq+aTtOx1ifVWUgG/TAmntgbh3xv4=",
        version = "v0.6.0",
    )
    go_repository(
        name = "com_github_rogpeppe_go_internal",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "github.com/rogpeppe/go-internal",
        sum = "h1:UQB4HGPB6osV0SQTLymcB4TgvyWu6ZyliaW0tI/otEQ=",
        version = "v1.14.1",
    )
    go_repository(
        name = "com_github_sethvargo_go_retry",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "github.com/sethvargo/go-retry",
        sum = "h1:T+jHEQy/zKJf5s95UkguisicE0zuF9y7+/vgz08Ocec=",
        version = "v0.2.4",
    )
    go_repository(
        name = "com_github_sirupsen_logrus",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "github.com/sirupsen/logrus",
        sum = "h1:dueUQJ1C2q9oE3F7wvmSGAaVtTmUizReu6fjN8uqzbQ=",
        version = "v1.9.3",
    )
    go_repository(
        name = "com_github_spf13_afero",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "github.com/spf13/afero",
        sum = "h1:b/YBCLWAJdFWJTN9cLhiXXcD7mzKn9Dm86dNnfyQw1I=",
        version = "v1.15.0",
    )
    go_repository(
        name = "com_github_spf13_pflag",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "github.com/spf13/pflag",
        sum = "h1:iy+VFUOCP1a+8yFto/drg2CJ5u0yRoB7fZw3DKv/JXA=",
        version = "v1.0.5",
    )
    go_repository(
        name = "com_github_spiffe_go_spiffe_v2",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "github.com/spiffe/go-spiffe/v2",
        sum = "h1:uXe1MflJoHw58wAUvxVlcM7WpKtijWG7I1UidcGh6g4=",
        version = "v2.7.0",
    )
    go_repository(
        name = "com_github_stretchr_objx",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "github.com/stretchr/objx",
        sum = "h1:1zr/of2m5FGMsad5YfcqgdqdWrIhu+EBEJRhR1U7z/c=",
        version = "v0.5.0",
    )
    go_repository(
        name = "com_github_stretchr_testify",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "github.com/stretchr/testify",
        sum = "h1:7s2iGBzp5EwR7/aIZr8ao5+dra3wiQyKjjFuvgVKu7U=",
        version = "v1.11.1",
    )
    go_repository(
        name = "com_github_yuin_goldmark",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "github.com/yuin/goldmark",
        sum = "h1:ruQGxdhGHe7FWOJPT0mKs5+pD2Xs1Bm/kdGlHO04FmM=",
        version = "v1.2.1",
    )
    go_repository(
        name = "com_github_zeebo_errs",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "github.com/zeebo/errs",
        sum = "h1:XNdoD/RRMKP7HD0UhJnIzUy74ISdGGxURlYG8HSWSfM=",
        version = "v1.4.0",
    )
    go_repository(
        name = "com_google_cloud_go",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "cloud.google.com/go",
        sum = "h1:2NAUJwPR47q+E35uaJeYoNhuNEM9kM8SjgRgdeOJUSE=",
        version = "v0.123.0",
    )
    go_repository(
        name = "com_google_cloud_go_accessapproval",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "cloud.google.com/go/accessapproval",
        sum = "h1:gq8OS+rQWgGRo91D2qztN+ion6AZ2T1CxBIu0ifCmVo=",
        version = "v1.8.8",
    )
    go_repository(
        name = "com_google_cloud_go_accesscontextmanager",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "cloud.google.com/go/accesscontextmanager",
        sum = "h1:aKIfg7Jyc73pe8bzx0zypNdS5gfFdSvFvB8YNA9k2kA=",
        version = "v1.9.7",
    )
    go_repository(
        name = "com_google_cloud_go_aiplatform",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "cloud.google.com/go/aiplatform",
        sum = "h1:TCrSLci+NFEAx0PZMv8btGe5j68RivArmDJbBLIc/3o=",
        version = "v1.114.0",
    )
    go_repository(
        name = "com_google_cloud_go_analytics",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "cloud.google.com/go/analytics",
        sum = "h1:souLxu9tQHzF+0NDpKoIw4pl2WQ9K2JfkdPPs36BfXw=",
        version = "v0.30.1",
    )
    go_repository(
        name = "com_google_cloud_go_apigateway",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "cloud.google.com/go/apigateway",
        sum = "h1:ehKUTy+QFsb3n07fEi18S2dpDDjCV4UlRyrbwfZV3Zk=",
        version = "v1.7.7",
    )
    go_repository(
        name = "com_google_cloud_go_apigeeconnect",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "cloud.google.com/go/apigeeconnect",
        sum = "h1:S6s2zojwMymx0fyZYKm0eK1TdDxrriIBAlNVvRAOzug=",
        version = "v1.7.7",
    )
    go_repository(
        name = "com_google_cloud_go_apigeeregistry",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "cloud.google.com/go/apigeeregistry",
        sum = "h1:QziFVsuPU2lhy40Ht9uWEyciV23SH9GETWiwcu3qzdg=",
        version = "v0.10.0",
    )
    go_repository(
        name = "com_google_cloud_go_appengine",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "cloud.google.com/go/appengine",
        sum = "h1:IxGz6j5xv0nTJX285wu95Vn6KEi2CeV9vbyRgCSEAoU=",
        version = "v1.9.7",
    )
    go_repository(
        name = "com_google_cloud_go_area120",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "cloud.google.com/go/area120",
        sum = "h1:BbpzLwaIXVPorrrzTH+ni7P5mLemmPPfSZ7o39k7zQc=",
        version = "v0.9.7",
    )
    go_repository(
        name = "com_google_cloud_go_artifactregistry",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "cloud.google.com/go/artifactregistry",
        sum = "h1:DaOHWeURq93K27/6Sa2fy3rJoftrVXKeT3tonM4fxtI=",
        version = "v1.19.0",
    )
    go_repository(
        name = "com_google_cloud_go_asset",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "cloud.google.com/go/asset",
        sum = "h1:81Ru5hjHfiGtk+u/Ix69eaWieKpvm7Ce7UHtcZhOLbk=",
        version = "v1.22.0",
    )
    go_repository(
        name = "com_google_cloud_go_assuredworkloads",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "cloud.google.com/go/assuredworkloads",
        sum = "h1:NQXyyGLksPmiapE1Oc64a3cMwYIBAoDBg6cWR+B3eaY=",
        version = "v1.13.0",
    )
    go_repository(
        name = "com_google_cloud_go_auth",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "cloud.google.com/go/auth",
        sum = "h1:+Nbt5Ev0xEqxlNjd6c+yYUeosQ5TtEUaNcN/3FozlaM=",
        version = "v0.18.2",
    )
    go_repository(
        name = "com_google_cloud_go_auth_oauth2adapt",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "cloud.google.com/go/auth/oauth2adapt",
        sum = "h1:keo8NaayQZ6wimpNSmW5OPc283g65QNIiLpZnkHRbnc=",
        version = "v0.2.8",
    )
    go_repository(
        name = "com_google_cloud_go_automl",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "cloud.google.com/go/automl",
        sum = "h1:YRwLbsBv4yApX64pkrdyy4emhWE6lHEnljX4b1aTQC4=",
        version = "v1.15.0",
    )
    go_repository(
        name = "com_google_cloud_go_baremetalsolution",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "cloud.google.com/go/baremetalsolution",
        sum = "h1:g67fjVdrNCHZl8jDWdZvo+6zGTTMMuvNWO7HSgG8lnI=",
        version = "v1.4.0",
    )
    go_repository(
        name = "com_google_cloud_go_batch",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "cloud.google.com/go/batch",
        sum = "h1:r5DEMPNXZk1as36Le3DaNQTRhhnR+E95a99SFxwF52o=",
        version = "v1.14.0",
    )
    go_repository(
        name = "com_google_cloud_go_beyondcorp",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "cloud.google.com/go/beyondcorp",
        sum = "h1:mre997ya7QHFWSU+O5cT/FhBKTMy6Riqf1EXFxN46zw=",
        version = "v1.2.0",
    )
    go_repository(
        name = "com_google_cloud_go_bigquery",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "cloud.google.com/go/bigquery",
        sum = "h1:D/yLju+3Ens2IXx7ou1DJ62juBm+/coBInn4VVOg5Cw=",
        version = "v1.72.0",
    )
    go_repository(
        name = "com_google_cloud_go_bigtable",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "cloud.google.com/go/bigtable",
        sum = "h1:99KOWShm/MUyuIbXBeVscdWJFV7GdgiYwFUrB5Iu4BI=",
        version = "v1.41.0",
    )
    go_repository(
        name = "com_google_cloud_go_billing",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "cloud.google.com/go/billing",
        sum = "h1:nbQjTXkpgB/E4XnYZQwcZnR63QFsbFwJ9DGsNg61Ghg=",
        version = "v1.21.0",
    )
    go_repository(
        name = "com_google_cloud_go_binaryauthorization",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "cloud.google.com/go/binaryauthorization",
        sum = "h1:YYK0BwiZv9uA6z+Ict908AykX4OBfDECMTE476OnS3A=",
        version = "v1.10.0",
    )
    go_repository(
        name = "com_google_cloud_go_certificatemanager",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "cloud.google.com/go/certificatemanager",
        sum = "h1:v5X8X+THKrS9OFZb6k0GRDP1WQxLXTdMko7OInBliw4=",
        version = "v1.9.6",
    )
    go_repository(
        name = "com_google_cloud_go_channel",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "cloud.google.com/go/channel",
        sum = "h1:ThoAmHBd9WkX2SSuF6n6uEOvbBNoTuhBT7Rk6bFS5ho=",
        version = "v1.21.0",
    )
    go_repository(
        name = "com_google_cloud_go_cloudbuild",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "cloud.google.com/go/cloudbuild",
        sum = "h1:Fkg+iJdN7bfICZJzLr/XV+k9aVxXS/hakIlhjDIRIDw=",
        version = "v1.25.0",
    )
    go_repository(
        name = "com_google_cloud_go_clouddms",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "cloud.google.com/go/clouddms",
        sum = "h1:YWsmRXTyK6Ba0hm4qTBak5g1oLhryuM8rSBxHWC8iq4=",
        version = "v1.8.8",
    )
    go_repository(
        name = "com_google_cloud_go_cloudtasks",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "cloud.google.com/go/cloudtasks",
        sum = "h1:H2v8GEolNtMFfYzUpZBaZbydqU7drpyo99GtAgA+m4I=",
        version = "v1.13.7",
    )
    go_repository(
        name = "com_google_cloud_go_compute",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "cloud.google.com/go/compute",
        sum = "h1:4CKmnpO+40z44bKG5bdcKxQ7ocNpRtOc9SCLLUzze1w=",
        version = "v1.54.0",
    )
    go_repository(
        name = "com_google_cloud_go_compute_metadata",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "cloud.google.com/go/compute/metadata",
        sum = "h1:pDUj4QMoPejqq20dK0Pg2N4yG9zIkYGdBtwLoEkH9Zs=",
        version = "v0.9.0",
    )
    go_repository(
        name = "com_google_cloud_go_contactcenterinsights",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "cloud.google.com/go/contactcenterinsights",
        sum = "h1:wA4j99BhsoeYlLx6xEIqrNN1aOTtUme0wimHZegg80s=",
        version = "v1.17.4",
    )
    go_repository(
        name = "com_google_cloud_go_container",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "cloud.google.com/go/container",
        sum = "h1:i1No5obpPxlIFLGHdUF6h2YjRR1qN9t/ZkA8KA5B//o=",
        version = "v1.45.0",
    )
    go_repository(
        name = "com_google_cloud_go_containeranalysis",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "cloud.google.com/go/containeranalysis",
        sum = "h1:OW2dlMPtR5VnjQGyAP+uJlZahc1l+JFxFlH/J3+l7gw=",
        version = "v0.14.2",
    )
    go_repository(
        name = "com_google_cloud_go_datacatalog",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "cloud.google.com/go/datacatalog",
        sum = "h1:bCRKA8uSQN8wGW3Tw0gwko4E9a64GRmbW1nCblhgC2k=",
        version = "v1.26.1",
    )
    go_repository(
        name = "com_google_cloud_go_dataflow",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "cloud.google.com/go/dataflow",
        sum = "h1:Z+UYlGrE+IoB+5IAN4/qWdPKO0IpIK9bs2Dy40HK6lg=",
        version = "v0.11.1",
    )
    go_repository(
        name = "com_google_cloud_go_dataform",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "cloud.google.com/go/dataform",
        sum = "h1:yf6Up6m1FUt+YB5CBgNtIZfz2OzjuNBdzZWV3SLSVNE=",
        version = "v0.12.1",
    )
    go_repository(
        name = "com_google_cloud_go_datafusion",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "cloud.google.com/go/datafusion",
        sum = "h1:tLCV+xYuOrSjdrRTkc9Cqsb5mBSQEsNfFmuTNYl5/rA=",
        version = "v1.8.7",
    )
    go_repository(
        name = "com_google_cloud_go_datalabeling",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "cloud.google.com/go/datalabeling",
        sum = "h1:wwoct7mw38s75XvEmLoItQ2TY0RFsGiRDb0iNbXUcX4=",
        version = "v0.9.7",
    )
    go_repository(
        name = "com_google_cloud_go_dataplex",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "cloud.google.com/go/dataplex",
        sum = "h1:rROI3iqMVI9nXT701ULoFRETQVAOAPC3mPSWFDxXFl0=",
        version = "v1.28.0",
    )
    go_repository(
        name = "com_google_cloud_go_dataproc_v2",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "cloud.google.com/go/dataproc/v2",
        sum = "h1:I/Yux/d8uaxf3W+d59kolGTOc52+VZaL6RzJw7oDOeg=",
        version = "v2.15.0",
    )
    go_repository(
        name = "com_google_cloud_go_dataqna",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "cloud.google.com/go/dataqna",
        sum = "h1:3FREvU+sjaEHSjlKrKF6KjUmafdOvM8CbZ897rttxNs=",
        version = "v0.9.8",
    )
    go_repository(
        name = "com_google_cloud_go_datastore",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "cloud.google.com/go/datastore",
        sum = "h1:dUrYq47ysCA4nM7u8kRT0WnbfXc6TzX49cP3TCwIiA0=",
        version = "v1.21.0",
    )
    go_repository(
        name = "com_google_cloud_go_datastream",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "cloud.google.com/go/datastream",
        sum = "h1:7PKeDpksi8nbOR4gspmNokzsr0q/uRzDIt20bR3BtRs=",
        version = "v1.15.1",
    )
    go_repository(
        name = "com_google_cloud_go_deploy",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "cloud.google.com/go/deploy",
        sum = "h1:QU8gLXsXDRqLyEWNrI6zJiVzuuOBX/WpMi4p0oexV+c=",
        version = "v1.27.3",
    )
    go_repository(
        name = "com_google_cloud_go_dialogflow",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "cloud.google.com/go/dialogflow",
        sum = "h1:YtSrYLd6CpRS1NZ67r4Drvm1gafUEGDm/8vVBRYEv+I=",
        version = "v1.74.0",
    )
    go_repository(
        name = "com_google_cloud_go_dlp",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "cloud.google.com/go/dlp",
        sum = "h1:+aMQYODOxCCZHpdzKvv/rIc9CbKd6XVmjVBRjaF8UvQ=",
        version = "v1.28.0",
    )
    go_repository(
        name = "com_google_cloud_go_documentai",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "cloud.google.com/go/documentai",
        sum = "h1:ngPNmVUb3xxj6/j7ruK4vmOMm+XTJO0Vi7Jkm0R3AdU=",
        version = "v1.39.0",
    )
    go_repository(
        name = "com_google_cloud_go_domains",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "cloud.google.com/go/domains",
        sum = "h1:G3kUq0vKBMhyOj5GqAfEYbVuez05U+ENHZUAtrEp/pI=",
        version = "v0.10.7",
    )
    go_repository(
        name = "com_google_cloud_go_edgecontainer",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "cloud.google.com/go/edgecontainer",
        sum = "h1:6KTQo6Qf0iEtfPVotlG7orazEO1I93Ham0PMlkHYpdQ=",
        version = "v1.4.4",
    )
    go_repository(
        name = "com_google_cloud_go_errorreporting",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "cloud.google.com/go/errorreporting",
        sum = "h1:uLcasn2hKpj6iSPvHrzRjkJcaNVaKx8yKQcP3VTS6aI=",
        version = "v0.4.0",
    )
    go_repository(
        name = "com_google_cloud_go_essentialcontacts",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "cloud.google.com/go/essentialcontacts",
        sum = "h1:v9sO4IHFuwplaOuDnEXZFtfOrjw2bi11TSIVp5PnAU4=",
        version = "v1.7.7",
    )
    go_repository(
        name = "com_google_cloud_go_eventarc",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "cloud.google.com/go/eventarc",
        sum = "h1:8WWG1/ogInYur1NQjML6EMHQ0ZBzAdMDGlUVpLD56cI=",
        version = "v1.18.0",
    )
    go_repository(
        name = "com_google_cloud_go_filestore",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "cloud.google.com/go/filestore",
        sum = "h1:3KZifUVTqGhNNv6MLeONYth1HjlVM4vDhaH+xrdPljU=",
        version = "v1.10.3",
    )
    go_repository(
        name = "com_google_cloud_go_firestore",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "cloud.google.com/go/firestore",
        sum = "h1:BhopUsx7kh6NFx77ccRsHhrtkbJUmDAxNY3uapWdjcM=",
        version = "v1.21.0",
    )
    go_repository(
        name = "com_google_cloud_go_functions",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "cloud.google.com/go/functions",
        sum = "h1:7LcOD18euIVGRUPaeCmgO6vfWSLNIsi6STWRQcdANG8=",
        version = "v1.19.7",
    )
    go_repository(
        name = "com_google_cloud_go_gkebackup",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "cloud.google.com/go/gkebackup",
        sum = "h1:gUgI3lZJYALZsHXE7YJOKI8bMpoAX/tF6jnNugvzT1g=",
        version = "v1.8.1",
    )
    go_repository(
        name = "com_google_cloud_go_gkeconnect",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "cloud.google.com/go/gkeconnect",
        sum = "h1:EFql3zRaFw74yATt5lf+mcPDqPZ4EeLvoIJ+0NaEkag=",
        version = "v0.12.5",
    )
    go_repository(
        name = "com_google_cloud_go_gkehub",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "cloud.google.com/go/gkehub",
        sum = "h1:Jk5pAXG54FlQzTRXhuKyym/NzOgS8oWRs0XNatZYDf4=",
        version = "v0.16.0",
    )
    go_repository(
        name = "com_google_cloud_go_gkemulticloud",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "cloud.google.com/go/gkemulticloud",
        sum = "h1:m0FX9o7t7xVmSZhqzm/m8nEZn8LnC5Kh60Wg4Yx1lyQ=",
        version = "v1.6.0",
    )
    go_repository(
        name = "com_google_cloud_go_gsuiteaddons",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "cloud.google.com/go/gsuiteaddons",
        sum = "h1:Dayrv57XW8kZIvmQjAc89Tp7Kr3O9Am/hf6pXkTjYFY=",
        version = "v1.7.8",
    )
    go_repository(
        name = "com_google_cloud_go_iam",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "cloud.google.com/go/iam",
        sum = "h1:+vMINPiDF2ognBJ97ABAYYwRgsaqxPbQDlMnbHMjolc=",
        version = "v1.5.3",
    )
    go_repository(
        name = "com_google_cloud_go_iap",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "cloud.google.com/go/iap",
        sum = "h1:Nheb77nO0/pECm/thoE3wHVAbkQSI+G8KBWviqBepiA=",
        version = "v1.11.3",
    )
    go_repository(
        name = "com_google_cloud_go_ids",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "cloud.google.com/go/ids",
        sum = "h1:V0pSk+KKW+5/AVpeQMhM9D1VI7aMZkayj5jddNETJos=",
        version = "v1.5.7",
    )
    go_repository(
        name = "com_google_cloud_go_iot",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "cloud.google.com/go/iot",
        sum = "h1:PDUtxCzlFwFHODEFAgaGJy/Zv4tdvLbZ+lvZ1mKQXE4=",
        version = "v1.8.7",
    )
    go_repository(
        name = "com_google_cloud_go_kms",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "cloud.google.com/go/kms",
        sum = "h1:gVqvGGUmz0nYCmtoxWmdc1wli2L1apgP8U4fghPGSbQ=",
        version = "v1.25.0",
    )
    go_repository(
        name = "com_google_cloud_go_language",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "cloud.google.com/go/language",
        sum = "h1:/0Fbd3/T4oNmpPqIq5/hrWdHc/eoYGtVH5lDNkuHH3k=",
        version = "v1.14.6",
    )
    go_repository(
        name = "com_google_cloud_go_lifesciences",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "cloud.google.com/go/lifesciences",
        sum = "h1:MO5aBahcYv7JeuCpHbg/11h7KL/BYt1+PpgHhleLDbI=",
        version = "v0.10.7",
    )
    go_repository(
        name = "com_google_cloud_go_logging",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "cloud.google.com/go/logging",
        sum = "h1:O7LvmO0kGLaHY/gq8cV7T0dyp6zJhYAOtZPX4TF3QtY=",
        version = "v1.13.1",
    )
    go_repository(
        name = "com_google_cloud_go_longrunning",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "cloud.google.com/go/longrunning",
        sum = "h1:LiKK77J3bx5gDLi4SMViHixjD2ohlkwBi+mKA7EhfW8=",
        version = "v0.8.0",
    )
    go_repository(
        name = "com_google_cloud_go_managedidentities",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "cloud.google.com/go/managedidentities",
        sum = "h1:vC/q7D+97PZfb0UNf7r/+/clHauuaf1PqWwP7neuaeg=",
        version = "v1.7.7",
    )
    go_repository(
        name = "com_google_cloud_go_maps",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "cloud.google.com/go/maps",
        sum = "h1:tcdo9oB3Ap4N9JJJFOhxRFldKUok4Mesd3ta7Rm79r0=",
        version = "v1.26.0",
    )
    go_repository(
        name = "com_google_cloud_go_mediatranslation",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "cloud.google.com/go/mediatranslation",
        sum = "h1:JXbjms+JxgaWkj/YuaQm1OeCzuF+IZCDV17uUcZgFOU=",
        version = "v0.9.7",
    )
    go_repository(
        name = "com_google_cloud_go_memcache",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "cloud.google.com/go/memcache",
        sum = "h1:ZDIfIMZsKKPzwdbvTMOL1il0shX24J7B9DC+sEt4Yj4=",
        version = "v1.11.7",
    )
    go_repository(
        name = "com_google_cloud_go_metastore",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "cloud.google.com/go/metastore",
        sum = "h1:nfyUDD9AeKIs6btY5buQ1No0OVco20WpX9wIruL8UOA=",
        version = "v1.14.8",
    )
    go_repository(
        name = "com_google_cloud_go_monitoring",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "cloud.google.com/go/monitoring",
        sum = "h1:dde+gMNc0UhPZD1Azu6at2e79bfdztVDS5lvhOdsgaE=",
        version = "v1.24.3",
    )
    go_repository(
        name = "com_google_cloud_go_networkconnectivity",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "cloud.google.com/go/networkconnectivity",
        sum = "h1:A0uRcZJdq7F6LYWcc2NIea3h0i7p6kY1/CyLavOTG0I=",
        version = "v1.20.0",
    )
    go_repository(
        name = "com_google_cloud_go_networkmanagement",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "cloud.google.com/go/networkmanagement",
        sum = "h1:041d0RqmwP/H7AWkF/AoAwFvuCkz8shUMA4EoQt0lOA=",
        version = "v1.21.0",
    )
    go_repository(
        name = "com_google_cloud_go_networksecurity",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "cloud.google.com/go/networksecurity",
        sum = "h1:+ahtCqEqwHw3a3UIeG21vT817xt9kkDDAO6k9+LCc18=",
        version = "v0.11.0",
    )
    go_repository(
        name = "com_google_cloud_go_notebooks",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "cloud.google.com/go/notebooks",
        sum = "h1:g5LTI1LHa/86abDTWd8nrq7/4qq8oFhVx1SmnNpZLVg=",
        version = "v1.12.7",
    )
    go_repository(
        name = "com_google_cloud_go_optimization",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "cloud.google.com/go/optimization",
        sum = "h1:dMtxINB6G7wULbdm8nZ/x1NMa579Q+GfJc5gaN8VeDw=",
        version = "v1.7.7",
    )
    go_repository(
        name = "com_google_cloud_go_orchestration",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "cloud.google.com/go/orchestration",
        sum = "h1:TVWDiZyvcflLFeTQH2GexHmtJ6iUSjzr0zsSiT338dA=",
        version = "v1.11.10",
    )
    go_repository(
        name = "com_google_cloud_go_orgpolicy",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "cloud.google.com/go/orgpolicy",
        sum = "h1:0hq12wxNwcfUMojr5j3EjWECSInIuyYDhkAWXTomRhc=",
        version = "v1.15.1",
    )
    go_repository(
        name = "com_google_cloud_go_osconfig",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "cloud.google.com/go/osconfig",
        sum = "h1:QQzK5njfsfO2rdOWYVDyLQktqSq9gKf2ohRYeKUuA10=",
        version = "v1.15.1",
    )
    go_repository(
        name = "com_google_cloud_go_oslogin",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "cloud.google.com/go/oslogin",
        sum = "h1:YQ8P/+MLwH0tpENYU9QOgwKQxe8DYfAKxIfm6y+OBtA=",
        version = "v1.14.7",
    )
    go_repository(
        name = "com_google_cloud_go_phishingprotection",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "cloud.google.com/go/phishingprotection",
        sum = "h1:ZJqHirY2/H6s+uTq1y1iiVASzm3ZuDiMglT5NXywPBE=",
        version = "v0.9.7",
    )
    go_repository(
        name = "com_google_cloud_go_policytroubleshooter",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "cloud.google.com/go/policytroubleshooter",
        sum = "h1:Bbj1EiVh96u9mfO2p+JNoHrvvyC0Ms6zP+vxqQnsaG8=",
        version = "v1.11.7",
    )
    go_repository(
        name = "com_google_cloud_go_privatecatalog",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "cloud.google.com/go/privatecatalog",
        sum = "h1:yOdy85WDvSCPxAMixkhs5X0Z96D74kosgOTp7aJEYvU=",
        version = "v0.10.8",
    )
    go_repository(
        name = "com_google_cloud_go_pubsub",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "cloud.google.com/go/pubsub",
        sum = "h1:fzbXpPyJnSGvWXF1jabhQeXyxdbCIkXTpjXHy7xviBM=",
        version = "v1.50.1",
    )
    go_repository(
        name = "com_google_cloud_go_pubsub_v2",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "cloud.google.com/go/pubsub/v2",
        sum = "h1:0qS6mRJ41gD1lNmM/vdm6bR7DQu6coQcVwD+VPf0Bz0=",
        version = "v2.0.0",
    )
    go_repository(
        name = "com_google_cloud_go_pubsublite",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "cloud.google.com/go/pubsublite",
        sum = "h1:jLQozsEVr+c6tOU13vDugtnaBSUy/PD5zK6mhm+uF1Y=",
        version = "v1.8.2",
    )
    go_repository(
        name = "com_google_cloud_go_recaptchaenterprise_v2",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "cloud.google.com/go/recaptchaenterprise/v2",
        sum = "h1:zHaPdgmV3LmzaUfn9Xiiqp5zE1Y16f0O8XCwERrAs2E=",
        version = "v2.21.0",
    )
    go_repository(
        name = "com_google_cloud_go_recommendationengine",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "cloud.google.com/go/recommendationengine",
        sum = "h1:NH89CyKQP8e98kpdKLwV0jXkQGzSEEZia0V867vkoy8=",
        version = "v0.9.7",
    )
    go_repository(
        name = "com_google_cloud_go_recommender",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "cloud.google.com/go/recommender",
        sum = "h1:ZVZg4wr1G7yzjIPcYUNSUJAaz9+2o78rmBU4QJgC7kg=",
        version = "v1.13.6",
    )
    go_repository(
        name = "com_google_cloud_go_redis",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "cloud.google.com/go/redis",
        sum = "h1:6LI8zSt+vmE3WQ7hE5GsJ13CbJBLV1qUw6B7CY31Wcw=",
        version = "v1.18.3",
    )
    go_repository(
        name = "com_google_cloud_go_resourcemanager",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "cloud.google.com/go/resourcemanager",
        sum = "h1:oPZKIdjyVTuag+D4HF7HO0mnSqcqgjcuA18xblwA0V0=",
        version = "v1.10.7",
    )
    go_repository(
        name = "com_google_cloud_go_resourcesettings",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "cloud.google.com/go/resourcesettings",
        sum = "h1:13HOFU7v4cEvIHXSAQbinF4wp2Baybbq7q9FMctg1Ek=",
        version = "v1.8.3",
    )
    go_repository(
        name = "com_google_cloud_go_retail",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "cloud.google.com/go/retail",
        sum = "h1:S9Rl1sB6Dmdak5y9I2Pwn9hcWA5ubk33Vnevxw+CSrI=",
        version = "v1.25.1",
    )
    go_repository(
        name = "com_google_cloud_go_run",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "cloud.google.com/go/run",
        sum = "h1:4cwyNv9SUQEsQOf5/DfPKyMWYSA52p38/o119BgMhO4=",
        version = "v1.15.0",
    )
    go_repository(
        name = "com_google_cloud_go_scheduler",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "cloud.google.com/go/scheduler",
        sum = "h1:BoXY2BvBsaRw3ggVMzC9tborZqJBu+NcJcD9PqeC5Kc=",
        version = "v1.11.8",
    )
    go_repository(
        name = "com_google_cloud_go_secretmanager",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "cloud.google.com/go/secretmanager",
        sum = "h1:19QT7ZsLJ8FSP1k+4esQvuCD7npMJml6hYzilxVyT+k=",
        version = "v1.16.0",
    )
    go_repository(
        name = "com_google_cloud_go_security",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "cloud.google.com/go/security",
        sum = "h1:cF3FkCRRbRC1oXuaGZFl3qU2sdu2gP3iOAHKzL5y04Y=",
        version = "v1.19.2",
    )
    go_repository(
        name = "com_google_cloud_go_securitycenter",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "cloud.google.com/go/securitycenter",
        sum = "h1:D9zpeguY4frQU35GBw8+M6Gw79CiuTF9iVs4sFm3FDY=",
        version = "v1.38.1",
    )
    go_repository(
        name = "com_google_cloud_go_servicedirectory",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "cloud.google.com/go/servicedirectory",
        sum = "h1:je2yZlVcVFI/TshPXjjF9ZAlWedj0s5EbO2kozJrzBo=",
        version = "v1.12.7",
    )
    go_repository(
        name = "com_google_cloud_go_shell",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "cloud.google.com/go/shell",
        sum = "h1:K1C9sh9EuNNhGpyCoqRdeudcU9zmfYTA95bhF5cokK8=",
        version = "v1.8.7",
    )
    go_repository(
        name = "com_google_cloud_go_spanner",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "cloud.google.com/go/spanner",
        sum = "h1:M9RGcj/4gJk6yY1lRLOz1Ze+5ufoWhbIiurzXLOOfcw=",
        version = "v1.87.0",
    )
    go_repository(
        name = "com_google_cloud_go_speech",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "cloud.google.com/go/speech",
        sum = "h1:ehOzN/IsAhjjAtWg4fI8A3iNtonb1N8yWjofVhSTv+c=",
        version = "v1.29.0",
    )
    go_repository(
        name = "com_google_cloud_go_storage",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "cloud.google.com/go/storage",
        sum = "h1:iixmq2Fse2tqxMbWhLWC9HfBj1qdxqAmiK8/eqtsLxI=",
        version = "v1.56.0",
    )
    go_repository(
        name = "com_google_cloud_go_storagetransfer",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "cloud.google.com/go/storagetransfer",
        sum = "h1:Sjukr1LtUt7vLTHNvGc2gaAqlXNFeDFRIRmWGrFaJlY=",
        version = "v1.13.1",
    )
    go_repository(
        name = "com_google_cloud_go_talent",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "cloud.google.com/go/talent",
        sum = "h1:1kJJ+WCY5LZ1A4rCa32zKh3N2xT3I8koiS63+vV0WC4=",
        version = "v1.8.4",
    )
    go_repository(
        name = "com_google_cloud_go_texttospeech",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "cloud.google.com/go/texttospeech",
        sum = "h1:Ra4w+6qmaeb12ozlPBqGw8Jzdge1yfzhvZgcXWdXw30=",
        version = "v1.16.0",
    )
    go_repository(
        name = "com_google_cloud_go_tpu",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "cloud.google.com/go/tpu",
        sum = "h1:5DDheA1f7yZ/KUbVT/9lL+Yhgd3IqHDSVVrSqDVkAFY=",
        version = "v1.8.4",
    )
    go_repository(
        name = "com_google_cloud_go_trace",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "cloud.google.com/go/trace",
        sum = "h1:kDNDX8JkaAG3R2nq1lIdkb7FCSi1rCmsEtKVsty7p+U=",
        version = "v1.11.7",
    )
    go_repository(
        name = "com_google_cloud_go_translate",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "cloud.google.com/go/translate",
        sum = "h1:aSxMbfJ3MVmEdQzu5jGXmPPxCAb1ySsor2yBMCI5MT4=",
        version = "v1.12.7",
    )
    go_repository(
        name = "com_google_cloud_go_video",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "cloud.google.com/go/video",
        sum = "h1:Hp+2AeM7b3AagdHcyh2820UTzSbGyqpFJVMu0nHbBcw=",
        version = "v1.27.1",
    )
    go_repository(
        name = "com_google_cloud_go_videointelligence",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "cloud.google.com/go/videointelligence",
        sum = "h1:FisUrSZ+y3oLuGdlFQQgZoNTDm7FAfb2hwSTsSqX+9g=",
        version = "v1.12.7",
    )
    go_repository(
        name = "com_google_cloud_go_vision_v2",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "cloud.google.com/go/vision/v2",
        sum = "h1:9UtOINPF8p9VACQ6KAyR/ZtkpuBHGmJsprutYupDcN0=",
        version = "v2.9.6",
    )
    go_repository(
        name = "com_google_cloud_go_vmmigration",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "cloud.google.com/go/vmmigration",
        sum = "h1:6AvttGxASQTiuIsNKUKOKsRiQG4qTMOY4KMyBhdZa1w=",
        version = "v1.10.0",
    )
    go_repository(
        name = "com_google_cloud_go_vmwareengine",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "cloud.google.com/go/vmwareengine",
        sum = "h1:TKvULKbk44QrIx674cnoVjcZueXhyCAm2sNAJu/S1ds=",
        version = "v1.3.6",
    )
    go_repository(
        name = "com_google_cloud_go_vpcaccess",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "cloud.google.com/go/vpcaccess",
        sum = "h1:K6siDR1T4HgSTv6sy6CAwupY7UGza6TQ1O8jtvEYoX4=",
        version = "v1.8.7",
    )
    go_repository(
        name = "com_google_cloud_go_webrisk",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "cloud.google.com/go/webrisk",
        sum = "h1:q6zEdVgD8Ka+4fQl3azDcSNRug8clNnQ9iVS2iLh+MM=",
        version = "v1.11.2",
    )
    go_repository(
        name = "com_google_cloud_go_websecurityscanner",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "cloud.google.com/go/websecurityscanner",
        sum = "h1:udhvvDDRryM3nrITJk/eQe74D06KK2N3SF60/FH2njQ=",
        version = "v1.7.7",
    )
    go_repository(
        name = "com_google_cloud_go_workflows",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "cloud.google.com/go/workflows",
        sum = "h1:FGF6QEl3rtOSIHPOMZofWRVy3KNx26jDdgoYzJZ6ZhY=",
        version = "v1.14.3",
    )
    go_repository(
        name = "dev_cel_expr",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "cel.dev/expr",
        sum = "h1:K6j46C81hXtZQfuX60cVWQFBJahKSE2gfRbNuvr5bFs=",
        version = "v0.25.2",
    )
    go_repository(
        name = "in_gopkg_alecthomas_kingpin_v2",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "gopkg.in/alecthomas/kingpin.v2",
        sum = "h1:jMFz6MfLP0/4fUyZle81rXUoxOBFi19VUFKVDOQfozc=",
        version = "v2.2.6",
    )
    go_repository(
        name = "in_gopkg_check_v1",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "gopkg.in/check.v1",
        sum = "h1:Hei/4ADfdWqJk1ZMxUNpqntNwaWcugrBjAiHlqqRiVk=",
        version = "v1.0.0-20201130134442-10cb98267c6c",
    )
    go_repository(
        name = "in_gopkg_inf_v0",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "gopkg.in/inf.v0",
        sum = "h1:73M5CoZyi3ZLMOyDlQh031Cx6N9NDJ2Vvfl76EDAgDc=",
        version = "v0.9.1",
    )
    go_repository(
        name = "in_gopkg_yaml_v2",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "gopkg.in/yaml.v2",
        sum = "h1:D8xgwECY7CYvx+Y2n4sBz93Jn9JRvxdiyyo8CTfuKaY=",
        version = "v2.4.0",
    )
    go_repository(
        name = "in_gopkg_yaml_v3",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "gopkg.in/yaml.v3",
        sum = "h1:fxVm/GzAzEWqLHuvctI91KS9hhNmmWOoWu0XTYJS7CA=",
        version = "v3.0.1",
    )
    go_repository(
        name = "io_k8s_api",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "k8s.io/api",
        sum = "h1:0pCo/AN9hONazBKlNUdhQymmnfLRbSZjd5H5H3f0bSs=",
        version = "v0.27.4",
    )
    go_repository(
        name = "io_k8s_apimachinery",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "k8s.io/apimachinery",
        sum = "h1:CdxflD4AF61yewuid0fLl6bM4a3q04jWel0IlP+aYjs=",
        version = "v0.27.4",
    )
    go_repository(
        name = "io_k8s_klog_v2",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "k8s.io/klog/v2",
        sum = "h1:7WCHKK6K8fNhTqfBhISHQ97KrnJNFZMcQvKp7gP/tmg=",
        version = "v2.100.1",
    )
    go_repository(
        name = "io_k8s_kube_openapi",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "k8s.io/kube-openapi",
        sum = "h1:2kWPakN3i/k81b0gvD5C5FJ2kxm1WrQFanWchyKuqGg=",
        version = "v0.0.0-20230501164219-8b0f38b5fd1f",
    )
    go_repository(
        name = "io_k8s_sigs_json",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "sigs.k8s.io/json",
        sum = "h1:EDPBXCAspyGV4jQlpZSudPeMmr1bNJefnuqLsRAsHZo=",
        version = "v0.0.0-20221116044647-bc3834ca7abd",
    )
    go_repository(
        name = "io_k8s_sigs_structured_merge_diff_v4",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "sigs.k8s.io/structured-merge-diff/v4",
        sum = "h1:UZbZAZfX0wV2zr7YZorDz6GXROfDFj6LvqCRm4VUVKk=",
        version = "v4.3.0",
    )
    go_repository(
        name = "io_k8s_sigs_yaml",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "sigs.k8s.io/yaml",
        sum = "h1:Mk1wCc2gy/F0THH0TAp1QYyJNzRm2KCLy3o5ASXVI5E=",
        version = "v1.4.0",
    )
    go_repository(
        name = "io_k8s_utils",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "k8s.io/utils",
        sum = "h1:sgn3ZU783SCgtaSJjpcVVlRqd6GSnlTLKgpAAttJvpI=",
        version = "v0.0.0-20230726121419-3b25d923346b",
    )
    go_repository(
        name = "io_opencensus_go",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "go.opencensus.io",
        sum = "h1:y73uSU6J157QMP2kn2r30vwW1A2W2WFwSCGnAVxeaD0=",
        version = "v0.24.0",
    )
    go_repository(
        name = "io_opentelemetry_go_auto_sdk",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "go.opentelemetry.io/auto/sdk",
        sum = "h1:jXsnJ4Lmnqd11kwkBV2LgLoFMZKizbCi5fNZ/ipaZ64=",
        version = "v1.2.1",
    )
    go_repository(
        name = "io_opentelemetry_go_contrib_detectors_gcp",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "go.opentelemetry.io/contrib/detectors/gcp",
        sum = "h1:NmLfL734pJhM0JKaYd2Y28+nY9dPRWYAAbxhRCrKXPw=",
        version = "v1.44.0",
    )
    go_repository(
        name = "io_opentelemetry_go_contrib_instrumentation_google_golang_org_grpc_otelgrpc",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc",
        sum = "h1:q4XOmH/0opmeuJtPsbFNivyl7bCt7yRBbeEm2sC/XtQ=",
        version = "v0.61.0",
    )
    go_repository(
        name = "io_opentelemetry_go_contrib_instrumentation_net_http_otelhttp",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp",
        sum = "h1:F7Jx+6hwnZ41NSFTO5q4LYDtJRXBf2PD0rNBkeB/lus=",
        version = "v0.61.0",
    )
    go_repository(
        name = "io_opentelemetry_go_otel",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "go.opentelemetry.io/otel",
        sum = "h1:JjwHmHpA4iZ3wBxluu2fbbE7j4kqlE8jXyAyPXH7HqU=",
        version = "v1.44.0",
    )
    go_repository(
        name = "io_opentelemetry_go_otel_exporters_stdout_stdoutmetric",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "go.opentelemetry.io/otel/exporters/stdout/stdoutmetric",
        sum = "h1:rixTyDGXFxRy1xzhKrotaHy3/KXdPhlWARrCgK+eqUY=",
        version = "v1.36.0",
    )
    go_repository(
        name = "io_opentelemetry_go_otel_metric",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "go.opentelemetry.io/otel/metric",
        sum = "h1:1w0gILTcHdr3YI+ixLyjemwrVnsMURbTZFrSYCdDdmc=",
        version = "v1.44.0",
    )
    go_repository(
        name = "io_opentelemetry_go_otel_metric_x",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "go.opentelemetry.io/otel/metric/x",
        sum = "h1:YkCrx1zLOChi9ZcZ6euupOcsgzbVlec7D/xoEU1+cTA=",
        version = "v0.66.0",
    )
    go_repository(
        name = "io_opentelemetry_go_otel_sdk",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "go.opentelemetry.io/otel/sdk",
        sum = "h1:nHYwb9lK+fJPU/dnT6s7W7Z8itMWyqrnVfbheVYrZ58=",
        version = "v1.44.0",
    )
    go_repository(
        name = "io_opentelemetry_go_otel_sdk_metric",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "go.opentelemetry.io/otel/sdk/metric",
        sum = "h1:3LlKgI+VjbVsjNRFZJZAJ30WjXC5VkNRks6si09iEfI=",
        version = "v1.44.0",
    )
    go_repository(
        name = "io_opentelemetry_go_otel_trace",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "go.opentelemetry.io/otel/trace",
        sum = "h1:jxF5CsGYCe74MCRx2X4g7WsY/VBKRqqpNvXlX/6gtIk=",
        version = "v1.44.0",
    )
    go_repository(
        name = "io_opentelemetry_go_proto_otlp",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "go.opentelemetry.io/proto/otlp",
        sum = "h1:l706jCMITVouPOqEnii2fIAuO3IVGBRPV5ICjceRb/A=",
        version = "v1.9.0",
    )
    go_repository(
        name = "org_bitbucket_creachadair_stringset",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "bitbucket.org/creachadair/stringset",
        sum = "h1:6Sv4CCv14Wm+OipW4f3tWOb0SQVpBDLW0knnJqUnmZ8=",
        version = "v0.0.11",
    )
    go_repository(
        name = "org_golang_google_api",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "google.golang.org/api",
        sum = "h1:+Fo3DQXBK8gLdf8rFZ3uLu39JpOnhvzJrLMQSoSYZJM=",
        version = "v0.264.0",
    )
    go_repository(
        name = "org_golang_google_appengine",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "google.golang.org/appengine",
        sum = "h1:IhEN5q69dyKagZPYMSdIjS2HqprW324FRQZJcGqPAsM=",
        version = "v1.6.8",
    )
    go_repository(
        name = "org_golang_google_genproto",
        build_extra_args = [
            "-exclude=googleapis/api",
            "-exclude=googleapis/rpc",
            "-exclude=googleapis/bytestream",
        ],
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "google.golang.org/genproto",
        sum = "h1:VQZ/yAbAtjkHgH80teYd2em3xtIkkHd7ZhqfH2N9CsM=",
        version = "v0.0.0-20260128011058-8636f8732409",
    )
    go_repository(
        name = "org_golang_google_genproto_googleapis_api",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "google.golang.org/genproto/googleapis/api",
        sum = "h1:Kjn0N0tCrDgiAFW+lGO4JZ3ck44CehvJQMAwj9QF0G8=",
        version = "v0.0.0-20260526163538-3dc84a4a5aaa",
    )
    go_repository(
        name = "org_golang_google_genproto_googleapis_bytestream",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "google.golang.org/genproto/googleapis/bytestream",
        sum = "h1:Q9v92SXbvCsk89QPHVik5fAtq93/x/R8/KNWeS3numk=",
        version = "v0.0.0-20260122232226-8e98ce8d340d",
    )
    go_repository(
        name = "org_golang_google_genproto_googleapis_rpc",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "google.golang.org/genproto/googleapis/rpc",
        sum = "h1:mZHHdPZl0dbGHCflZgAq/Q468DWVFcU2whhB2KAo8fk=",
        version = "v0.0.0-20260526163538-3dc84a4a5aaa",
    )
    go_repository(
        name = "org_golang_google_grpc",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "google.golang.org/grpc",
        sum = "h1:HIO0+BEtBP6soyqvqC8sNUjZ7bTs+0hFQuFF+RAy++Y=",
        version = "v1.83.1",
    )
    go_repository(
        name = "org_golang_google_grpc_examples",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "google.golang.org/grpc/examples",
        sum = "h1:ExN12ndbJ608cboPYflpTny6mXSzPrDLh0iTaVrRrds=",
        version = "v0.0.0-20250407062114-b368379ef8f6",
    )
    go_repository(
        name = "org_golang_google_protobuf",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "google.golang.org/protobuf",
        sum = "h1:fV6ZwhNocDyBLK0dj+fg8ektcVegBBuEolpbTQyBNVE=",
        version = "v1.36.11",
    )
    go_repository(
        name = "org_golang_x_crypto",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "golang.org/x/crypto",
        sum = "h1:IBPXwPfKxY7cWQZ38ZCIRPI50YLeevDLlLnyC5wRGTI=",
        version = "v0.51.0",
    )
    go_repository(
        name = "org_golang_x_exp",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "golang.org/x/exp",
        sum = "h1:c2HOrn5iMezYjSlGPncknSEr/8x5LELb/ilJbXi9DEA=",
        version = "v0.0.0-20190121172915-509febef88a4",
    )
    go_repository(
        name = "org_golang_x_lint",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "golang.org/x/lint",
        sum = "h1:XQyxROzUlZH+WIQwySDgnISgOivlhjIEwaQaJEJrrN0=",
        version = "v0.0.0-20190313153728-d0100b6bd8b3",
    )
    go_repository(
        name = "org_golang_x_mod",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "golang.org/x/mod",
        sum = "h1:Ww1D637e6Pg+Zb2KrWfHQUnH2dQRLBQyAtpr/haaJeM=",
        version = "v0.35.0",
    )
    go_repository(
        name = "org_golang_x_net",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "golang.org/x/net",
        sum = "h1:bcvxaJn3e1U6InsFWt1JUq1aSjnRxLzT2rtD2KfkDF8=",
        version = "v0.55.0",
    )
    go_repository(
        name = "org_golang_x_oauth2",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "golang.org/x/oauth2",
        sum = "h1:peZ/1z27fi9hUOFCAZaHyrpWG5lwe0RJEEEeH0ThlIs=",
        version = "v0.36.0",
    )
    go_repository(
        name = "org_golang_x_sync",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "golang.org/x/sync",
        sum = "h1:e0PTpb7pjO8GAtTs2dQ6jYa5BWYlMuX047Dco/pItO4=",
        version = "v0.20.0",
    )
    go_repository(
        name = "org_golang_x_sys",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "golang.org/x/sys",
        sum = "h1:dO4czNzziLiiXplLQgBCEpCvXQ3dnkn0SdaZSYdQ+FY=",
        version = "v0.45.0",
    )
    go_repository(
        name = "org_golang_x_term",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "golang.org/x/term",
        sum = "h1:S4RLU2sB31O/NCl+zFN9Aru9A/Cq2aqKpTZJ6B+DwT4=",
        version = "v0.43.0",
    )
    go_repository(
        name = "org_golang_x_text",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "golang.org/x/text",
        sum = "h1:Cqjiwd9eSg8e0QAkyCaQTNHFIIzWtidPahFWR83rTrc=",
        version = "v0.37.0",
    )
    go_repository(
        name = "org_golang_x_time",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "golang.org/x/time",
        sum = "h1:MRx4UaLrDotUKUdCIqzPC48t1Y9hANFKIRpNx+Te8PI=",
        version = "v0.14.0",
    )
    go_repository(
        name = "org_golang_x_tools",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "golang.org/x/tools",
        sum = "h1:UP4ajHPIcuMjT1GqzDWRlalUEoY+uzoZKnhOjbIPD2c=",
        version = "v0.44.0",
    )
    go_repository(
        name = "org_golang_x_xerrors",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "golang.org/x/xerrors",
        sum = "h1:go1bK/D/BFZV2I8cIQd1NKEZ+0owSTG1fDTci4IqFcE=",
        version = "v0.0.0-20200804184101-5ec99f83aff1",
    )
    go_repository(
        name = "org_gonum_v1_gonum",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "gonum.org/v1/gonum",
        sum = "h1:VbpOemQlsSMrYmn7T2OUvQ4dqxQXU+ouZFQsZOx50z4=",
        version = "v0.17.0",
    )
    go_repository(
        name = "org_uber_go_goleak",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "go.uber.org/goleak",
        sum = "h1:2K3zAYmnTNqV73imy9J1T3WC+gmCePx2hEGkimedGto=",
        version = "v1.3.0",
    )
    go_repository(
        name = "tech_einride_go_aip",
        build_file_generation = "on",
        build_file_proto_mode = "disable",
        importpath = "go.einride.tech/aip",
        sum = "h1:bPo4oqBo2ZQeBKo4ZzLb1kxYXTY1ysJhpvQyfuGzvps=",
        version = "v0.73.0",
    )
