
# Tabulator

This component reads test-group-based [state proto] files, combines them with dashboard tab
configuration, and produces tab-based [state proto] files.

The TestGrid frontend reads these protos and converts them to JSON, which the
javascript UI reads and renders on the screen.

These protos are read by:
- The frontend (e.g. [testgrid.k8s.io](https://testgrid.k8s.io))
- The [Summarizer] component

## Local development
See also [common tips](/cmd/README.md) for running locally.

```bash
# --config can take a local file (e.g. `/tmp/testgrid/config`) or GCS file (e.g. `gs://my-testgrid-bucket/config`)
bazelisk run //cmd/tabulator -- \
  --config=gs://my-testgrid-bucket/somewhere/config \
  # --groups=foo,bar \  # If specified, only tabulate these test groups.
  # --debug \
  # --confirm \
```

[state proto]: /pb/state/state.proto
[Summarizer]: /cmd/summarizer