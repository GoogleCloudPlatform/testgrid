# TestGrid Components

The `cmd` directory contains the main files for TestGrid components, and documentation for how to
run them. For specifics on a particular component, see the subdirectory for that component.

## Tips for running locally

### Google Cloud Storage (GCS) Authentication

If you're using files from GCS and you hit an authentication or 'unauthorized' error in logs, run:
```gcloud auth application-default login```

Most components will automatically find and use the credentials generated from this.

### Common Flags

Most components have similar flags and defaults. Here's a few you can expect to use:
- Run `bazelisk run //cmd/$COMPONENT -- --help` for full flag list and descriptions.
- `--config` specifies the path to a valid [config proto]. It can take a:
  - local file: e.g. `/tmp/testgrid/my-config.pb`
  - GCS (Google Cloud Storage) file: e.g. `gs://my-bucket/my-config.pb`
- `--confirm`, if true, writes data to the same base path as your `--config`.
  - Without this, it's "dry-run` by default and doesn't write data anywhere.
- `--wait`, if true, runs the component continuously until force-quit, waiting the specified time
  before beginning another cycle.
  - Without this, it will run once and exit.
- `--debug` or `--trace` print debug-level or trace-level logs, respectively.
  - Without this, only logs at info-level or higher will be displayed.
  - Debug logs tend to be useful for development or debugging.
  - Trace logs can be _very_ noisy, and tend to be useful for debugging something very specific.

### Developing and Testing

Most of the TestGrid libraries used for these components live under the `pkg/` directory.
- e.g. `cmd/updater` library code is under `pkg/updater`

To run unit tests for a particular component, run:

```shell
COMPONENT={component} # e.g. 'updater', 'summarizer', etc. 
bazelisk test //cmd/$COMPONENT/... # Runs unit tests for the main/binary, like flag-parsing or default arguments
bazelisk test //pkg/$COMPONENT/... # Runs unit tests for library code
```