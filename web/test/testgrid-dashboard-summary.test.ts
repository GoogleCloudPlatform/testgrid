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

    expect(element.tabSummariesInfo).to.not.be.empty;
  });
});
