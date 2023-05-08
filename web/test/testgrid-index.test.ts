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
    expect(element.respectiveDashboards).to.be.empty;
  });

  it('fetches respective dashboards after clicking on a dashboard-group', async () => {
    // before click event, check if show (boolean) is true
    expect(element.show).to.be.true;

    await waitUntil(
      () => element.shadowRoot!.querySelector('mwc-list-item.dashboard-group'),
      'Index did not render dashboard groups',
      {
        timeout: 4000,
      },
    );

    expect(element.dashboardGroups).to.not.be.empty;

    // click on first dashboard group to fetch respective dashboards
    const dashboardGroup: ListItemBase = element.shadowRoot!.querySelector('mwc-list-item.dashboard-group')!;
    dashboardGroup.click();

    await aTimeout(3000);
    
    expect(element.show).to.be.false;
    expect(element.respectiveDashboards).to.not.be.empty;
  });

  // check the functionality of the close button
  it('renders the close button and changes the show attribute after clicking on it', async () => {
    expect(element.show).to.be.true;

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

    expect(element.show).to.be.false;

    await waitUntil(
      () => element.shadowRoot!.querySelector('mwc-button.column'),
      'Element did not render children',
      {
        timeout: 4000,
      },
    );
    
    const closeBtn: Button = element.shadowRoot!.querySelector('mwc-button.column')!;
    closeBtn.click();
    expect(element.show).to.be.true;
  });

  it('navigates to /dashboards after clicking on dashboard',async () => {
 
     await waitUntil(
       () => element.shadowRoot!.querySelector('mwc-list-item.dashboard'),
       'Index did not render dashboards',
       {
         timeout: 4000,
       },
     );
 
     // click on first dashboard group to fetch respective dashboards
     const dashboard: ListItemBase = element.shadowRoot!.querySelector('mwc-list-item.dashboard')!;
     dashboard.click();

     expect(location.pathname).to.not.equal('/');
  });
});
