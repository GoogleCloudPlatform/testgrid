# API

This component exposes TestGrid data publicly that could otherwise be viewed through the UI.

## Local development
See also [common tips](/cmd/README.md) for running locally.

The surface of the API is described in this (proto definition)[/pb/api/v1/data.proto]. Usage is similar between protocols.

You may want to set `--scope=gs://your-bucket`. This will set this as the server's default; otherwise you'll be required to specify with each call.

If you're using this for developing in the `web/` directory, optionally set `--allowed-origin=*` to avoid
CORS issues.

```bash
bazelisk run //cmd/api -- \
  # --scope=gs://your-bucket
  # --allowed-origin=*
```

### HTTP

Use the `--http-port` option to set the listening port. Default is 8080.

You can specify further with URL parameters: `?scope=gs://your-bucket`.

`curl localhost:8080/api/v1/dashboards`

### gRPC

Use the `--grpc-port` option to set the listening port. Default is 50051.

`grpc_cli call localhost:50051 testgrid.api.v1.TestGridData.ListDashboard`

Reflection is enabled in gRPC, allowing you to ask the server what methods are available.

`grpc_cli ls localhost:50051 testgrid.api.v1.TestGridData`