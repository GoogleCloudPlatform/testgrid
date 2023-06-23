import { html, TemplateResult } from 'lit';
import '../src/tab-summary.js';
import { FailingTestInfo, FailuresSummaryInfo, FailureStats, FlakyTestInfo, HealthinessStats, HealthinessSummaryInfo, TabSummaryInfo } from '../src/testgrid-dashboard-summary';

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
  const failuresSummary = {} as FailuresSummaryInfo
  const failureStats: FailureStats = {
    numFailingTests: 1
  }
  const FailingTest = {
    displayName: "TEST0",
    failCount: 1,
    passTimestamp: "today",
    failTimestamp: "today",
  } as FailingTestInfo

  failuresSummary.failureStats = failureStats
  failuresSummary.topFailingTests = [FailingTest];

  const healthinessSummary = {} as HealthinessSummaryInfo
  const healthinessStats: HealthinessStats = {
    startTimestamp: "today",
    endTimestamp: "today",
    numFlakyTests: 1,
    averageFlakiness: 0,
    previousFlakiness: 100,
  }
  const FlakyTest = {
    displayName: "TEST1",
    flakiness: 0,
  } as FlakyTestInfo

  healthinessSummary.healthinessStats = healthinessStats
  healthinessSummary.topFlakyTests = [FlakyTest]

  const tsi: TabSummaryInfo = {
    icon,
    name: 'TEST',
    overallStatus,
    detailedStatusMsg: 'Very detailed message',
    lastRunTimestamp: 'yesterday',
    lastUpdateTimestamp: 'today',
    latestGreenBuild: 'HULK!',
    dashboardName: 'TEST1',
    failuresSummary: failuresSummary,
    healthinessSummary: healthinessSummary,
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
