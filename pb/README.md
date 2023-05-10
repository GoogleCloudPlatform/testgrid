# Protocol Buffers in TestGrid

TestGrid stores its configuration, state, and other information in cloud storage
encoded via these protocol buffers.

## Reading a Protocol Buffer

Protocol buffers can be read using the proto compiler `protoc`. Be sure your
working directory is this repository.

This example uses gsutil to read a Configuration from Google Cloud Storage. Then,
it uses protoc to decode it.
```bash
gsutil cat gs://example-bucket/config | protoc --decode=Configuration pb/config/config.proto
```

You need to pass protoc the proto name and file used to encode the file.

These components generally generate these types of protos:

| Component | Message | Source |
|-----------|---------|--------|
| Configurator or [Config Merger](/cmd/config_merger) | `Configuration` | [config.proto](./config/config.proto) |
| [Summarizer](/cmd/summarizer) | `DashboardSummary` | [summary.proto](./summary/summary.proto) |
| [Updater](/cmd/updater)  | `Grid` (see [Reading a Grid](#reading-a-grid))| [state.proto](./state/state.proto) |
| [Tabulator](/cmd/tabulator) | `Grid` (see [Reading a Grid](#reading-a-grid)) | [state.proto](./state/state.proto) |

### Reading a Grid

The Updater and Tabulator will compress its state as well as encoding it. To read it, you'll
need to do one of the following:
- In Go: Use [DownloadGrid()](/util/gcs/gcs.go) or `zlib.NewReader(reader)`
- In shell: Use a script that will uncompress zlib, then pipe that result to `protoc`

### Reading an Unknown Protocol Buffer

```bash
gsutil cat gs://example-bucket/config | protoc --decode_raw
```

The result will use message numbers instead of message names. For example, `1`
instead of `test_groups`

## Changing a Protocol Buffer Definition

If you want to change one of the .proto files in this repository, you'll also
need to regenerate the .pb.go files. Do so with this command:
```bash
bazel run //hack:update-protos
```
