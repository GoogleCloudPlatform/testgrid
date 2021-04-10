# Config Printer

The config printer is a debugging utility that prints a TestGrid configuration in a
human-readable format. It will read from the local filesystem or Google Cloud Storage.

## Usage and installation

```sh
go install ./config/print
print gs://example/config
```
A Bazel `run` command also works, but the utility won't be able to read from the local file system as expected.
