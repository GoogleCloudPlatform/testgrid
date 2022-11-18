import { LitElement, html, css } from 'lit';
// eslint-disable-next-line @typescript-eslint/no-unused-vars
import { customElement, property } from 'lit/decorators.js';
import { map } from 'lit/directives/map.js';
import { ListDashboardResponse } from './pb/api/v1/data.js';
import '@material/mwc-button';
import '@material/mwc-list';

@customElement('example-app')
// eslint-disable-next-line @typescript-eslint/no-unused-vars
export class ExampleApp extends LitElement {
  @property({ type: String }) title: string = 'My app';

  @property({ type: Array<string> }) dashboards: Array<string> = [];

  // TODO(chases2): inject an APIClient object so we can figure out how to inject it into tests later

  connectedCallback() {
    super.connectedCallback();
    // add other things here
  }

  render() {
    return html`
      <mwc-list>
        ${map(this.dashboards, (dash: string, index: number) => {
          if (index !== 0) {
            return html`
              <li divider role="separator"></li>
              <mwc-list-item>${dash}</mwc-list-item>
            `;
          }
          return html`<mwc-list-item>${dash}</mwc-list-item>`;
        })}
      </mwc-list>
      <mwc-button raised @click="${this.getDashboards}">Call API</mwc-button>
    `;
  }

  getDashboards() {
    this.dashboards = ['Loading...'];

    fetch('http://localhost:8080/api/v1/dashboards').then(async response => {
      const resp = ListDashboardResponse.fromJson(await response.json());

      this.dashboards = [];

      resp.dashboards.forEach(db => {
        this.dashboards.push(db.name);
      });
    });
  }

  static styles = css`
    :host {
      min-height: 100vh;
      display: flex;
      flex-direction: column;
      justify-content: flex-start;
      font-size: calc(10px + 2vmin);
      color: #1a2b42;
      max-width: 960px;
      margin: 0 auto;
      text-align: center;
      background-color: var(--example-app-background-color);
    }
  `;
}
