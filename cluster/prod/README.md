# Add a new component to the release

1. Create the component, such that no other components depend on it. (If it fails, you can debug without worrying about breaking production.)
1. **PR**: Add the component to the BUILD file, and send a PR for this. (Example: https://github.com/GoogleCloudPlatform/testgrid/pull/822).
1. Wait until Container Registry has a date-versioned image for the component.
    - e.g. https://github.com/GoogleCloudPlatform/testgrid/pull/822 -> tabulator image with version`v20220210-v0.0.121-16-g5bceb0c`
1. **PR**: Create a configuration `<component>.yaml` file under `cluster/canary`, referencing the image version all components are using in production (so the autobumper can catch it).
1. Bind the service account(s) for the component in the `testgrid-canary` namespace:

    ```
    NAMESPACE=testgrid-canary
    PROJECT=<project>
    COMPONENT=<component name>
    SERVICE_ACCOUNT=<service account to bind to>
    gcloud iam service-accounts add-iam-policy-binding   --project=$PROJECT   --role=roles/iam.workloadIdentityUser   --member=serviceAccount:k8s-testgrid.svc.id.goog[$NAMESPACE/$COMPONENT]   $SERVICE_ACCOUNT
    ```
1. **PR**: Create `<component>.yaml` under `cluster/prod`.
1. Bind the service account(s) for the component in the `testgrid` namespace:

    ```
    NAMESPACE=testgrid
    PROJECT=<project>
    COMPONENT=<component name>
    SERVICE_ACCOUNT=<service account to bind to>
    gcloud iam service-accounts add-iam-policy-binding   --project=$PROJECT   --role=roles/iam.workloadIdentityUser   --member=serviceAccount:k8s-testgrid.svc.id.goog[$NAMESPACE/$COMPONENT]   $SERVICE_ACCOUNT
    ```