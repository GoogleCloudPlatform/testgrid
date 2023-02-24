
# Testgrid Updater

This component is responsible for compiling collated GCS results into a [state proto].

These protos are read by the [Tabulator] component.

## Local development
See also [common tips](/cmd/README.md) for running locally.

```bash
# --config can take a local file (e.g. `/tmp/testgrid/config`) or GCS file (e.g. `gs://my-testgrid-bucket/config`)
bazelisk run //cmd/updater -- \
  --config=gs://my-testgrid-bucket/somewhere/config \
  # --test-group=foo,bar \  # If specified, only update these test groups.
  # --debug \
  # --confirm \
```

### Debugging

The two most useful flags are `--test-group=foo` and `--confirm=false` (default).

Setting the `--test-group` flag tells the client to only update a specific group.
This is useful when debugging problems in a specific group.

The `--confirm` flag controls whether any data is written. Nothing is written by default.


## Update cycles

Each update cycle the updater:

* Downloads the specified config proto to get the list of test groups.
* Iterates through each group
  - Downloads the existing state proto if present
    * Drops the oldest and newest columns
    * Old ones are no longer relevant
    * New columns may still change
  - Grabs the gcs prefix
  - Scans GCS under that prefix for results greater than existing ones
    * Each job is in a unique GCS\_PREFIX/JOB\_ID folder
    * New folders must be monotonically greater than old ones
  - Compiles the job result in each folder into a cell mapping
  - Converts the cell into the existing state grid proto.
    * Appends a new column into the state grid.
    * Creates any new rows.
    * Appends data to existing rows.
* Determines which (if any) rows have alerts
* Optionally uploads the proto to GCS

If the `--wait` flag is unset, the job returns at this time.

Otherwise it repeats after sleeping for that duration.

[state proto]: /pb/state/state.proto
[Tabulator]: /cmd/tabulator