import { html, TemplateResult } from 'lit';
import '../src/TestgridIndex.js';

export default {
  title: 'Index',
  component: 'testgrid-index',
  // argTypes: {
  //   backgroundColor: { control: 'color' },
  // },
};

interface Story<T> {
  (args: T): TemplateResult;
  args?: Partial<T>;
  // argTypes?: Record<string, unknown>;
}

interface ArgTypes {
  backgroundColor?: string;
}

const Template: Story<ArgTypes> = ({
  backgroundColor = 'white',
}: ArgTypes) => html`
  <testgrid-index
    style="--example-app-background-color: ${backgroundColor}"
  ></testgrid-index>
`;

export const App = Template.bind({});
App.args = {
  backgroundColor: '#ededed',
};
