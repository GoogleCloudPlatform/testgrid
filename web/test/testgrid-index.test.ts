import {
  html,
  fixture,
  defineCE,
  unsafeStatic,
  expect,
} from '@open-wc/testing';

import { TestgridIndex } from '../src/testgrid-index.js';

describe('ExampleApp', () => {
  let element: TestgridIndex;
  beforeEach(async () => {
    // Need to wrap an element to apply its properties (ex. @customElement)
    // See https://open-wc.org/docs/testing/helpers/#test-a-custom-class-with-properties
    const tagName = defineCE(class extends TestgridIndex {});
    const tag = unsafeStatic(tagName);
    element = await fixture(html`<${tag}></${tag}>`);
  });

  it('renders a button', async () => {
    const h1 = element.shadowRoot!.querySelector('mwc-button')!;
    expect(h1).to.exist;
  });

  it('passes the a11y audit', async () => {
    await expect(element).shadowDom.to.be.accessible();
  });

  // clicking the button fetches dashboards and dashboard groups
  it('clicking the button fetches dashboards and dashboard-groups', async () => {
    // adding a click event to the button
    const button = element.shadowRoot!.querySelector('mwc-button')!;
    button.click();

    // fetch dashboards and dashboard groups on button click
    await element.getDashboards();
    await element.getDashboardGroups();

    // check if dashboards and dashboard groups exist
    expect(element.dashboards).to.exist;
    expect(element.dashboardGroups).not.to.be.empty;
  });

  // clicking on individual dashboard-group fetches respective dashboards
  it('clicking on an individual dashboard-group fetches respective dashboards', async () => {
    // before click event, check if show (boolean) is true
    expect(element.show).to.be.true;

    // check if dashboard-groups exist
    await element.getDashboardGroups();

    expect(element.dashboardGroups).to.exist;

    // adding a click event to the dashboard-group
    const dashboardGroup = element.shadowRoot!.querySelector('mwc-list-item')!;
    dashboardGroup.click();

    await element.getRespectiveDashboards('cert-manager');

    expect(element.respectiveDashboards).to.exist;
  });

  // check the functionality of the close button
  it('check the functionality of the close button', async () => {
    // before rendering respective dashboards, show(boolean) is true
    expect(element.show).to.be.true;

    await element.getDashboardGroups();

    // on click of dashboard-group, show(boolean) is false and close button is rendered
    const dashboardGroup = element.shadowRoot!.querySelector('mwc-list-item')!;
    dashboardGroup.click();

    element.show = false;

    expect(element.show).to.be.false;
    expect(element.shadowRoot!.querySelector('mwc-button')).to.exist;

    // on click of close button, show(boolean) is true, hide the respective dashboards and close button
    const closeButton = element.shadowRoot!.querySelector('mwc-button')!;
    closeButton.click();

    element.show = true;
    expect(element.show).to.be.true;

    // check if dashboard-groups and dashboards exist
    await element.getDashboards();

    expect(element.dashboardGroups).to.exist;
    expect(element.dashboards).to.exist;
  });
});
