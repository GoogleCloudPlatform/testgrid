import { html, TemplateResult } from 'lit';
import '../src/testgrid-grid-row-header';

export default {
  title: 'Grid Row Header',
  component: 'testgrid-grid-row-header',
};

interface Story<T> {
  (args: T): TemplateResult;
  args?: T;
}

interface Args {
  name: string;
}

const Template: Story<Args> = ({
  name = '',
}: Args) => {
  return html`<testgrid-grid-row-header .name="${name}"></testgrid-grid-row-header>`;
};

export const Empty = Template.bind({});
Empty.args = {name: ''};
export const Short = Template.bind({});
Short.args = {name: '//path/to/my:test'};
export const Long = Template.bind({});
Long.args = {name: '//this/test/of/mine/and/its/path/are/quite/long/so/here/is/my:test'};
