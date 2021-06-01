# Config Printer

The config printer is a debugging utility that prints a TestGrid configuration in a
human-readable format. It will read from the local filesystem or Google Cloud Storage.

## Usage and installation

The tool can be built and run with Bazel.

```sh
bazel run //config/print -- gs://example/config
```

The tool can be installed via go install. You may want to rename the
resulting binary so it doesn't shadow your shell's `print` utility.

```sh
go install ./config/print
print gs://example/config
```
