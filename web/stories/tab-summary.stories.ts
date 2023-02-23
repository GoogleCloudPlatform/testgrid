import { html } from 'lit';
import '../src/tab-summary.js';
import { TabSummaryInfo } from '../src/dashboard-summary.js';

export default {
  title: 'Tab summary',
  component: 'tab-summary',
};
const passing: TabSummaryInfo = {
  icon: 'done',
  name: 'TEST',
  overallStatus: 'PASSING',
  detailedStatusMsg: 'Very detailed message',
  lastRunTimestamp: 'yesterday',
  lastUpdateTimestamp: 'today',
  latestGreenBuild: 'HULK!',
};
export const Passing = () =>
  html`<link
      rel="stylesheet"
      href="https://fonts.googleapis.com/icon?family=Material+Icons"
    /><tab-summary .info=${passing}></tab-summary>`;
// export const Secondary = () => html`<demo-button .background="#ff0" .label="😄👍😍💯"></demo-button>`;
// export const Tertiary = () => html`<demo-button .background="#ff0" .label="📚📕📈🤓"></demo-button>`;
