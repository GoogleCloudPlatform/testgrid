# gazelle:repository_macro repos.bzl%go_repositories
workspace(name = "com_github_googlecloudplatform_testgrid")

load("@bazel_tools//tools/build_defs/repo:http.bzl", "http_archive", "http_file")

http_archive(
    name = "io_bazel_rules_go",
    sha256 = "86d3dc8f59d253524f933aaf2f3c05896cb0b605fc35b460c0b4b039996124c6",
    urls = [
        "https://mirror.bazel.build/github.com/bazel-contrib/rules_go/releases/download/v0.60.0/rules_go-v0.60.0.zip",
        "https://github.com/bazel-contrib/rules_go/releases/download/v0.60.0/rules_go-v0.60.0.zip",
    ],
)

local_repository(
    name = "rbe_default",
    path = ".",
)

canary_repo_infra = False  # Set to True to use the local version

canary_repo_infra and local_repository(
    name = "io_k8s_repo_infra",
    path = "../repo-infra",
) or http_archive(
    name = "io_k8s_repo_infra",
    sha256 = "08520be644db1ade22a36d84dbeec9a00995e3eb450e433aef20ae56fbaf3bea",
    strip_prefix = "repo-infra-0.2.5",
    urls = [
        "https://github.com/kubernetes/repo-infra/archive/v0.2.5.tar.gz",
    ],
)

http_archive(
    name = "io_bazel_rules_docker",
    sha256 = "b1e80761a8a8243d03ebca8845e9cc1ba6c82ce7c5179ce2b295cd36f7e394bf",
    urls = ["https://github.com/bazelbuild/rules_docker/releases/download/v0.25.0/rules_docker-v0.25.0.tar.gz"],
)

http_archive(
    name = "io_bazel_rules_k8s",
    sha256 = "ce5b9bc0926681e2e7f2147b49096f143e6cbc783e71bc1d4f36ca76b00e6f4a",
    strip_prefix = "rules_k8s-0.7",
    urls = ["https://github.com/bazelbuild/rules_k8s/archive/refs/tags/v0.7.tar.gz"],
)

http_archive(
    name = "bazel_gazelle",
    sha256 = "75df288c4b31c81eb50f51e2e14f4763cb7548daae126817247064637fd9ea62",
    urls = [
        "https://mirror.bazel.build/github.com/bazelbuild/bazel-gazelle/releases/download/v0.36.0/bazel-gazelle-v0.36.0.tar.gz",
        "https://github.com/bazelbuild/bazel-gazelle/releases/download/v0.36.0/bazel-gazelle-v0.36.0.tar.gz",
    ],
)

http_archive(
    name = "com_google_protobuf",
    sha256 = "10a0d58f39a1a909e95e00e8ba0b5b1dc64d02997f741151953a2b3659f6e78c",
    strip_prefix = "protobuf-29.0",
    urls = [
        "https://github.com/protocolbuffers/protobuf/releases/download/v29.0/protobuf-29.0.tar.gz",
    ],
)

http_archive(
    name = "rules_python",
    sha256 = "778aaeab3e6cfd56d681c89f5c10d7ad6bf8d2f1a72de9de55b23081b2d31618",
    strip_prefix = "rules_python-0.34.0",
    urls = [
        "https://github.com/bazelbuild/rules_python/releases/download/0.34.0/rules_python-0.34.0.tar.gz",
    ],
)

http_archive(
    name = "rules_proto",
    sha256 = "602e7161d9195e50246177e7c55b2f39950a9cf7366f74ed5f22fd45750cd208",
    strip_prefix = "rules_proto-97d8af4dc474595af3900dd85cb3a29ad28cc313",
    urls = [
        "https://mirror.bazel.build/github.com/bazelbuild/rules_proto/archive/97d8af4dc474595af3900dd85cb3a29ad28cc313.tar.gz",
        "https://github.com/bazelbuild/rules_proto/archive/97d8af4dc474595af3900dd85cb3a29ad28cc313.tar.gz",
    ],
)

#### LOAD

load("@bazel_gazelle//:deps.bzl", "gazelle_dependencies")
load("@io_bazel_rules_go//go:deps.bzl", "go_register_toolchains", "go_rules_dependencies")
load("//:repos.bzl", "go_repositories")

go_repositories()

go_rules_dependencies()

go_register_toolchains(version = "1.25.10")

gazelle_dependencies()

load(
    "@io_bazel_rules_docker//go:image.bzl",
    _go_image_repos = "repositories",
    _go_repositories = "repositories",
)

_go_image_repos()

load("@rules_python//python:repositories.bzl", "py_repositories")

py_repositories()

http_archive(
    name = "com_google_absl",
    strip_prefix = "abseil-cpp-20240722.0",
    urls = [
        "https://github.com/abseil/abseil-cpp/archive/refs/tags/20240722.0.tar.gz",
    ],
)

load("@com_google_protobuf//:protobuf_deps.bzl", "protobuf_deps")

protobuf_deps()  # No options

http_file(
    name = "go_puller_linux_amd64",
    executable = True,
    sha256 = "08b8963cce9234f57055bafc7cadd1624cdce3c5990048cea1df453d7d288bc6",
    urls = ["https://mirror.bazel.build/storage.googleapis.com/rules_docker/aad94363e63d31d574cf701df484b3e8b868a96a/puller-linux-amd64"],
)

http_file(
    name = "go_puller_darwin",
    executable = True,
    sha256 = "4855c4f5927f8fb0f885510ab3e2a166d5fa7cde765fbe9aec97dc6b2761bb22",
    urls = ["https://mirror.bazel.build/storage.googleapis.com/rules_docker/aad94363e63d31d574cf701df484b3e8b868a96a/puller-darwin-amd64"],
)

http_file(
    name = "go_puller_linux_arm64",
    executable = True,
    sha256 = "912ee7c469b3e4bf15ba5d1f0ee500e7ec6724518862703fa8b09e4d58ce3ee6",
    urls = ["https://mirror.bazel.build/storage.googleapis.com/rules_docker/aad94363e63d31d574cf701df484b3e8b868a96a/puller-linux-arm64"],
)

http_file(
    name = "go_puller_linux_s390x",
    executable = True,
    sha256 = "a5527b7b3b4a266e4680a4ad8939429665d4173f26b35d5d317385134369e438",
    urls = ["https://mirror.bazel.build/storage.googleapis.com/rules_docker/aad94363e63d31d574cf701df484b3e8b868a96a/puller-linux-s390x"],
)

load(
    "@io_bazel_rules_docker//repositories:repositories.bzl",
    _container_repositories = "repositories",
)

_container_repositories()

load("@io_bazel_rules_docker//repositories:deps.bzl", _container_deps = "deps")

_container_deps()

_go_repositories()

load("@io_bazel_rules_k8s//k8s:k8s.bzl", "k8s_repositories")

k8s_repositories()
