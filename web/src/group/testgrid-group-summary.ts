import { LitElement, html, css } from 'lit';
import { customElement, property, state } from 'lit/decorators.js';
import { ListDashboardSummariesResponse, DashboardSummary } from '../gen/pb/api/v1/data';
import { map } from 'lit/directives/map.js';
import { TabStatusIcon } from '../dashboard/testgrid-dashboard-summary';

/**
 * RenderedDashboardSummary defines the dashboard summary representation required for rendering
 */
interface RenderedDashboardSummary {
  name: string;
  overallStatus: string;
  icon: string;
  tabDescription: string;
}

@customElement('testgrid-group-summary')
export class TestgridGroupSummary extends LitElement {

  @property({ type: String })
  groupName = '';

  @state()
  dashboardSummaries: RenderedDashboardSummary[] = [];

  render() {
    return html`
      <link
        rel="stylesheet"
        href="https://fonts.googleapis.com/icon?family=Material+Icons"
      />
      <table>
        <thead>
          <tr>
            <th>Status</th>
            <th>Dashboard</th>
            <th>Health</th>
          </tr>
        </thead>
        <tbody>
          ${map(this.dashboardSummaries,
            (ds: RenderedDashboardSummary) => html`
              <tr>
                <td><i class="material-icons ${ds.overallStatus}">${ds.icon}</i></td>
                <td>${ds.name}</td>
                <td>${ds.tabDescription}</td>
              </tr>
            `)}
        </tbody>
      </table>
    `;
  }

  connectedCallback() {
    super.connectedCallback();
    this.fetchDashboardSummaries();
  }

  private async fetchDashboardSummaries() {
    try {
      const response = await fetch(
        `http://${process.env.API_HOST}:${process.env.API_PORT}/api/v1/dashboard-groups/${this.groupName}/dashboard-summaries`
      );
      if (!response.ok) {
        throw new Error(`HTTP error: ${response.status}`);
      }
      const data = ListDashboardSummariesResponse.fromJson(await response.json());
      var summaries: RenderedDashboardSummary[] = [];
      data.dashboardSummaries.forEach(summary => summaries.push(this.convertResponse(summary)));
      this.dashboardSummaries = summaries;
    } catch (error) {
      console.error(`Could not get grid rows: ${error}`);
    }
  }

  private convertResponse(summary: DashboardSummary){
    const sortedStatuses: string[] = [
      "PASSING",
      "ACCEPTABLE",
      "FLAKY",
      "FAILING",
      "STALE",
      "BROKEN",
      "PENDING"
    ]

    var numPassing = 0;
    var total = 0;
    for (const key in summary.tabStatusCount){
      if (key === "PASSING"){
        numPassing = summary.tabStatusCount[key];
      }
      total += summary.tabStatusCount[key];
    }

    var prefix = `${numPassing} / ${total} PASSING`;
    var descriptions: string [] = [];
    sortedStatuses.forEach(status => {
      if (summary.tabStatusCount[status] > 0){
        descriptions.push(`${summary.tabStatusCount[status]} ${status}`);
      }
    });

    if (descriptions.length >0){
      prefix += " ("+ descriptions.join(", ") +")";
    }

    const rds: RenderedDashboardSummary = {
      name: summary.name,
      overallStatus: summary.overallStatus,
      icon: TabStatusIcon.get(summary.overallStatus)!,
      tabDescription: prefix
    };

    return rds;
  }

  static styles = css`

    body{
      font-size: 12px;
    }

    .material-icons{
      font-size: 2em;
    }

    th, td {
      padding: 0.5em 2em;
    }

    thead{
      background-color: #e0e0e0;
    }

    table {
      border-radius: 6px;
      border: 1px solid #cbcbcb;
      border-spacing: 0;
    }

    .PENDING {
      color: #cc8200;
    }

    .PASSING {
      color: #0c3;
    }

    .FAILING {
      color: #a00;
    }

    .FLAKY {
      color: #609;
    }

    .ACCEPTABLE {
      color: #39a2ae;
    }

    .STALE {
      color: #808b96;
    }

    .BROKEN {
      color: #000;
    }
  `;
}
