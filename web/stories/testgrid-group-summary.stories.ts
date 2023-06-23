import { html, TemplateResult } from 'lit';
import '../src/group/testgrid-group-summary';

export default {
  title: 'Dashboard Group View',
  component: 'testgrid-group-summary',
};

interface Story<T> {
  (args: T): TemplateResult;
  args?: T;
}

interface Args {
  groupName: string;
}

const Template: Story<Args> = ({
  groupName = '',
}: Args) => {
  return html`
    <link
      rel="stylesheet"
      href="https://fonts.googleapis.com/icon?family=Material+Icons"
    />
    <testgrid-group-summary .groupName="${groupName}" "></testgrid-group-summary>`;
};

export const DashboardGroup = Template.bind({});
DashboardGroup.args = {groupName:'fake-dashboard-group-1'}
