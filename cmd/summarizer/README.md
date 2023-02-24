# Summarizer

This component reads dashboard-tab-based [state proto] files and summarizes them into a [summary proto].

These protos are read by the frontend (e.g. https://testgrid.k8s.io)

## Local development
See also [common tips](/cmd/README.md) for running locally.

```bash
# --config can take a local file (e.g. `/tmp/testgrid/config`) or GCS file (e.g. `gs://my-testgrid-bucket/config`)
bazelisk run //cmd/summarizer -- \
  --config=gs://my-testgrid-bucket/somewhere/config \
  # --dashboards=foo,bar \  # If specified, only summarize these dashboards. 
  # --debug \
  # --confirm \
```

[summary proto]: pb/summary/summary.proto
