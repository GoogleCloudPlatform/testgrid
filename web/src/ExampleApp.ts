import { LitElement, html, css } from 'lit';
// eslint-disable-next-line @typescript-eslint/no-unused-vars
import { customElement, property } from 'lit/decorators.js';
import { map } from 'lit/directives/map.js';
import { ListDashboardResponse } from './pb/api/v1/data.js';

@customElement('example-app')
// eslint-disable-next-line @typescript-eslint/no-unused-vars
export class ExampleApp extends LitElement {
  @property({ type: String }) title = 'My app';

  @property({ type: Array<String> }) dashboards = ['foo'];

  render() {
    return html`
      <ol>
        ${map(this.dashboards, dash => html`<li>${dash}</li>`)}
      </ol>
    `;
  }

  // TODO(slchase) add a button to call or something
  getDashboards() {
    const result: Array<string> = [];
    for (let i = 0; i < 100; i += 1) {
      result.push('Foo');
    }
    this.dashboards = result;

    fetch('localhost:8080/api/v1/dashboards').then(async response => {
      const json = await response.json();
      console.log(json);
      const resp = ListDashboardResponse.fromJson(json);
      console.log(resp);

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
      align-items: center;
      justify-content: flex-start;
      font-size: calc(10px + 2vmin);
      color: #1a2b42;
      max-width: 960px;
      margin: 0 auto;
      text-align: center;
      background-color: var(--example-app-background-color);
    }

    main {
      flex-grow: 1;
    }

    .logo {
      margin-top: 36px;
      animation: app-logo-spin infinite 20s linear;
    }

    @keyframes app-logo-spin {
      from {
        transform: rotate(0deg);
      }
      to {
        transform: rotate(360deg);
      }
    }

    .app-footer {
      font-size: calc(12px + 0.5vmin);
      align-items: center;
    }

    .app-footer a {
      margin-left: 5px;
    }
  `;
}
