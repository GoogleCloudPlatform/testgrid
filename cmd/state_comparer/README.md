# Compare States
Compares TestGrid states of the same names between two directories. Useful for validating that
changes to the updater or other code don't unexpectedly change the resulting state protos.

## Running
You should have two directories set up: one each for the states you want to compare (e.g. "/tmp/cmp/first" and "/tmp/cmp/second")

```shell
# Example basic run:
bazel run //cmd/state_comparer -- --first="/tmp/cmp/first/" --second="/tmp/cmp/second/" --diff-ratio-ok=0.3

# Example detailed/advanced run:
bazel run //cmd/state_comparer -- --first="/tmp/tgcmp/first/" --second="/tmp/tgcmp/second/" --diff-ratio-ok=0.3 --test-group-url="http://testgrid-canary/q/testgroup/" --config="/tmp/cmp/config" --debug
```