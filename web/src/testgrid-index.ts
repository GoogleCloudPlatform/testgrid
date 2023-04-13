import { LitElement, html, css } from 'lit';
// eslint-disable-next-line @typescript-eslint/no-unused-vars
import { customElement, property } from 'lit/decorators.js';
import { map } from 'lit/directives/map.js';
import {
  ListDashboardResponse,
  ListDashboardGroupResponse,
} from './gen/pb/api/v1/data.js';
import '@material/mwc-button';
import '@material/mwc-list';
import './dashboard-summary.js';
import { navigate } from './utils/navigation';

const host = 'testgrid-data.k8s.io';

// dashboards template
// clicking on any dashboard should navigate to the /dashboards view
const dashboardTemplate = (dashboards: Array<string>) => html`
  <div>
    <mwc-list activatable style="min-width: 755px">
      ${map(
        dashboards,
        (dash: string, index: number) => html`
          <mwc-list-item id=${index} @click=${() => navigate()} class="column card dashboard">
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

@customElement('testgrid-index')
// eslint-disable-next-line @typescript-eslint/no-unused-vars
export class TestgridIndex extends LitElement {
  @property({ type: Array<string> }) dashboards: Array<string> = [];

  @property({ type: Array<string> }) dashboardGroups: Array<string> = [];

  @property({ type: Array<string> }) respectiveDashboards: Array<string> = [];

  @property({ type: Boolean }) show = true;

  // TODO(chases2): inject an APIClient object so we can inject it into tests/storybook later

  render() {
    return html`
      <mwc-button raised @click="${this.callAPI}">Call API</mwc-button>

      <div class="flex-container">
        <!-- loading dashboard groups -->
        <mwc-list style="min-width: 760px">
          ${map(
            this.dashboardGroups,
            (dash: string, index: number) => html`
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
            `
          )}
        </mwc-list>

        <!-- loading dashboards -->
        ${this.show ? dashboardTemplate(this.dashboards) : ''}

        <!-- loading respective dashboards -->
        ${!this.show ? dashboardTemplate(this.respectiveDashboards) : ''}
        ${!this.show
          ? html`
              <mwc-button
                class="column"
                raised
                @click="${() => {
                  this.show = !this.show;
                }}"
                >X</mwc-button
              >
            `
          : ''}
      </div>
    `;
  }

  // function to get dashboards
  async getDashboards() {
    this.dashboards = ['Loading...'];

    fetch(`http://${host}/api/v1/dashboards`).then(async response => {
      const resp = ListDashboardResponse.fromJson(await response.json());

      this.dashboards = [];

      resp.dashboards.forEach(db => {
        this.dashboards.push(db.name);
      });
    });
  }

  // function to get dashboard groups
  async getDashboardGroups() {
    this.dashboardGroups = ['Loading...'];

    fetch(`http://${host}/api/v1/dashboard-groups`).then(async response => {
      const resp = ListDashboardGroupResponse.fromJson(await response.json());

      this.dashboardGroups = [];

      resp.dashboardGroups.forEach(db => {
        this.dashboardGroups.push(db.name);
      });
    });
  }

  // function to get respective dashboards of dashboard group
  async getRespectiveDashboards(name: string) {
    this.show = false;
    // this.respectiveDashboards = ['Loading...'];
    try {
      fetch(`http://${host}/api/v1/dashboard-groups/${name}`).then(
        async response => {
          const resp = ListDashboardResponse.fromJson(await response.json());

          this.respectiveDashboards = [];

          resp.dashboards.forEach(ts => {
            this.respectiveDashboards.push(ts.name);
          });
          console.log(this.respectiveDashboards);
        }
      );
    } catch (error) {
      console.error(`Could not get dashboard summaries: ${error}`);
    }
  }

  // call both of these at same time
  callAPI() {
    this.getDashboardGroups();
    this.getDashboards();
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
