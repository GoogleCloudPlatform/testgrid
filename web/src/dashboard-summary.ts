import { LitElement, html } from 'lit';
// eslint-disable-next-line @typescript-eslint/no-unused-vars
import { customElement, query, state } from 'lit/decorators.js';
import { map } from 'lit/directives/map.js';
import { ListTabSummariesResponse, TabSummary } from './gen/pb/api/v1/data.js';
import { Timestamp } from './gen/google/protobuf/timestamp.js';
import '@material/mwc-button';
import './tab-summary.js';

const host = 'testgrid-data.k8s.io';

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

// TODO: generate the correct time representation
function convertResponse(ts: TabSummary) {
  const tsi: TabSummaryInfo = {
    icon: tabStatusIcon.get(ts.overallStatus)!,
    name: ts.tabName,
    overallStatus: ts.overallStatus,
    detailedStatusMsg: ts.detailedStatusMessage,
    lastUpdateTimestamp: Timestamp.toDate(
      ts.lastUpdateTimestamp!
    ).toISOString(),
    lastRunTimestamp: Timestamp.toDate(ts.lastRunTimestamp!).toISOString(),
    latestGreenBuild: ts.latestPassingBuild,
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
        `http://${host}/api/v1/dashboards/${this.input.value}/tab-summaries`
      );
      if (!response.ok) {
        throw new Error(`HTTP error: ${response.status}`);
      }
      const data = ListTabSummariesResponse.fromJson(await response.json());
      data.tabSummaries.forEach(ts => {
        const si = convertResponse(ts);
        this.tabSummariesInfo = [...this.tabSummariesInfo, si];
      });
    } catch (error) {
      console.error(`Could not get dashboard summaries: ${error}`);
    }
  }
}
