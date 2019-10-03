# gazelle:repository_macro repos.bzl%go_repositories
workspace(name = "com_github_googlecloudplatform_testgrid")

load("@bazel_tools//tools/build_defs/repo:git.bzl", "git_repository")

git_repository(
    name = "io_k8s_repo_infra",
    commit = "db6ceb5f992254db76af7c25db2edc5469b5ea82",
    remote = "https://github.com/kubernetes/repo-infra.git",
    shallow_since = "1570128715 -0700",
)

load("@io_k8s_repo_infra//:load.bzl", "repositories")

repositories()

load("@io_k8s_repo_infra//:repos.bzl", "configure")

configure(go_modules = None)

load("//:repos.bzl", "go_repositories")

go_repositories()

load("@bazel_tools//tools/build_defs/repo:http.bzl", "http_archive")

http_archive(
    name = "io_bazel_rules_docker",
    sha256 = "e513c0ac6534810eb7a14bf025a0f159726753f97f74ab7863c650d26e01d677",
    strip_prefix = "rules_docker-0.9.0",
    urls = ["https://github.com/bazelbuild/rules_docker/archive/v0.9.0.tar.gz"],
)

load("@io_bazel_rules_docker//repositories:repositories.bzl", _container_repositories = "repositories")

_container_repositories()

load("@io_bazel_rules_docker//go:image.bzl", _go_repositories = "repositories")

_go_repositories()

git_repository(
    name = "io_bazel_rules_k8s",
    commit = "e7ae2825f0296314ac1ecf13e4c9acef66597986",
    remote = "https://github.com/bazelbuild/rules_k8s.git",
    shallow_since = "1565892120 -0400",
)

load("@io_bazel_rules_k8s//k8s:k8s.bzl", "k8s_repositories")

k8s_repositories()
