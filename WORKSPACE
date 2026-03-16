# gazelle:repository_macro repos.bzl%go_repositories
workspace(name = "com_github_googlecloudplatform_testgrid")

load("@bazel_tools//tools/build_defs/repo:http.bzl", "http_archive")

http_archive(
    name = "io_bazel_rules_go",
    integrity = "sha256-M6zErg9wUC20uJPJ/B3Xqb+ZjCPn/yxFF3QdQEmpdvg=",
    urls = [
        "https://mirror.bazel.build/github.com/bazelbuild/rules_go/releases/download/v0.48.0/rules_go-v0.48.0.zip",
        "https://github.com/bazelbuild/rules_go/releases/download/v0.48.0/rules_go-v0.48.0.zip",
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
    sha256 = "f6dcb97e992f13bc9effd794e9bb300f06b0dadc88061f81ae68d8d5994be964",
    urls = [
        "https://mirror.bazel.build/github.com/bazelbuild/rules_docker/releases/download/v0.26.0/rules_docker-v0.26.0.tar.gz",
        "https://github.com/bazelbuild/rules_docker/releases/download/v0.26.0/rules_docker-v0.26.0.tar.gz",
    ],
)

http_archive(
    name = "bazel_gazelle",
    integrity = "sha256-12v3pg/YsFBEQJDfooN6Tq+YKeEWVhjuNdzspcvfWNU=",
    urls = [
        "https://mirror.bazel.build/github.com/bazelbuild/bazel-gazelle/releases/download/v0.37.0/bazel-gazelle-v0.37.0.tar.gz",
        "https://github.com/bazelbuild/bazel-gazelle/releases/download/v0.37.0/bazel-gazelle-v0.37.0.tar.gz",
    ],
)

http_archive(
    name = "com_google_protobuf",
    sha256 = "c29d8b4b79389463c546f98b15aa4391d4ed7ec459340c47bffe15db63eb9126",
    strip_prefix = "protobuf-3.21.3",
    urls = ["https://github.com/protocolbuffers/protobuf/archive/v3.21.3.tar.gz"],
)

http_archive(
    name = "rules_python",
    sha256 = "cdf6b84084aad8f10bf20b46b77cb48d83c319ebe6458a18e9d2cebf57807cdd",
    strip_prefix = "rules_python-0.8.1",
    url = "https://github.com/bazelbuild/rules_python/archive/refs/tags/0.8.1.tar.gz",
)

http_archive(
    name = "rules_proto",
    sha256 = "a88d018bdcb8df1ce8185470eb4b4899d778f9ac3a66cb36d514beb81e345282",
    strip_prefix = "rules_proto-6.0.0-rc3",
    url = "https://github.com/bazelbuild/rules_proto/releases/download/6.0.0-rc3/rules_proto-6.0.0-rc3.tar.gz",
)

load("@rules_proto//proto:repositories.bzl", "rules_proto_dependencies")

rules_proto_dependencies()

load("@rules_proto//proto:toolchains.bzl", "rules_proto_toolchains")

rules_proto_toolchains()

#### LOAD

load("@bazel_gazelle//:deps.bzl", "gazelle_dependencies")
load("@io_bazel_rules_go//go:deps.bzl", "go_register_toolchains", "go_rules_dependencies")
load("//:deps.bzl", "go_dependencies")

# gazelle:repository_macro deps.bzl%go_dependencies
go_dependencies()

go_rules_dependencies()

go_register_toolchains(version = "1.20.5")

load("//:repos.bzl", "go_repositories")

go_repositories()

gazelle_dependencies()

load(
    "@io_bazel_rules_docker//go:image.bzl",
    _go_image_repos = "repositories",
    _go_repositories = "repositories",
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

_go_repositories()

http_archive(
    name = "io_bazel_rules_k8s",
    sha256 = "773aa45f2421a66c8aa651b8cecb8ea51db91799a405bd7b913d77052ac7261a",
    strip_prefix = "rules_k8s-0.5",
    urls = ["https://github.com/bazelbuild/rules_k8s/archive/v0.5.tar.gz"],
)

load("@io_bazel_rules_k8s//k8s:k8s.bzl", "k8s_repositories")

k8s_repositories()

load("@io_bazel_rules_k8s//k8s:k8s_go_deps.bzl", k8s_go_deps = "deps")

k8s_go_deps()

load("@bazel_features//:deps.bzl", "bazel_features_deps")

bazel_features_deps()
