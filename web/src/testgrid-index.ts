import { LitElement, html, css } from 'lit';
import { customElement, property } from 'lit/decorators.js';
import { map } from 'lit/directives/map.js';
import { ListDashboardsResponse, ListDashboardGroupsResponse } from './gen/pb/api/v1/data.js';
import { navigateGroup } from './utils/navigation';
import '@material/mwc-list';


// dashboards template
// clicking on any dashboard should navigate to the /dashboards view
const dashboardTemplate = (dashboards: Array<string>) => html`
  <div>
    <mwc-list activatable style="min-width: 755px">
      ${map(
        dashboards,
        (dash: string, index: number) => html`
          <mwc-list-item id=${index} class="column card dashboard">
              <div class="container">
                <p>${dash}</p>
              </div>
            </a>
          </mwc-list-item>
        `
      )}
    </mwc-list>
  </div>
`;

/**
 * Class definition for the `testgrid-index` element.
 * Renders the list of dashboards and dashboard groups available.
 */
@customElement('testgrid-index')
export class TestgridIndex extends LitElement {

  @property({ type: Array<string> }) 
  dashboards: Array<string> = [];

  @property({ type: Array<string> }) 
  dashboardGroups: Array<string> = [];

  /**
   * Lit-element lifecycle method.
   * Invoked when a component is added to the document's DOM.
   */
  connectedCallback() {
    super.connectedCallback();
    this.fetchDashboardGroups();
    this.fetchDashboards();
  }

  /**
   * Lit-element lifecycle method.
   * Invoked on each update to perform rendering tasks.
   */
  render() {
    return html`
      <div class="flex-container">
        <!-- loading dashboard groups -->
        <mwc-list style="min-width: 760px">
          ${map(this.dashboardGroups, (group: string, index: number) => 
            html`
              <mwc-list-item
                id=${index}
                class="column card dashboard-group"
                raised
                @click="${() => navigateGroup(group, true)}"
              >
                <div class="container">
                  <p>${group}</p>
                </div>
              </mwc-list-item>
            `)}
        </mwc-list>

        <!-- loading dashboards -->
        ${dashboardTemplate(this.dashboards)}

      </div>
    `;
  }

  // fetch the the list of dashboards from the API
  async fetchDashboards() {
    try{
      fetch(`http://${process.env.API_HOST}:${process.env.API_PORT}/api/v1/dashboards`).then(
        async response => {
          const resp = ListDashboardsResponse.fromJson(await response.json());
          const dashboards: string[] = [];

          resp.dashboards.forEach(db => {
            if (db.dashboardGroupName === ""){
              dashboards.push(db.name);
            }
          });
          this.dashboards = dashboards;
        }
      );
    } catch (error) {
      console.log(`failed to fetch: ${error}`);
    }
  }

  // fetch the list of dashboard groups from API
  async fetchDashboardGroups() {
    try{
      fetch(`http://${process.env.API_HOST}:${process.env.API_PORT}/api/v1/dashboard-groups`).then(
        async response => {
          const resp = ListDashboardGroupsResponse.fromJson(await response.json());
          const dashboardGroups: string[] = [];

          resp.dashboardGroups.forEach(db => {
            dashboardGroups.push(db.name);
          });

          this.dashboardGroups = dashboardGroups;
        }
      );
    } catch(error){
      console.log(`failed to fetch: ${error}`);
    }
  }

  static styles = css`
    :host {
      overflow: auto;
      min-height: 100vh;
      display: flex;
      flex-direction: column;
      justify-content: flex;
      font-size: calc(10px + 2vmin);
      color: #1a2b42;
      margin: 0 auto;
      text-align: center;
      background-color: var(--example-app-background-color);
    }

    .flex-container {
      display: grid;
      gap: 30px;
      grid-template-columns: auto auto auto;
    }

    .column {
      display: inline-grid;
      padding: 10px;
    }

    .card {
      /* Add shadows to create the "card" effect */
      width: 350px;
      height: 80px;
      margin-bottom: 10px;
      box-shadow: 0 30px 30px -25px rgba(#7168c9, 0.25);
    }

    .dashboard {
      background-color: #9e60eb;
      color: #fff;
    }

    .dashboard-group {
      background-color: #707df1;
      color: #fff;
    }

    .dashboard-group:focus,
    .dashboard-group:hover {
      background-color: #fff;
      color: #707df1;
      border-style: solid;
      border-color: #707df1;
    }

    .dashboard:hover,
    .dashboard:focus {
      background-color: #fff;
      color: #9e60eb;
      border-style: solid;
      border-color: #9e60eb;
    }

    /* Add some padding inside the card container */
    .container {
      padding: 2px 16px;
    }
  `;
}
