import {
    html,
    fixture,
    defineCE,
    unsafeStatic,
    expect,
} from '@open-wc/testing';
import { TestgridGridColumnHeader } from '../src/testgrid-grid-column-header';

describe('TestGrid grid column header', () => {
    let element: TestgridGridColumnHeader;
    beforeEach(async () => {
        // Need to wrap an element to apply its properties (ex. @customElement)
        // See https://open-wc.org/docs/testing/helpers/#test-a-custom-class-with-properties
        const tagName = defineCE(class extends TestgridGridColumnHeader { });
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
