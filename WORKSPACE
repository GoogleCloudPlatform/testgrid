# gazelle:repository_macro repos.bzl%go_repositories
workspace(name = "com_github_googlecloudplatform_testgrid")

load("@bazel_tools//tools/build_defs/repo:http.bzl", "http_archive")

canary_repo_infra = False  # Set to True to use the local version

canary_repo_infra and local_repository(
    name = "io_k8s_repo_infra",
    path = "../repo-infra",
) or http_archive(
    name = "io_k8s_repo_infra",
    sha256 = "ae75a3a8de9698df30dd5a177c61f31ae9dd3a5da96ec951f0d6e60b2672d5fe",
    strip_prefix = "repo-infra-0.2.2",
    urls = [
        "https://github.com/kubernetes/repo-infra/archive/v0.2.2.tar.gz",
    ],
)

load("@io_k8s_repo_infra//:load.bzl", "repositories")

repositories()

load("@io_k8s_repo_infra//:repos.bzl", "configure", _repo_infra_go_repos = "go_repositories")

configure()

load("//:repos.bzl", "go_repositories")

go_repositories()

_repo_infra_go_repos()  # Load any missing repos that repo-infra depends on

go_repositories()  # We appear to need this call both before and after in order to use our version of modules.

http_archive(
    name = "io_bazel_rules_docker",
    sha256 = "4521794f0fba2e20f3bf15846ab5e01d5332e587e9ce81629c7f96c793bb7036",
    strip_prefix = "rules_docker-0.14.4",
    urls = ["https://github.com/bazelbuild/rules_docker/releases/download/v0.14.4/rules_docker-v0.14.4.tar.gz"],
)

load("@io_bazel_rules_docker//repositories:repositories.bzl", _container_repositories = "repositories")

_container_repositories()

load("@io_bazel_rules_docker//repositories:deps.bzl", _container_deps = "deps")

_container_deps()

load("@io_bazel_rules_docker//repositories:pip_repositories.bzl", _container_pip_deps = "pip_deps")

_container_pip_deps()

load("@io_bazel_rules_docker//go:image.bzl", _go_repositories = "repositories")

_go_repositories()

load("@bazel_tools//tools/build_defs/repo:git.bzl", "git_repository")

http_archive(
    name = "io_bazel_rules_k8s",
    sha256 = "773aa45f2421a66c8aa651b8cecb8ea51db91799a405bd7b913d77052ac7261a",
    strip_prefix = "rules_k8s-0.5",
    urls = ["https://github.com/bazelbuild/rules_k8s/archive/v0.5.tar.gz"],
)

load("@io_bazel_rules_k8s//k8s:k8s.bzl", "k8s_repositories")

k8s_repositories()

go_repositories()
