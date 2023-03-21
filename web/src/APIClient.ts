export interface Resource {
  name: string;
}

export interface ListDashboardResponse {
  dashboards: Resource[];
}

export interface APIClient {
  getDashboards(): Array<String>;
}

// TODO(chases2): Use in index
export class APIClientImpl implements APIClient {
  host: String = 'http://localhost:8080';

  public getDashboards(): Array<String> {
    const dashboards: Array<String> = [];

    fetch(`${this.host}/api/v1/dashboards`).then(async response => {
      const resp: ListDashboardResponse = await response.json();
      resp.dashboards.forEach(db => {
        dashboards.push(db.name);
      });
    });

    return dashboards;
  }
}
