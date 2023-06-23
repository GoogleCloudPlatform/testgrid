import {
  html,
  fixture,
  defineCE,
  unsafeStatic,
  expect,
  waitUntil,
  aTimeout,
} from '@open-wc/testing';

import { TestgridDashboardSummary } from '../src/testgrid-dashboard-summary.js';

describe('Testgrid Dashboard Summary page', () => {
  let element: TestgridDashboardSummary;
  beforeEach(async () => {
    // Need to wrap an element to apply its properties (ex. @customElement)
    // See https://open-wc.org/docs/testing/helpers/#test-a-custom-class-with-properties
    const tagName = defineCE(class extends TestgridDashboardSummary {});
    const tag = unsafeStatic(tagName);
    element = await fixture(html`<${tag} .dashboardName=${'fake-dashboard-1'}></${tag}>`);
  });

  // TODO - add accessibility tests
  it('renders the tab summaries', async () => {

    // waiting until list items (dashboards and groups) are fully rendered
    await waitUntil(
      () => element.shadowRoot!.querySelector('tab-summary'),
      'Dashboard summary did not render tab summaries',
      {
        timeout: 4000,
      },
    );

    expect(element.tabSummariesInfo.length).to.equal(4);
    expect(element.tabSummariesInfo[0].name).to.equal("fake-tab-1");
    expect(element.tabSummariesInfo[0].overallStatus).to.equal("PASSING");
    expect(element.tabSummariesInfo[0].failuresSummary?.failureStats.numFailingTests).to.equal(1);
    expect(element.tabSummariesInfo[0].failuresSummary?.topFailingTests[0].failCount).to.equal(1);
    expect(element.tabSummariesInfo[0].failuresSummary?.topFailingTests[0].displayName).to.equal("fake-test-1");
    expect(element.tabSummariesInfo[0].healthinessSummary?.topFlakyTests[0].displayName).to.equal("fake-test-2");
    expect(element.tabSummariesInfo[0].healthinessSummary?.topFlakyTests[0].flakiness).to.equal(2);
    expect(element.tabSummariesInfo[0].healthinessSummary?.healthinessStats.numFlakyTests).to.equal(2);
  });
});
