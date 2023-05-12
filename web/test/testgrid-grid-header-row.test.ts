import {
    html,
    fixture,
    defineCE,
    unsafeStatic,
    expect,
} from '@open-wc/testing';
import { TestgridGridHeaderRow } from '../src/testgrid-grid-header-row';

describe('TestGrid grid header row', () => {
    let element: TestgridGridHeaderRow;
    beforeEach(async () => {
        // Need to wrap an element to apply its properties (ex. @customElement)
        // See https://open-wc.org/docs/testing/helpers/#test-a-custom-class-with-properties
        const tagName = defineCE(class extends TestgridGridHeaderRow { });
        const tag = unsafeStatic(tagName);
        element = await fixture(html`<${tag}></${tag}>`);
    });
    it('passes the a11y audit', async () => {
        expect(element).shadowDom.to.be.accessible();
    });

    it('can instantiate an element', async () => {
        expect(element).to.exist;
    });
});
