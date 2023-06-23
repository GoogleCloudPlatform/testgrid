import { html, TemplateResult } from 'lit';
import '../src/testgrid-index.js';

export default {
  title: 'Index',
  component: 'testgrid-index',
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
  backgroundColor?: string;
}

// dummy data
const dashboards: Array<string> = [
  // dashboard-group_name-id
  'dashboard-g1-1',
  'dashboard-g1-2',
  'dashboard-g1-3',
  'dashboard-g1-4',
  'dashboard-g2-5',
  'dashboard-g2-6',
  'dashboard-g3-7',
  'dashboard-g3-8',
  'dashboard-g4-9',
  'dashboard-10',
];

const dashboardGroups: Array<string> = [
  'Group-1',
  'Group-2',
  'Group-3',
  'Group-4',
];

const respectiveDashboards: Record<string, Array<string>> = {
  'Group-1': [
    'dashboard-g1-1',
    'dashboard-g1-2',
    'dashboard-g1-3',
    'dashboard-g1-4',
  ],
};

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

export const Dashboards: Story<ArgTypes> = ({
  backgroundColor = 'white',
}: ArgTypes) => html`
  <testgrid-index
    style="--example-app-background-color: ${backgroundColor}"
    .dashboards=${dashboards}
  ></testgrid-index>
`;

export const DashboardGroups: Story<ArgTypes> = ({
  backgroundColor = 'white',
}: ArgTypes) => html`
  <testgrid-index
    style="--example-app-background-color: ${backgroundColor}"
    .dashboardGroups=${dashboardGroups}
  ></testgrid-index>
`;
