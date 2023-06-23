import { LitElement, html, css, PropertyValues } from 'lit';
import { customElement, property, state } from 'lit/decorators.js';
import { map } from 'lit/directives/map.js';
import { when } from 'lit/directives/when.js';
import { GetDashboardGroupResponse } from '../gen/pb/api/v1/data.js';
import { navigateDashboardWithoutReload, navigateGroup } from '../utils/navigation.js';
import '@material/mwc-tab';
import '@material/mwc-tab-bar';
import './testgrid-group-summary';
import '../dashboard/testgrid-dashboard-content.js';

/**
 * Class definition for the `testgrid-group-content` element.
 * Acts as a container which can toggle between group summary view or dashboard-content element.
 * Renders the tab bar with dashboard names irrespective of the view.
 */
@customElement('testgrid-group-content')
export class TestgridGroupContent extends LitElement {

  @state()
  dashboardNames: string[] = [];

  @state()
  activeIndex = 0;

  @property({ type: String })
  groupName = '';

  @property({ type: String })
  dashboardName?: string

  // tabName is not required for this component per se,
  // but we propagate it further to dashboard-content component
  @property({ type: String })
  tabName?: string

  // set the functionality when any tab is clicked on
  private onDashboardActivated(event: CustomEvent<{index: number}>) {
    const dashboardIndex = event.detail.index;
    if (dashboardIndex === this.activeIndex){
      return
    }

    this.tabName = undefined;

    // change from group view to dashboard view or between dashboard views
    if ((this.activeIndex === 0) || (this.activeIndex !== 0 && dashboardIndex !== 0)){
      this.dashboardName = this.dashboardNames[dashboardIndex];
      navigateDashboardWithoutReload(this.groupName, this.dashboardName!)
    }
    // change from dashboard view to group view
    else {
      this.dashboardName = undefined;
      navigateGroup(this.groupName, false);
    }

    this.activeIndex = dashboardIndex;
  }

  /**
   * Lit-element lifecycle method.
   * Invoked when a component is added to the document's DOM.
   */
  connectedCallback() {
    super.connectedCallback();
    this.fetchDashboardNames();
  }

  /**
   * Lit-element lifecycle method.
   * Invoked when element properties are changed.
   */
  willUpdate(changedProperties: PropertyValues<this>) {
    if (changedProperties.has('dashboardName')) {
      this.highlightIndex(this.dashboardName);
    }
  }

  /**
   * Lit-element lifecycle method.
   * Invoked on each update to perform rendering tasks.
   */
  render() {
    var tabBar = html`${
      // make sure we only render the tabs when there are tabs
      when(this.dashboardNames.length > 0, () => html`
        <mwc-tab-bar .activeIndex=${this.activeIndex} @MDCTabBar:activated="${this.onDashboardActivated}">
          ${map(
            this.dashboardNames,(name: string) => html`<mwc-tab label=${name}></mwc-tab>`
          )}
        </mwc-tab-bar>`)
    }`;
    return html`
      ${tabBar}
      ${!this.dashboardName ?
        html`<testgrid-group-summary .groupName=${this.groupName}></testgrid-group-summary>` :
        html`<testgrid-dashboard-content .groupName=${this.groupName} .dashboardName=${this.dashboardName} .tabName=${this.tabName}></testgrid-dashboard-content>`}
    `;
  }

  // fetch the tab names to populate the tab bar
  private async fetchDashboardNames() {
    try {
      const response = await fetch(
        `http://${process.env.API_HOST}:${process.env.API_PORT}/api/v1/dashboard-groups/${this.groupName}`
      );
      if (!response.ok) {
        throw new Error(`HTTP error: ${response.status}`);
      }
      const data = GetDashboardGroupResponse.fromJson(await response.json());
      var dashboardNames: string[] = [`${this.groupName}`];
      data.dashboards.forEach(dashboard => {
        dashboardNames.push(dashboard.name);
      });
      this.dashboardNames = dashboardNames;
      this.highlightIndex(this.dashboardName);
    } catch (error) {
      console.error(`Could not get dashboards for the group: ${error}`);
    }
  }

  // identify which tab to highlight on the tab bar
  private highlightIndex(dashboardName: string | undefined) {
    if (dashboardName === undefined){
      this.activeIndex = 0;
      return
    }
    var index = this.dashboardNames.indexOf(dashboardName);
    if (index > -1){
      this.activeIndex = index;
    }
  }

  static styles = css`
    mwc-tab{
      --mdc-typography-button-letter-spacing: 0;
      --mdc-tab-horizontal-padding: 12px;
      --mdc-typography-button-font-size: 0.8rem;
      --mdc-theme-primary: #4B607C;
    }

    md-divider{
      --md-divider-thickness: 3px;
    }
`;
}
