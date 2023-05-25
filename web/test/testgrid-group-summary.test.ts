import {
  html,
  fixture,
  defineCE,
  unsafeStatic,
  expect,
  waitUntil,
} from '@open-wc/testing';

import { TestgridGroupSummary } from '../src/testgrid-group-summary.js';

describe('Testgrid Group Summary page', () => {
  let element: TestgridGroupSummary;
  beforeEach(async () => {
    // Need to wrap an element to apply its properties (ex. @customElement)
    // See https://open-wc.org/docs/testing/helpers/#test-a-custom-class-with-properties
    const tagName = defineCE(class extends TestgridGroupSummary {});
    const tag = unsafeStatic(tagName);
    element = await fixture(html`<${tag} .groupName=${'fake-dashboard-group-1'}></${tag}>`);
  });

  // TODO - add accessibility tests
  it('renders the table with dashboard summaries', async () => {

    // waiting dashboard summary entries are fully rendered
    await waitUntil(
      () => element.shadowRoot!.querySelector('i.material-icons'),
      'Group summary did not render dashboard summaries',
      {
        timeout: 4000,
      },
    );

    expect(element.dashboardSummaries.length).to.be.equal(2);
    // verify the summary health description
    expect(element.dashboardSummaries[0].tabDescription).to.have.string('2 / 7 PASSING');
    expect(element.dashboardSummaries[0].tabDescription).to.have.string('(2 PASSING, 4 FLAKY, 1 FAILING)');
    expect(element.dashboardSummaries[1].tabDescription).to.have.string('3 / 9 PASSING');
    expect(element.dashboardSummaries[1].tabDescription).to.have.string('(3 PASSING, 1 ACCEPTABLE, 5 FLAKY)');
  });
});
