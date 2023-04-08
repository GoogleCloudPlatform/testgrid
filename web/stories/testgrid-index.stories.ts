import { html, TemplateResult } from 'lit';
import '../src/TestgridIndex.js';

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

// data
const dashboards = [
  'cert-manager-jetstack-testing-janitors',
  'cert-manager-periodics-master',
  'cert-manager-periodics-release-1.10',
  'cert-manager-periodics-release-1.11',
  'cert-manager-presubmits-master',
  'conformance-all',
  'conformance-apisnoop',
  'conformance-cloud-provider-huaweicloud',
  'gardener-ci-infra',
  'gardener-dependency-watchdog',
  'gardener-extension-networking-calico',
  'google-aws',
  'google-cel',
  'google-gce',
];

const dashboardGroups = ['cert-manager', 'conformance', 'gardener', 'google'];

const respectiveDashboards = {
  'cert-manager': [
    'cert-manager-jetstack-testing-janitors',
    'cert-manager-periodics-master',
    'cert-manager-periodics-release-1.10',
    'cert-manager-periodics-release-1.11',
    'cert-manager-presubmits-master',
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

export const RespectiveDashboards: Story<ArgTypes> = ({
  backgroundColor = 'white',
}: ArgTypes) => html`
  <testgrid-index
    style="--example-app-background-color: ${backgroundColor}"
    .show=${false}
    .dashboardGroups=${[dashboardGroups[0]]}
    .respectiveDashboards=${respectiveDashboards['cert-manager']}
  ></testgrid-index>
`;
