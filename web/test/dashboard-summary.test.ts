import { Button } from '@material/mwc-button';
import { ListItemBase } from '@material/mwc-list/mwc-list-item-base.js';
import {
  html,
  fixture,
  defineCE,
  unsafeStatic,
  expect,
  waitUntil,
  aTimeout,
} from '@open-wc/testing';

import { DashboardSummary } from '../src/dashboard-summary.js';

describe('Testgrid Dashboard Summary page', () => {
  let element: DashboardSummary;
  beforeEach(async () => {
    // Need to wrap an element to apply its properties (ex. @customElement)
    // See https://open-wc.org/docs/testing/helpers/#test-a-custom-class-with-properties
    const tagName = defineCE(class extends DashboardSummary {});
    const tag = unsafeStatic(tagName);
    element = await fixture(html`<${tag} .name=${"sig-autoscaling-hpa"}></${tag}>`);
  });

  it('passes the a11y audit', async () => {
    await expect(element).shadowDom.to.be.accessible();
  });

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

  it('renders the tab bar with tabs', async () => {

    // waiting until list items (dashboards and groups) are fully rendered
    await waitUntil(
      () => element.shadowRoot!.querySelector('mwc-tab'),
      'Dashboard summary did not render tabs within the tab bar',
      {
        timeout: 4000,
      },
    );

    expect(element.tabNames).to.not.be.empty;
    expect(element.tabNames.length).to.not.equal(1);
  });
});
