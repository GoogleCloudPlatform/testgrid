import {
  html,
  fixture,
  expect,
  waitUntil,
} from '@open-wc/testing';

import { TestgridDashboardContent} from '../src/dashboard/testgrid-dashboard-content';
import '../src/dashboard/testgrid-dashboard-content';
import { Tab } from '@material/mwc-tab';

describe('Testgrid Dashboard Content page', () => {

  // TODO - add accessibility tests
  it('renders the tab bar and displays the dashboard summary', async () => {
    var tdc: TestgridDashboardContent = await fixture(html`<testgrid-dashboard-content 
      .groupName=${'fake-dashboard-group-1'} .dashboardName=${'fake-dashboard-1'}></testgrid-dashboard-content>`);

    await waitUntil(
      () => tdc.shadowRoot!.querySelector('mwc-tab-bar'),
      'Dashboard content element did not render the tab bar',
      {
        timeout: 4000,
      },
    );

    var summary = tdc.shadowRoot!.querySelector('testgrid-dashboard-summary')!;
    var grid = tdc.shadowRoot!.querySelector('testgrid-grid');
    expect(tdc.activeIndex).to.equal(0);
    expect(tdc.tabNames).to.not.be.empty;
    expect(summary).to.not.be.null;
    expect(grid).to.be.null;
  });

  it('renders the grid when tabName is provided', async () => {
    var tdc: TestgridDashboardContent = await fixture(html`<testgrid-dashboard-content 
      .groupName=${'fake-dashboard-group-1'} .dashboardName=${'fake-dashboard-1'} .tabName=${'fake-tab-1'}></testgrid-dashboard-content>`);

    await waitUntil(
      () => tdc.shadowRoot!.querySelector('mwc-tab-bar'),
      'Dashboard content element did not render the tab bar',
      {
        timeout: 4000,
      },
    );

    var summary = tdc.shadowRoot!.querySelector('testgrid-dashboard-summary')!;
    var grid = tdc.shadowRoot!.querySelector('testgrid-grid');
    expect(tdc.tabNames).to.not.be.empty;
    expect(tdc.activeIndex).to.equal(1);
    expect(summary).to.be.null;
    expect(grid).to.not.be.null;
  });

  it('renders the grid display and updates URL if a tab is clicked', async() => {
    var tdc: TestgridDashboardContent = await fixture(html`<testgrid-dashboard-content 
      .groupName=${'fake-dashboard-group-1'} .dashboardName=${'fake-dashboard-1'}></testgrid-dashboard-content>`);

    await waitUntil(
      () => tdc.shadowRoot!.querySelector('mwc-tab-bar'),
      'Dashboard content element did not render the tab bar',
      {
        
        timeout: 4000,
      },
    );

    const tabs: NodeListOf<Tab>| null = tdc.shadowRoot!.querySelectorAll('mwc-tab');
    tabs[1]!.click();

    await waitUntil(
      () => tdc.shadowRoot!.querySelector('testgrid-grid'),
      'Dashboard content element did not render the grid data',
      {
        timeout: 4000,
      },
    );
    
    var summary = tdc.shadowRoot!.querySelector('testgrid-dashboard-summary')!;
    var grid = tdc.shadowRoot!.querySelector('testgrid-grid');
    expect(tdc.activeIndex).to.equal(1);
    expect(tdc.tabName).to.not.be.undefined;
    expect(summary).to.be.null;
    expect(grid).to.not.be.null;
    expect(location.pathname).to.contain('/tabs');
  });
});
