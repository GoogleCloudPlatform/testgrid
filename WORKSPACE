# gazelle:repository_macro repos.bzl%go_repositories
workspace(name = "com_github_googlecloudplatform_testgrid")

load("@bazel_tools//tools/build_defs/repo:http.bzl", "http_archive")

http_archive(
    name = "io_bazel_rules_go",
    sha256 = "685052b498b6ddfe562ca7a97736741d87916fe536623afb7da2824c0211c369",
    urls = [
        "https://mirror.bazel.build/github.com/bazelbuild/rules_go/releases/download/v0.33.0/rules_go-v0.33.0.zip",
        "https://github.com/bazelbuild/rules_go/releases/download/v0.33.0/rules_go-v0.33.0.zip",
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
    sha256 = "27d53c1d646fc9537a70427ad7b034734d08a9c38924cc6357cc973fed300820",
    strip_prefix = "rules_docker-0.24.0",
    urls = ["https://github.com/bazelbuild/rules_docker/releases/download/v0.24.0/rules_docker-v0.24.0.tar.gz"],
)

load("@bazel_tools//tools/build_defs/repo:git.bzl", "git_repository")

git_repository(
    name = "io_bazel_rules_k8s",
    commit = "d63175c26408c06c47aea3063384fbaf1f720145",  # branch = "master"
    remote = "https://github.com/bazelbuild/rules_k8s.git",
    shallow_since = "1655156597 -0700",
)

http_archive(
    name = "bazel_gazelle",
    sha256 = "5982e5463f171da99e3bdaeff8c0f48283a7a5f396ec5282910b9e8a49c0dd7e",
    urls = [
        "https://mirror.bazel.build/github.com/bazelbuild/bazel-gazelle/releases/download/v0.25.0/bazel-gazelle-v0.25.0.tar.gz",
        "https://github.com/bazelbuild/bazel-gazelle/releases/download/v0.25.0/bazel-gazelle-v0.25.0.tar.gz",
    ],
)

http_archive(
    name = "com_google_protobuf",
    sha256 = "f1a83673cbcaff6346a8fba87a9c02c0f943a4a696b6c7d1b71586d97609db12",
    strip_prefix = "protobuf-21.1",
    urls = [
        #"https://mirror.bazel.build/github.com/protocolbuffers/protobuf/archive/v21.1.tar.gz",
        "https://github.com/protocolbuffers/protobuf/archive/v21.1.tar.gz",
    ],
)

http_archive(
    name = "rules_python",
    sha256 = "cdf6b84084aad8f10bf20b46b77cb48d83c319ebe6458a18e9d2cebf57807cdd",
    strip_prefix = "rules_python-0.8.1",
    url = "https://github.com/bazelbuild/rules_python/archive/refs/tags/0.8.1.tar.gz",
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

load("@io_bazel_rules_go//go:deps.bzl", "go_register_toolchains", "go_rules_dependencies")
load("@bazel_gazelle//:deps.bzl", "gazelle_dependencies")

go_rules_dependencies()

go_register_toolchains(version = "1.18.3")

load("//:repos.bzl", "go_repositories")

go_repositories()

gazelle_dependencies()

load(
    "@io_bazel_rules_docker//go:image.bzl",
    _go_image_repos = "repositories",
)

_go_image_repos()

load("@com_google_protobuf//:protobuf_deps.bzl", "protobuf_deps")

protobuf_deps()  # No options

load(
    "@io_bazel_rules_docker//repositories:repositories.bzl",
    _container_repositories = "repositories",
)

_container_repositories()

load("@io_bazel_rules_docker//repositories:deps.bzl", _container_deps = "deps")

_container_deps()

load("@io_bazel_rules_docker//go:image.bzl", _go_repositories = "repositories")

_go_repositories()

load("@io_bazel_rules_k8s//k8s:k8s.bzl", "k8s_repositories")

k8s_repositories()
