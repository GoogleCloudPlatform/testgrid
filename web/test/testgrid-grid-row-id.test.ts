import {
    html,
    fixture,
    defineCE,
    unsafeStatic,
    expect,
} from '@open-wc/testing';
import { TestgridGridRowId } from '../src/testgrid-grid-row-id';

describe('TestGrid grid row header', () => {
    let element: TestgridGridRowId;
    beforeEach(async () => {
        // Need to wrap an element to apply its properties (ex. @customElement)
        // See https://open-wc.org/docs/testing/helpers/#test-a-custom-class-with-properties
        const tagName = defineCE(class extends TestgridGridRowId { });
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
