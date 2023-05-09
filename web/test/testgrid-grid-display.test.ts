import {
  html,
  fixture,
  defineCE,
  unsafeStatic,
  expect,
  waitUntil,
} from '@open-wc/testing';

import { TestgridGridDisplay } from '../src/testgrid-grid-display';

describe('Testgrid Grid Display page', () => {
  let element: TestgridGridDisplay;
  beforeEach(async () => {
    // Need to wrap an element to apply its properties (ex. @customElement)
    // See https://open-wc.org/docs/testing/helpers/#test-a-custom-class-with-properties
    const tagName = defineCE(class extends TestgridGridDisplay {});
    const tag = unsafeStatic(tagName);
    element = await fixture(html`<${tag} .dashboardName=${'fake-dashboard-3'} .tabName=${'fake_tab'}></${tag}>`);
  });

  it('renders the grid data', async () => {
    await waitUntil(
      () => element.shadowRoot!.querySelector('p'),
      'Grid display did not render grid data',
      {
        timeout: 4000,
      },
    );
    expect(element.tabGridRows).to.not.be.empty;
  });
});
