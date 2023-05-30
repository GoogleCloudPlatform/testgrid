import { html, TemplateResult } from 'lit';
import '../src/tab-summary.js';
import { TabSummaryInfo } from '../src/testgrid-dashboard-summary';

export default {
  title: 'Tab summary',
  component: 'tab-summary',
};

interface Story<T> {
  (args: T): TemplateResult;
  args?: T;
}

interface Args {
  icon: string;
  overallStatus: string;
}

const Template: Story<Args> = ({
  icon = 'done',
  overallStatus = 'PASSING',
}: Args) => {
  const tsi: TabSummaryInfo = {
    icon,
    name: 'TEST',
    overallStatus,
    detailedStatusMsg: 'Very detailed message',
    lastRunTimestamp: 'yesterday',
    lastUpdateTimestamp: 'today',
    latestGreenBuild: 'HULK!',
    dashboardName: 'TEST1',
  };

  return html`<link
      rel="stylesheet"
      href="https://fonts.googleapis.com/icon?family=Material+Icons"
    />
    <tab-summary .info=${tsi}></tab-summary>`;
};

export const Passing = Template.bind({});

export const Flaky = Template.bind({});
Flaky.args = { icon: 'remove_circle_outline', overallStatus: 'FLAKY' };

export const Failing = Template.bind({});
Failing.args = { icon: 'warning', overallStatus: 'FAILING' };

export const Stale = Template.bind({});
Stale.args = { icon: 'error_outline', overallStatus: 'STALE' };

export const Broken = Template.bind({});
Broken.args = { icon: 'broken_image', overallStatus: 'BROKEN' };

export const Pending = Template.bind({});
Pending.args = { icon: 'schedule', overallStatus: 'PENDING' };

export const Acceptable = Template.bind({});
Acceptable.args = { icon: 'add_circle_outline', overallStatus: 'ACCEPTABLE' };
