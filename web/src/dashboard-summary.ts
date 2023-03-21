/* eslint-disable camelcase */
import { LitElement, html } from 'lit';
// eslint-disable-next-line @typescript-eslint/no-unused-vars
import { customElement, query, state } from 'lit/decorators.js';
import { map } from 'lit/directives/map.js';
import '@material/mwc-button';
import './tab-summary.js';

// TODO(chases2): Split into API client and view object
// API Object
export interface TabSummary {
  overall_status: string;
  tab_name: string;
  detailed_status_message: string;
  latest_passing_build: string;
  last_update_timestamp: string;
  last_run_timestamp: string;
}

// API Object
export interface ListTabSummariesResponse {
  tab_summaries: TabSummary[];
}

// Controller Object
export interface TabSummaryInfo {
  icon: string;
  name: string;
  overallStatus: string;
  detailedStatusMsg: string;
  lastUpdateTimestamp: string;
  lastRunTimestamp: string;
  latestGreenBuild: string;
}

// TODO: define in a shared file (dashboard group also uses this)
const tabStatusIcon = new Map<string, string>([
  ['PASSING', 'done'],
  ['FAILING', 'warning'],
  ['FLAKY', 'remove_circle_outline'],
  ['STALE', 'error_outline'],
  ['BROKEN', 'broken_image'],
  ['PENDING', 'schedule'],
  ['ACCEPTABLE', 'add_circle_outline'],
]);

function convertResponse(ts: TabSummary) {
  const tsi: TabSummaryInfo = {
    icon: tabStatusIcon.get(ts.overall_status)!,
    name: ts.tab_name,
    overallStatus: ts.overall_status,
    detailedStatusMsg: ts.detailed_status_message,
    lastUpdateTimestamp: new Date(ts.last_update_timestamp).toISOString(),
    lastRunTimestamp: new Date(ts.last_run_timestamp).toISOString(),
    latestGreenBuild: ts.latest_passing_build,
  };
  return tsi;
}

@customElement('dashboard-summary')
// eslint-disable-next-line @typescript-eslint/no-unused-vars
export class DashboardSummary extends LitElement {
  @state()
  private tabSummariesInfo: Array<TabSummaryInfo> = [];

  render() {
    return html`
      ${map(
        this.tabSummariesInfo,
        (ts: TabSummaryInfo) => html`<tab-summary .info=${ts}></tab-summary>`
      )}
      <input id="dashboard-name-input" label="Dashboard name" />
      <mwc-button raised @click="${this.getTabSummaries}">Fetch</mwc-button>
    `;
  }

  @query('#dashboard-name-input')
  input!: HTMLInputElement;

  async getTabSummaries() {
    this.tabSummariesInfo = [];
    try {
      const response = await fetch(
        `http://localhost:8080/api/v1/dashboards/${this.input.value}/tab-summaries`
      );
      if (!response.ok) {
        throw new Error(`HTTP error: ${response.status}`);
      }
      const data: ListTabSummariesResponse = await response.json();
      data.tab_summaries.forEach(ts => {
        const si = convertResponse(ts);
        this.tabSummariesInfo = [...this.tabSummariesInfo, si];
      });
    } catch (error) {
      console.error(`Could not get dashboard summaries: ${error}`);
    }
  }
}
