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

load("@io_bazel_rules_docker//container:image.bzl", "container_image")
load("@io_bazel_rules_docker//container:bundle.bzl", "container_bundle")
load("@io_bazel_rules_docker//contrib:push-all.bzl", "container_push")
load("@io_bazel_rules_docker//go:image.bzl", _go_image = "go_image")

## make_image is a macro for creating :app and :image targets
def go_image(
        name,  # use "image"
        base = None,
        stamp = True,  # stamp by default, but allow overrides
        app_name = "app",
        **kwargs):
    _go_image(
        name = app_name,
        base = base,
        embed = [":go_default_library"],
        goarch = "amd64",
        goos = "linux",
        pure = "on",
    )

    container_image(
        name = name,
        base = ":" + app_name,
        stamp = stamp,
        **kwargs
    )

# push_image creates a bundle of container images, and a target to push them.
def push_image(
        name,
        bundle_name = "bundle",
        images = None):
    container_bundle(
        name = bundle_name,
        images = images,
    )
    container_push(
        name = name,
        bundle = ":" + bundle_name,
        format = "Docker",  # TODO(fejta): consider OCI?
    )

# tags appends default tags to name
#
# In particular, names is a {image_prefix: image_target} mapping, which gets
# expanded into three full image paths:
#   image_prefix:latest
#   image_prefix:latest-{BUILD_USER}
#   image_prefix:{DOCKER_TAG}
# (See hack/print-workspace-status.sh for how BUILD_USER and DOCKER_TAG are created.
def tags(targets):
    outs = {}
    for img, target in targets.items():
        outs["%s:{DOCKER_TAG}" % img] = target
        outs["%s:latest-{BUILD_USER}" % img] = target
        outs["%s:latest" % img] = target
    return outs
