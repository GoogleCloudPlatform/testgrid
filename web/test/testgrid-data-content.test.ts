import {
  html,
  fixture,
  defineCE,
  unsafeStatic,
  expect,
  waitUntil,
} from '@open-wc/testing';

import { TestgridDataContent} from '../src/testgrid-data-content';
import { Tab } from '@material/mwc-tab';

describe('Testgrid Data Content page', () => {
  let element: TestgridDataContent;
  beforeEach(async () => {
    // Need to wrap an element to apply its properties (ex. @customElement)
    // See https://open-wc.org/docs/testing/helpers/#test-a-custom-class-with-properties
    const tagName = defineCE(class extends TestgridDataContent {});
    const tag = unsafeStatic(tagName);
    element = await fixture(html`<${tag} .dashboardName=${'fake-dashboard-5'}></${tag}>`);
  });

  it('fetches the tab names and renders the tab bar', async () => {
    await waitUntil(
      () => element.shadowRoot!.querySelector('mwc-tab-bar'),
      'Data content did not render the tab bar',
      {
        timeout: 4000,
      },
    );

    expect(element.tabNames).to.not.be.empty;
  });

  it('renders the dashboard summary if `showTab` attribute is not passed', async () => {
    await waitUntil(
      () => element.shadowRoot!.querySelector('mwc-tab-bar'),
      'Data content did not render the tab bar',
      {
        timeout: 4000,
      },
    );

    expect(element.tabNames).to.not.be.empty;
    expect(element.activeIndex).to.equal(0);
  });

  it('renders the grid display if a tab is clicked', async() => {

    await waitUntil(
      () => element.shadowRoot!.querySelector('mwc-tab'),
      'Data content did not render the tab bar',
      {
        
        timeout: 4000,
      },
    );

    const tab: NodeListOf<Tab>| null = element.shadowRoot!.querySelectorAll('mwc-tab');
    console.log(tab[1])
    tab[1]!.click();

    await waitUntil(
      () => element.shadowRoot!.querySelector('testgrid-grid-display'),
      'Data content did not render the grid data',
      {
        timeout: 4000,
      },
    );

    expect(element.activeIndex).to.not.equal(0);
    expect(element.showTab).to.equal(true);
  });
});
