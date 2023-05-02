import { LitElement, html, css } from 'lit';
// eslint-disable-next-line @typescript-eslint/no-unused-vars
import { customElement, property } from 'lit/decorators.js';
import { map } from 'lit/directives/map.js';
import { ListDashboardResponse, ListDashboardGroupResponse } from './gen/pb/api/v1/data.js';
import { navigate } from './utils/navigation';
import '@material/mwc-button';
import '@material/mwc-list';


// dashboards template
// clicking on any dashboard should navigate to the /dashboards view
const dashboardTemplate = (dashboards: Array<string>) => html`
  <div>
    <mwc-list activatable style="min-width: 755px">
      ${map(
        dashboards,
        (dash: string, index: number) => html`
          <mwc-list-item id=${index} @click=${() => navigate(dash)} class="column card dashboard">
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
// eslint-disable-next-line @typescript-eslint/no-unused-vars
export class TestgridIndex extends LitElement {

  @property({ type: Array<string> }) 
  dashboards: Array<string> = [];

  @property({ type: Array<string> }) 
  dashboardGroups: Array<string> = [];

  @property({ type: Array<string> }) 
  respectiveDashboards: Array<string> = [];

  // toggles between the dashboards of a particular group or a dashboard without a group
  @property({ type: Boolean }) 
  show = true;

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
          ${map(this.dashboardGroups, (dash: string, index: number) => 
            html`
              <mwc-list-item
                id=${index}
                class="column card dashboard-group"
                raised
                @click="${() => this.getRespectiveDashboards(dash)}"
              >
                <div class="container">
                  <p>${dash}</p>
                </div>
              </mwc-list-item>
            `)}
        </mwc-list>

        <!-- loading dashboards -->
        ${this.show ? dashboardTemplate(this.dashboards) : ''}

        <!-- loading respective dashboards -->
        ${!this.show ? dashboardTemplate(this.respectiveDashboards) : ''}
        ${!this.show ? html`
              <mwc-button class="column" raised @click="${() => this.show = !this.show }">X</mwc-button>
            `
          : ''}
      </div>
    `;
  }

  // fetch the the list of dashboards from the API
  async fetchDashboards() {
    try{
      fetch(`http://${process.env.API_HOST}:${process.env.API_PORT}/api/v1/dashboards`).then(
        async response => {
          const resp = ListDashboardResponse.fromJson(await response.json());
          const dashboards: string[] = [];

          resp.dashboards.forEach(db => {
            dashboards.push(db.name);
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
          const resp = ListDashboardGroupResponse.fromJson(await response.json());
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

  // fetch the list of respective dashboards for a group from API
  async getRespectiveDashboards(name: string) {
    this.show = false;
    try {
      fetch(`http://${process.env.API_HOST}:${process.env.API_PORT}/api/v1/dashboard-groups/${name}`).then(
        async response => {
          const resp = ListDashboardResponse.fromJson(await response.json());
          const respectiveDashboards: string[] = [];

          resp.dashboards.forEach(ts => {
            respectiveDashboards.push(ts.name);
          });

          this.respectiveDashboards = respectiveDashboards;
        }
      );
    } catch (error) {
      console.error(`Could not get dashboard summaries: ${error}`);
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
