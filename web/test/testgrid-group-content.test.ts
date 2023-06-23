import {
  html,
  fixture,
  expect,
  waitUntil,
} from '@open-wc/testing';

import { TestgridGroupContent} from '../src/group/testgrid-group-content';
import '../src/group/testgrid-group-content';
import { Tab } from '@material/mwc-tab';

describe('Testgrid Dashboard Content page', () => {

  // TODO - add accessibility tests
  it('renders the tab bar and displays the group summary', async () => {
    var tgc: TestgridGroupContent = await fixture(html`<testgrid-group-content 
      .groupName=${'fake-dashboard-group-1'}></testgrid-group-content>`);

    await waitUntil(
      () => tgc.shadowRoot!.querySelector('mwc-tab-bar'),
      'Group content element did not render the tab bar',
      {
        timeout: 4000,
      },
    );

    var summary = tgc.shadowRoot!.querySelector('testgrid-group-summary')!;
    var dashboard = tgc.shadowRoot!.querySelector('testgrid-dashboard-content');
    expect(tgc.dashboardNames).to.not.be.empty;
    expect(tgc.activeIndex).to.equal(0);
    expect(summary).to.not.be.null;
    expect(dashboard).to.be.null;
  });

  it('renders the dashboard content when dashboardName is provided', async () => {
    var tgc: TestgridGroupContent = await fixture(html`<testgrid-group-content 
      .groupName=${'fake-dashboard-group-1'} .dashboardName=${'fake-dashboard-1'}></testgrid-group-content>`);

    await waitUntil(
      () => tgc.shadowRoot!.querySelector('mwc-tab-bar'),
      'Group content element did not render the tab bar',
      {
        timeout: 4000,
      },
    );

    var summary = tgc.shadowRoot!.querySelector('testgrid-group-summary')!;
    var dashboard = tgc.shadowRoot!.querySelector('testgrid-dashboard-content');
    expect(tgc.dashboardNames).to.not.be.empty;
    expect(tgc.activeIndex).to.equal(1);
    expect(summary).to.be.null;
    expect(dashboard).to.not.be.null;
  });

  it('renders the dashboard content and updates URL if a tab is clicked', async() => {
    var tgc: TestgridGroupContent = await fixture(html`<testgrid-group-content 
      .groupName=${'fake-dashboard-group-1'}></testgrid-group-content>`);

    await waitUntil(
      () => tgc.shadowRoot!.querySelector('mwc-tab-bar'),
      'Group content element did not render the tab bar',
      {
        
        timeout: 4000,
      },
    );

    const dashboardTabs: NodeListOf<Tab>| null = tgc.shadowRoot!.querySelectorAll('mwc-tab');
    dashboardTabs[1]!.click();

    await waitUntil(
      () => tgc.shadowRoot!.querySelector('testgrid-dashboard-content'),
      'Group content element did not render the dashboard content element',
      {
        timeout: 4000,
      },
    );
    
    var summary = tgc.shadowRoot!.querySelector('testgrid-group-summary')!;
    var dashboard = tgc.shadowRoot!.querySelector('testgrid-dashboard-content');
    expect(tgc.dashboardNames).to.not.be.undefined;
    expect(tgc.activeIndex).to.equal(1);
    expect(summary).to.be.null;
    expect(dashboard).to.not.be.null;
    expect(location.pathname).to.contain('/dashboards');
  });
});
