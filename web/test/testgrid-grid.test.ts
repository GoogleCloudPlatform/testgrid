import {
  html,
  fixture,
  defineCE,
  unsafeStatic,
  expect,
  waitUntil,
} from '@open-wc/testing';

import { TestgridGrid } from '../src/testgrid-grid';

describe('Testgrid Grid page', () => {
  let element: TestgridGrid;
  beforeEach(async () => {
    // Need to wrap an element to apply its properties (ex. @customElement)
    // See https://open-wc.org/docs/testing/helpers/#test-a-custom-class-with-properties
    const tagName = defineCE(class extends TestgridGrid {});
    const tag = unsafeStatic(tagName);
    element = await fixture(html`<${tag} .dashboardName=${'fake-dashboard-1'} .tabName=${'fake_tab_3'}></${tag}>`);
  });

  // TODO - add accessibility tests
  it('renders the grid data', async () => {
    await waitUntil(
      () => element.shadowRoot!.querySelector('testgrid-grid-row'),
      'Grid display did not render grid data',
      {
        timeout: 4000,
      },
    );
    expect(element).to.exist;
    expect(element.shadowRoot?.children.length).to.be.equal(3, 'ShadowRoot children differ');
    expect(element.tabGridRows.length).to.be.equal(2, 'Number of rows differ');
    expect(element.tabGridHeaders?.headers.length).to.be.equal(5, 'Number of columns differ');
  });
});
