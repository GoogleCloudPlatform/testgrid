import { LitElement, html, css } from 'lit';
// eslint-disable-next-line @typescript-eslint/no-unused-vars
import { customElement, property, state } from 'lit/decorators.js';
import { map } from 'lit/directives/map.js';
import { when } from 'lit/directives/when.js';
import { navigateTab } from './utils/navigation.js';
import { ListDashboardTabsResponse } from './gen/pb/api/v1/data.js';
import '@material/mwc-tab';
import '@material/mwc-tab-bar';
import './testgrid-dashboard-summary';
import './testgrid-grid-display';

/**
 * Class definition for the `testgrid-data-content` element.
 * Acts as a container for dashboard summary or grid data.
 */
@customElement('testgrid-data-content')
// eslint-disable-next-line @typescript-eslint/no-unused-vars
export class TestgridDataContent extends LitElement {

  @state()
  tabNames: string[] = [];

  @state()
  activeIndex = 0;

  @property({ type: Boolean })
  showTab = false;

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

    this.tabName = this.tabNames[tabIndex];
    console.log(this.tabName);

    if (this.activeIndex === 0 || tabIndex === 0){
      this.showTab = !this.showTab;
    }
    this.activeIndex = tabIndex;
    navigateTab(this.dashboardName, this.tabName)
  }

  /**
   * Lit-element lifecycle method.
   * Invoked when a component is added to the document's DOM.
   */
  connectedCallback() {
    super.connectedCallback();
    this.fetchTabNames();
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
      ${!this.showTab ? 
        html`<testgrid-dashboard-summary .dashboardName=${this.dashboardName}></testgrid-dashboard-summary>` : 
        html`<testgrid-grid-display .dashboardName=${this.dashboardName} .tabName=${this.tabName}></testgrid-grid-display>`}
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
