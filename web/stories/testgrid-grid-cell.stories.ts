import { html, TemplateResult } from 'lit';
import { TestStatus } from '../src/gen/pb/test_status/test_status';
import '../src/testgrid-grid-cell';

export default {
  title: 'Grid Cell',
  component: 'testgrid-grid-cell',
};

interface Story<T> {
  (args: T): TemplateResult;
  args?: T;
}

interface Args {
  icon: string;
  status: String;
}

const Template: Story<Args> = ({
  icon = '', status = '',
}: Args) => {
  return html`<testgrid-grid-cell .icon="${icon}" .status="${status}"></testgrid-grid-cell>`;
};

export const NoResult = Template.bind({});
NoResult.args = {icon: '', status: TestStatus[TestStatus.NO_RESULT]};
export const Pass = Template.bind({});
Pass.args = {icon: '', status: TestStatus[TestStatus.PASS]};
export const PassWithErrors = Template.bind({});
PassWithErrors.args = {icon: 'E', status: TestStatus[TestStatus.PASS_WITH_ERRORS]};
export const PassWithSkips = Template.bind({});
PassWithSkips.args = {icon: 'S', status: TestStatus[TestStatus.PASS_WITH_SKIPS]};
export const Running = Template.bind({});
Running.args = {icon: 'R', status: TestStatus[TestStatus.RUNNING]};
export const CategorizedAbort = Template.bind({});
CategorizedAbort.args = {icon: 'A', status: TestStatus[TestStatus.CATEGORIZED_ABORT]};
export const Unknown = Template.bind({});
Unknown.args = {icon: '?', status: TestStatus[TestStatus.UNKNOWN]};
export const Cancel = Template.bind({});
Cancel.args = {icon: 'C', status: TestStatus[TestStatus.CANCEL]};
export const Blocked = Template.bind({});
Blocked.args = {icon: 'B', status: TestStatus[TestStatus.BLOCKED]};
export const TimedOut = Template.bind({});
TimedOut.args = {icon: 'T', status: TestStatus[TestStatus.TIMED_OUT]};
export const CategorizedFail = Template.bind({});
CategorizedFail.args = {icon: 'F', status: TestStatus[TestStatus.CATEGORIZED_FAIL]};
export const BuildFail = Template.bind({});
BuildFail.args = {icon: 'X', status: TestStatus[TestStatus.BUILD_FAIL]};
export const Fail = Template.bind({});
Fail.args = {icon: '', status: TestStatus[TestStatus.FAIL]};
export const Flaky = Template.bind({});
Flaky.args = {icon: '1/2', status: TestStatus[TestStatus.FLAKY]};
export const ToolFail = Template.bind({});
ToolFail.args = {icon: '!', status: TestStatus[TestStatus.TOOL_FAIL]};
export const BuildPassed = Template.bind({});
BuildPassed.args = {icon: '', status: TestStatus[TestStatus.BUILD_PASSED]};
