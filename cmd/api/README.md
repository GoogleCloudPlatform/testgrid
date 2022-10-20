## TestGrid API

The TestGrid API controller exposes TestGrid data publicly that could otherwise be viewed through the UI.

### Usage

The surface of the API is described in this (proto definition)[/pb/api/v1/data.proto]. Usage is similar between protocols.

You may want to set `--scope=gs://your-bucket`. This will set this as the server's default; otherwise you'll be required to specify with each call.

#### HTTP

Use the `--http-port` option to set the listening port. Default is 8080.

You can specify further with URL parameters: `?scope=gs://your-bucket`.

`curl localhost:8080/api/v1/dashboards`

#### gRPC

Use the `--grpc-port` option to set the listening port. Default is 50051.

`grpc_cli call localhost:50051 testgrid.api.v1.TestGridData.ListDashboard`

Reflection is enabled in gRPC, allowing you to ask the server what methods are available.

`grpc_cli ls localhost:50051 testgrid.api.v1.TestGridData`