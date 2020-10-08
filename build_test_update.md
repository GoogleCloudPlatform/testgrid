# Updating TestGrid

If you're looking to develop for TestGrid, welcome! Note that most of the inner workings of TestGrid
are not open source yet. Follow [this issue](https://github.com/kubernetes/test-infra/issues/10409)
for updates.

:information_source: The Front End UI is not currently open source, if you would like UI changes or features, please file a bug describing how you would like the UI to behave. :information_source:

## YAML configuration and config.proto

While TestGrid configuration is located in YAML files, TestGrid doesn't natively
read YAML. Instead, it expects configuration data in [`config.proto`]. This file
is commented, and should be treated as the authoritative "input" schema to
TestGrid.

[`config.proto`] is generated primarily with
[Configurator](https://github.com/kubernetes/test-infra/blob/master/testgrid/cmd/configurator).
Updates to the [testgrid.k8s.io config] are automatically Configurated when a change is
merged.

You can convert a yaml file to the config proto with:
```
bazel run //testgrid/cmd/configurator -- \
  --yaml=testgrid/config.yaml \
  --print-text \
  --oneshot \
  --output=/tmp/config.pb \
  # Or push to gcs
  # --output=gs://my-bucket/config
  # --gcp-service-account=/path/to/foo.json
```

For our production instance of TestGrid (https://testgrid.k8s.io), this file is read to a Google
Cloud Storage (GCS) location, and read from there.

## Changing a .proto file

If you modify a .proto file, you'll also need to generate and check in the
.pb.go files.

Run `bazel run //hack:update-protos` to generate, and `bazel run //hack:verify-protos.sh`
to verify.

## Testing

Run `bazel test //...` to run all unit tests in TestGrid. Note that this does not validate
the [testgrid.k8s.io config]; those tests are in `bazel test //config/tests/testgrids/...`

[`config.proto`]: ./config/config.proto
[testgrid.k8s.io config]: https://github.com/kubernetes/test-infra/blob/master/config/testgrids
