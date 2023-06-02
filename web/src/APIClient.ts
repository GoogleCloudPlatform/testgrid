import {
  ListDashboardsResponse,
  ListDashboardGroupsResponse,
} from './gen/pb/api/v1/data.js';

export interface APIClient {
  getDashboards(): Array<String>;
  getDashboardGroups(): Array<String>;
}

export class APIClientImpl implements APIClient {
  host: String = 'testgrid-data.k8s.io';

  public getDashboards(): Array<String> {
    const dashboards: Array<String> = [];

    fetch(`${this.host}/api/v1/dashboards`).then(async response => {
      const resp = ListDashboardsResponse.fromJson(await response.json());
      resp.dashboards.forEach(db => {
        dashboards.push(db.name);
      });
    });

    return dashboards;
  }

  public getDashboardGroups(): Array<String> {
    const dashboardGroups: Array<String> = [];

    fetch(`${this.host}/api/v1/dashboard-groups`).then(async response => {
      const resp = ListDashboardGroupsResponse.fromJson(await response.json());
      resp.dashboardGroups.forEach(db => {
        dashboardGroups.push(db.name);
      });
    });

    return dashboardGroups;
  }
}
