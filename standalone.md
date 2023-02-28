# Setting up TestGrid

This tutorial goes over setting up your own TestGrid instance in [Google Cloud Storage]
(GCS). It assumes you are somewhat familiar with deploying on [Kubernetes], running a job on [Prow],
and using GCS.

Usually, to get your tests to display in TestGrid, setting up an instance isn't
necessary. Instead, add your tests to an existing [configuration].

## Architecture Overview

TestGrid consists of two major components; its "backend" controllers and its
"frontend" user interface.

These two components communicate using data stored in a cloud storage provider.
The controllers are responsible for keeping this data up-to-date. The user
interface displays this data.

- The source for the backend controllers are in the [cmd](./cmd) folder.
- Formats for the data layer are found in the [pb](./pb) folder.
- The frontend user interface isn't open source, but it can still be used as a service.

## Configuration

TestGrid depends on a [configuration](./pb/config) file to
determine what it should display.

You can get a configuration file in multiple ways:
- Serialize it from a YAML configuration file you write yourself
- Have a Prow instance generate a configuration file with Configurator

The configuration file should be named "config" (e.g. `gs://your-bucket/config`) in cloud storage,
and will be needed by every controller.

## Controllers

From a configuration file, controller programs fetch and generate the other
necessary data files.

The [Updater](./cmd/updater) generates and maintains the grid data to be displayed, for each test group.
Each test's current [state](./pb/state) is stored in cloud storage.

The [Tabulator](./cmd/tabulator) takes grid data from the Updater and creates a viewable grid (also a
[state](./pb/state)) for each dashboard tab. Each of these are stored in cloud storage.

The [Summarizer](./cmd/summarizer) generates and maintains a summary for each dashboard. These
[summaries](./pb/summary) are stored in cloud storage.

## Frontend Usage

- **Frontend API endpoints are subject to change as development continues.**

A TestGrid instance, like the one at [testgrid.k8s.io], displays a particular
configuration and state by default.

You can display other data (generated locally or by another instance) from a GCS bucket.
The TestGrid configuration located at `gs://your-bucket/config` can be rendered at
`testgrid.k8s.io/r/your-bucket/`.

- The service account `k8s-testgrid@appspot.gserviceaccount.com` must be given
permission to read from these files.

## Examples

### Standalone Testgrid FE For Knative Prow

#### Generate testgrid config

Set up a prow job that generates testgrid config from prow configs with
`configurator`. In knative scenario, the [configurator job example] writes the
config to a GCS bucket `gs://knative-own-testgrid/config`.

After this step, `testgrid.k8s.io/r/knative-own-testgrid` can already display
dashboards and dashboard tabs, but there are no test results yet.

#### Load data

- Create a `testgrid` cluster
  - Note: this can be a kubernetes cluster of your choice.
- Create the [deployment] of `Updater`, `Tabulator`, and `Summarizer` in this cluster.
  - Make sure to pass `--config=gs://knative-own-testgrid` to each controller.

After this step, data will slowly roll into `testgrid.k8s.io/r/knative-own-testgrid`. The initial round
could take a couple of hours, then subsequent updates will be much faster.


[testgrid.k8s.io]: (http://testgrid.k8s.io)
[configuration]: config.md
[configurator job example]: https://github.com/knative/test-infra/blob/13d8d5df1f5273ff3d751bf8cd92eec873a35d7b/config/prod/prow/jobs/custom/test-infra.yaml#L643
[deployment]:
https://github.com/GoogleCloudPlatform/testgrid/tree/master/cluster/prod/knative
[Prow]: https://docs.prow.k8s.io/docs/
[Google Cloud Storage]: https://cloud.google.com/storage/docs
[Kubernetes]: https://kubernetes.io/docs/home/

