(Modified from https://github.com/GoogleCloudPlatform/oss-test-infra/tree/master/prow/oss/terraform)

## Terraform

This directory contains terrafrom configurations for provisioning monitoring and alerting stacks on GCP for TestGrid. These are applied manually.

### Prerequisite For Provisioning

-   Terraform 0.13.1
    [Installation guide](https://www.terraform.io/downloads.html)

-   Authenticate with GCP

    ```shell
    gcloud auth login && gcloud auth application-default login
    ```

### Initial Setup (One time action)

This is done once before initial provisioning of monitoring and alerting stacks.

```shell
    gsutil mb -p k8s-testgrid gs://k8s-testgrid-metrics-terraform
    gsutil versioning set on gs://k8s-testgrid-metrics-terraform
```

### Provisioning

1.  Run `terraform init`. Terraform will automatically download the plugins
    required to execute this code. You only need to do this once per machine.

    ```shell
    terraform init
    ```

1.  Validate with:

    ```shell
    make validate
    ```

1.  Execute Terraform:

    ```shell
    terraform apply
    ```
