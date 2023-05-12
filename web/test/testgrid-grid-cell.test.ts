import {
    html,
    fixture,
    defineCE,
    unsafeStatic,
    expect,
} from '@open-wc/testing';
import { TestgridGridCell } from '../src/testgrid-grid-cell';
import { TestStatus } from '../src/gen/pb/test_status/test_status';

describe('TestGrid grid cell', () => {
    let element: TestgridGridCell;
    beforeEach(async () => {
        // Need to wrap an element to apply its properties (ex. @customElement)
        // See https://open-wc.org/docs/testing/helpers/#test-a-custom-class-with-properties
        const tagName = defineCE(class extends TestgridGridCell { });
        const tag = unsafeStatic(tagName);
        element = await fixture(html`<${tag}></${tag}>`);
    });
    it('passes the a11y audit', async () => {
        expect(element).shadowDom.to.be.accessible();
    });

    it('can instantiate', async () => {
        expect(element).to.exist;
        expect(element.icon).to.undefined;
        expect(element.status).to.undefined;
    });

    [
        {icon: 'P', status: TestStatus.PASS},
        {icon: 'F', status: TestStatus.FAIL},
        {icon: 'R', status: TestStatus.RUNNING},
    ].forEach(function(testCase) {
        it("renders with status " + TestStatus[testCase.status], async() => {
            const tagName = defineCE(class extends TestgridGridCell { });
            const tag = unsafeStatic(tagName);
            let el: TestgridGridCell;
            el = await fixture(html`<${tag} .icon=${testCase.icon} .status=${TestStatus[testCase.status]}></${tag}>`);
    
            expect(el).to.exist;
            expect(el.icon).to.equal(testCase.icon);
            expect(el.status).to.equal(TestStatus[testCase.status]);
        });
    });
});
