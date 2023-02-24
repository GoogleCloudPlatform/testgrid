# Config Merger
This component validates and combines two or more [config proto] files into a single TestGrid
configuration (also a config proto). This config is used by basically every other component in TestGrid.

## Local development
See also [common tips](/cmd/README.md) for running locally.

```bash
  --config-list=/tmp/testgrid/my-config-list.yaml \  # See 'Configuration List' below for details.
  # --confirm \
```

## Configuration List
The config merger requires a YAML file containing:
- a list of the configurations its trying to merge
- a location to put the final configuration

The `--config-list` flag should point to a file like this:

```yaml
target: "gs://path/to/write/config"         # Final result goes here
sources:
- name: "red"                               # Used in renaming
  location: "gs://example/red-team/config"  
  contact: "red-admin@example.com"          # Used for cross-team communication, not yet by config_merger
- name: "blue"
  location: "gs://example/blue-team/config"
  contact: "blue.team.contact@example.com"
```

### Renaming
Test Groups, Dashboards, and Dashboard Groups may be renamed to prevent
duplicates in the final config. In this case, the `name` in the config list
is added as a prefix, giving precedence by alphabetical order.

For example, if both configurations in the example above contain a dashboard 
named `"foo"`, the red dashboard will be renamed to `"red-foo"`.

[config proto]: /pb/config/config.proto