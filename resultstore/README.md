# TestGrid ResultStore
The ResultStore client creates a CRUD interface for users to interact with the Google ResultStore.

## ResultStore invocation searching
The ResultStore client allows users to directly search invocations stored in a
GCP project given a query. The query format is documented in the
[SearchInvocationsRequest
type](https://godoc.org/google.golang.org/genproto/googleapis/devtools/resultstore/v2#SearchInvocationsRequest).
Search will return a list of `resultstore.Invocation` that satisfies the query condition.

Sample search code snippet
```go
conn, err := resultstore.Connect(ctx, serviceAccountPath)
if err != nil {
  // error handling
}
client := resultstore.NewClient(conn).WithContext(ctx)
invocationClient := client.Invocations()

projectID := "GCP Project ID"
queryTime := time.Unix(1567800000, 0).Format(time.RFC3339)
query := fmt.Sprintf("timing.start_time>%q", queryTime)
invocationsFieldMask := []string{
  "invocations.name",
  "invocations.timing",
  "next_page_token",
}
result, err := invocationClient.Search(ctx, projectID, query, invocationsFieldMask...)
```

