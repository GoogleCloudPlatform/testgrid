import { ListDashboardResponse } from './gen/pb/api/v1/data.js';

export interface APIClient {
  getDashboards(): Array<String>;
}

export class APIClientImpl implements APIClient {
  host: String = 'http://localhost:8080';

  public getDashboards(): Array<String> {
    const dashboards: Array<String> = [];

    fetch(`${this.host}/api/v1/dashboards`).then(async response => {
      const resp = ListDashboardResponse.fromJson(await response.json());
      resp.dashboards.forEach(db => {
        dashboards.push(db.name);
      });
    });

    return dashboards;
  }
}
