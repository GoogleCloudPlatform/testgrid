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

locals {}

resource "google_monitoring_alert_policy" "probers" {
  project      = var.project
  provider     = google-beta  // To include `condition_monitoring_query_language`
  display_name = "HostDown"
  combiner     = "OR"

  conditions {
    display_name = "Host is unreachable"
    condition_monitoring_query_language {
      duration = "120s"
      query    = <<-EOT
      fetch uptime_url
      | metric 'monitoring.googleapis.com/uptime_check/check_passed'
      | align next_older(1m)
      | filter resource.project_id == '${var.project}'
      | every 1m
      | group_by [resource.host],
          [value_check_passed_not_count_true: count_true(not(value.check_passed))]
      | condition val() > 1 '1'
      EOT
      trigger {
        count = 1
      }
    }
  }

  documentation {
    content   = "Host Down"
    mime_type = "text/markdown"
  }

  # gcloud beta monitoring channels list --project=oss-prow
  notification_channels = ["projects/${var.project}/notificationChannels/${var.notification_channel_id}"]
}

resource "google_monitoring_alert_policy" "pubsub-unack-too-old" {
  project      = var.project
  provider     = google-beta  // To include `condition_monitoring_query_language`
  for_each     = var.pubsub_topics
  display_name = "pubsub-unack-too-old/${var.project}/${each.key}"
  combiner     = "OR" # required

  conditions {
    display_name = "pubsub-unack-too-old/${var.project}/${each.key}"
    
    condition_monitoring_query_language {
      duration = "60s"
      query    = <<-EOT
      fetch pubsub_subscription
      | metric 'pubsub.googleapis.com/subscription/oldest_unacked_message_age'
      | filter
          (metadata.system_labels.topic_id == '${each.key}')
      | group_by 30m,
          [value_oldest_unacked_message_age_mean:
          mean(value.oldest_unacked_message_age)]
      | every 30m
      | condition val() > 1.08e+07 's'
      EOT
      trigger {
        count = 1
      }
    }
  }

  documentation {
    content   = "${var.project}: TestGrid is not acknowledging PubSub messages in time.\n\nOncall Playbook: http://go/test-infra-playbook"
    mime_type = "text/markdown"
  }

  # gcloud beta monitoring channels list --project=oss-REPLACE
  notification_channels = ["projects/${var.project}/notificationChannels/${var.notification_channel_id}"]
}