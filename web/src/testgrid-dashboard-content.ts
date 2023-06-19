import { LitElement, html, css, PropertyValues } from 'lit';
import { customElement, property, state } from 'lit/decorators.js';
import { map } from 'lit/directives/map.js';
import { when } from 'lit/directives/when.js';
import { navigateDashboardWithoutReload, navigateTabWithoutReload } from './utils/navigation.js';
import { ListDashboardTabsResponse } from './gen/pb/api/v1/data.js';
import '@material/mwc-tab';
import '@material/mwc-tab-bar';
import './testgrid-dashboard-summary';
import './testgrid-grid';

/**
 * Class definition for the `testgrid-data-content` element.
 * Acts as a container for dashboard summary or grid data.
 * Renders the tab bar with tab names irrespective of the view.
 */
@customElement('testgrid-dashboard-content')
export class TestgridDashboardContent extends LitElement {

  @state()
  tabNames: string[] = [];

  @state()
  activeIndex = 0;

  @property({ type: String})
  groupName = '';

  @property({ type: String })
  dashboardName = '';

  @property({ type: String })
  tabName?: string;

  // set the functionality when any tab is clicked on
  private onTabActivated(event: CustomEvent<{index: number}>) {
    const tabIndex = event.detail.index;
    if (tabIndex === this.activeIndex){
      return
    }

    // change from dashboard view to grid view or between grid views
    if ((this.activeIndex === 0) || (this.activeIndex !== 0 && tabIndex !== 0)){
      this.tabName = this.tabNames[tabIndex];
      navigateTabWithoutReload(this.groupName, this.dashboardName, this.tabName!)
    }
    // change from dashboard view to group view
    else {
      this.tabName = undefined;
      navigateDashboardWithoutReload(this.groupName, this.dashboardName);
    }

    this.activeIndex = tabIndex;
  }

  /**
   * Lit-element lifecycle method.
   * Invoked when a component is added to the document's DOM.
   */
  connectedCallback() {
    super.connectedCallback();
    window.addEventListener('tab-changed', (evt: Event) => {
      this.tabName = (<CustomEvent>evt).detail.tabName;
      this.highlightIndex(this.tabName);
      navigateTabWithoutReload(this.groupName, this.dashboardName, this.tabName!);
    });
  }

  /**
   * Lit-element lifecycle method.
   * Invoked when element properties are changed.
   */
  willUpdate(changedProperties: PropertyValues<this>) {
    if (changedProperties.has('dashboardName') && changedProperties.has('tabName')) {
      this.fetchTabNames();
    }
    else if (changedProperties.has('dashboardName')){
      this.tabName = undefined;
      this.fetchTabNames();
    }
    else if (changedProperties.has('tabName')){
      this.highlightIndex(this.tabName);
    }
  }

  /**
   * Lit-element lifecycle method.
   * Invoked on each update to perform rendering tasks.
   */
  render() {
    var tabBar = html`${
      // make sure we only render the tabs when there are tabs
      when(this.tabNames.length > 0, () => html`
        <mwc-tab-bar .activeIndex=${this.activeIndex} @MDCTabBar:activated="${this.onTabActivated}">
          ${map(
            this.tabNames,(name: string) => html`<mwc-tab label=${name}></mwc-tab>`
          )}
        </mwc-tab-bar>`)
    }`;
    return html`
      ${tabBar}
      ${!this.tabName ?
        html`<testgrid-dashboard-summary .dashboardName=${this.dashboardName}></testgrid-dashboard-summary>` :
        html`<testgrid-grid .dashboardName=${this.dashboardName} .tabName=${this.tabName}></testgrid-grid>`}
    `;
  }

  // fetch the tab names to populate the tab bar
  private async fetchTabNames() {
    try {
      const response = await fetch(
        `http://${process.env.API_HOST}:${process.env.API_PORT}/api/v1/dashboards/${this.dashboardName}/tabs`
      );
      if (!response.ok) {
        throw new Error(`HTTP error: ${response.status}`);
      }
      const data = ListDashboardTabsResponse.fromJson(await response.json());
      var tabNames: string[] = ['Summary'];
      data.dashboardTabs.forEach(tab => {
        tabNames.push(tab.name);
      });
      this.tabNames = tabNames;
      this.highlightIndex(this.tabName);
    } catch (error) {
      console.error(`Could not get dashboard summaries: ${error}`);
    }
  }

  // identify which tab to highlight on the tab bar
  private highlightIndex(tabName: string | undefined) {
    if (tabName === undefined){
      this.activeIndex = 0;
      return
    }
    var index = this.tabNames.indexOf(tabName);
    if (index > -1){
      this.activeIndex = index;
    }
  }

  static styles = css`
    mwc-tab{
      --mdc-typography-button-letter-spacing: 0;
      --mdc-tab-horizontal-padding: 12px;
      --mdc-typography-button-font-size: 0.8rem;
    }
`;
}
