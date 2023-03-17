# TestGrid HTTP API
Valid responses are all in JSON. Error responses may not be in JSON. Replace things in curly braces.

Exact API definitions can be found on [GitHub](https://github.com/GoogleCloudPlatform/testgrid/blob/master/pb/api/v1/data.proto).

## LIST
"List" methods use the GET HTTP verb. See https://cloud.google.com/apis/design/standard_methods for details.

- /api/v1/dashboards - List dashboards
- /api/v1/dashboard-groups - List dashboard groups
- /api/v1/dashboards/{dashboard}/tabs - List a dashboard's tabs
- /api/v1/dashboard-groups/{dashboard-group} - List the dashboards in a dashboard group
- /api/v1/dashboards/{dashboard}/tab-summaries - List the tab summaries for the dashboard (data rendered in dashboard view)
- /api/v1/dashboard-groups/{dashboard-group}/dashboard-summaries - List the dashboard summaries for the dashboard group (data rendered in dashboard group view)

## GET
- /api/v1/dashboards/{dashboard} - Returns a dashboard's configuration. Often empty; dashboard-level configuration is rare.
- /api/v1/dashboards/{dashboard}/tabs/{tab}/headers - Returns the headers for a tab's grid result
- /api/v1/dashboards/{dashboard}/tabs/{tab}/rows - Returns information on a tab's rows and the data within those rows.
- /api/v1/dashboards/{dashboard}/tab-summaries/{tab} - Returns the summary for a particular tab in the given dashboard
- /api/v1/dashboards/{dashboard}/summary - Returns the aggregated summary for a particular dashboard.