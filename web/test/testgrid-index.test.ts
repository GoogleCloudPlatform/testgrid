import { Button } from '@material/mwc-button';
import { ListItemBase } from '@material/mwc-list/mwc-list-item-base.js';
import {
  html,
  fixture,
  defineCE,
  unsafeStatic,
  expect,
  waitUntil,
} from '@open-wc/testing';

import { TestgridIndex } from '../src/testgrid-index.js';

describe('Testgrid Index page', () => {
  let element: TestgridIndex;
  beforeEach(async () => {
    // Need to wrap an element to apply its properties (ex. @customElement)
    // See https://open-wc.org/docs/testing/helpers/#test-a-custom-class-with-properties
    const tagName = defineCE(class extends TestgridIndex {});
    const tag = unsafeStatic(tagName);
    element = await fixture(html`<${tag}></${tag}>`);
  });

  // TODO - add accessibility tests
  it('fetches dashboards and dashboard-groups after loading the component', async () => {

    // waiting until list items (dashboards and groups) are fully rendered
    await waitUntil(
      () => element.shadowRoot!.querySelector('mwc-list-item.dashboard'),
      'Index did not render dashboards',
      {
        timeout: 5000,
      },
    );

    await waitUntil(
      () => element.shadowRoot!.querySelector('mwc-list-item.dashboard-group'),
      'Index did not render dashboard groups',
      {
        timeout: 5000,
      },
    );
    // check if dashboards and dashboard groups exist
    expect(element.dashboards).to.not.be.empty;
    expect(element.dashboardGroups).not.to.be.empty;
  });

  it('navigates to /groups/{group} after clicking on dashboard',async () => {
 
    await waitUntil(
      () => element.shadowRoot!.querySelector('mwc-list-item.dashboard-group'),
      'Index did not render dashboard groups',
      {
        timeout: 4000,
      },
    );
 
    // click on first dashboard group to fetch respective dashboards
    const dashboardGroup: ListItemBase = element.shadowRoot!.querySelector('mwc-list-item.dashboard-group')!;
    dashboardGroup.click();

    expect(location.pathname).to.include('groups');
    expect(location.pathname.split('/').length).to.equal(3);
  });
});
