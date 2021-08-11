
# Skeleton

TODO(fejta): improve this documentation.

See:
* [job.go](/metadata/job.go) for information about `started.json` and `finished.json`.
* [junit subpackage](/metadata/junit) for information about the junit files.
* [prow](https://github.com/kubernetes/test-infra/tree/master/prow), which typically creates these results.
  - In particular its [pod utilities](https://github.com/kubernetes/test-infra/blob/master/prow/pod-utilities.md)
    which create these files as testgrid expects them.

# Pubsub

See documentation for [pubsub](https://cloud.google.com/pubsub) and [GCS' integration](https://cloud.google.com/storage/docs/pubsub-notifications).

Testgrid can provide near realtime results by configuring GCS to send notifications of newly written results to a pubsub topic.

TODO(fejta): improve documentation, link to setup script.
