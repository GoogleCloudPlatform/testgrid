import { html, TemplateResult } from 'lit';
import '../src/testgrid-grid';

export default {
  title: 'Grid',
  component: 'testgrid-grid',
};

interface Story<T> {
  (args: T): TemplateResult;
  args?: T;
}

interface Args {
  dashboardName: string;
  tabName: String;
}

const Template: Story<Args> = ({
  dashboardName = '', tabName = '',
}: Args) => {
  return html`<testgrid-grid .dashboardName="${dashboardName}" .tabName="${tabName}"></testgrid-grid>`;
};

export const Grid = Template.bind({});
Grid.args = {dashboardName: 'fake-dashboard-1', tabName: 'fake_tab_3'}
