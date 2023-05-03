import { html, TemplateResult } from 'lit';
import '../src/testgrid-grid-column-header';

export default {
  title: 'Grid Column Header',
  component: 'testgrid-grid-column-header',
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
  return html`<testgrid-grid-column-header .name="${name}"></testgrid-grid-column-header>`;
};

export const Empty = Template.bind({});
Empty.args = {name: ''};
export const Short = Template.bind({});
Short.args = {name: '1234'};
export const Long = Template.bind({});
Long.args = {name: '1234-5678-123456789012-very-long-id'};
