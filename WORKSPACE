# gazelle:repository_macro repos.bzl%go_repositories
workspace(name = "com_github_googlecloudplatform_testgrid")

load("@bazel_tools//tools/build_defs/repo:http.bzl", "http_archive")

http_archive(
    name = "io_k8s_repo_infra",
    strip_prefix = "repo-infra-0.0.3",
    sha256 = "a6ca952e365600a17f56f0fc8e41016e1d13cfb2b74c0c29bad6bdba3e3d8a4d",
    urls = [
        "https://github.com/kubernetes/repo-infra/archive/v0.0.3.tar.gz",
    ],
)

load("@io_k8s_repo_infra//:load.bzl", "repositories")

repositories()

load("@io_k8s_repo_infra//:repos.bzl", "configure", _repo_infra_go_repos = "go_repositories")

configure()

load("//:repos.bzl", "go_repositories")

go_repositories()

_repo_infra_go_repos()  # Load any missing repos that repo-infra depends on

http_archive(
    name = "io_bazel_rules_docker",
    sha256 = "413bb1ec0895a8d3249a01edf24b82fd06af3c8633c9fb833a0cb1d4b234d46d",
    strip_prefix = "rules_docker-0.12.0",
    urls = ["https://github.com/bazelbuild/rules_docker/archive/v0.12.0.tar.gz"],
)

load("@io_bazel_rules_docker//repositories:repositories.bzl", _container_repositories = "repositories")

_container_repositories()

load("@io_bazel_rules_docker//go:image.bzl", _go_repositories = "repositories")

_go_repositories()

load("@bazel_tools//tools/build_defs/repo:git.bzl", "git_repository")

git_repository(
    name = "io_bazel_rules_k8s",
    commit = "c7db606023bef31ca5c2ad49942f33c6137cb7f8",
    remote = "https://github.com/bazelbuild/rules_k8s.git",
    shallow_since = "1571437004 -0400",
)

load("@io_bazel_rules_k8s//k8s:k8s.bzl", "k8s_repositories")

k8s_repositories()
