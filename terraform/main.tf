# Copyright 2022 The TestGrid Authors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

# Store terraform states in GCS
terraform {
  backend "gcs" {
    bucket = "k8s-testgrid-metrics-terraform"
  }
}

module "alert" {
  source = "./modules/alerts"

  project = "k8s-testgrid"
  # gcloud alpha monitoring channels list --project=k8s-testgrid
  # grep displayName: Michelle
  notification_channel_id = "12611470047778396886"

  blackbox_probers = [
    // Production
    // Check both the original and the aliased URLs
    "k8s-testgrid.appspot.com",
    "testgrid.k8s.io",
    // Canary
    "external-canary-dot-k8s-testgrid.appspot.com",
  ]

  pubsub_topics = [
    "canary-testgrid",
    "testgrid",
  ]
}