import { html, TemplateResult } from 'lit';
import '../src/ExampleApp.js';

export default {
  title: 'ExampleApp',
  component: 'example-app',
  argTypes: {
    backgroundColor: { control: 'color' },
  },
};

interface Story<T> {
  (args: T): TemplateResult;
  args?: Partial<T>;
  argTypes?: Record<string, unknown>;
}

interface ArgTypes {
  title?: string;
  backgroundColor?: string;
}

const Template: Story<ArgTypes> = ({
  title,
  backgroundColor = 'white',
}: ArgTypes) => html`
  <example-app
    style="--example-app-background-color: ${backgroundColor}"
    .title=${title || ''}
  ></example-app>
`;

export const App = Template.bind({});
App.args = {
  title: 'My app',
};
