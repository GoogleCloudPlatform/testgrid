import { LitElement, html, css } from 'lit';
// eslint-disable-next-line @typescript-eslint/no-unused-vars
import { customElement, property } from 'lit/decorators.js';
import { TabSummaryInfo } from './testgrid-dashboard-summary';
@customElement('tab-summary')
// eslint-disable-next-line @typescript-eslint/no-unused-vars
export class TabSummary extends LitElement {
  @property({ type: Object })
  info?: TabSummaryInfo;
  render() {
    return html`
      <link
        rel="stylesheet"
        href="https://fonts.googleapis.com/icon?family=Material+Icons"
      />
      <div class="tab">
        <div class="left">
          <div class="icon-wrapper">
            <i class="material-icons ${this.info?.overallStatus}"
              >${this.info?.icon}</i
            >
          </div>
        </div>
        <div class="mid">
          <div @click=${() => this.changeTab()} class="tab-name">
            ${this.info?.name}: ${this.info?.overallStatus}
          </div>
          <div class="detailed-status">${this.info?.detailedStatusMsg}</div>
        </div>
        <div class="right">
          <div class="stats">
            Last update: ${this.info?.lastUpdateTimestamp}
          </div>
          <div class="stats">
            Tests last ran: ${this.info?.lastRunTimestamp}
          </div>
          <div class="stats">
            Last green run: ${this.info?.latestGreenBuild}
          </div>
        </div>
      </div>
    `;
  }
  /**
   * Lets the data content element know that the tab changed
   * 
   * @fires tab-changed
   * @param tabName string
   */
  private changeTab(){
    window.dispatchEvent(new CustomEvent('tab-changed',{
      detail: {
        tabName: this.info?.name!
      },
    }))
  }

  static styles = css`

    .tab-name { // title/link in each Summary card
      cursor: pointer;
      position: relative;
      padding: 4px 8px;
      color: #00c;
      text-decoration: underline;
    }

    .tab {
      border: 1px solid #6b90da;
      border-radius: 6px;
      color: #000;
      display: grid;
      grid-template-columns: 1fr 17fr 6fr;
      margin: 5px;
      overflow: hidden;
      align-items: center;
    }

    .tab-name { // title/link in each Summary card
      cursor: pointer;
      position: relative;
      padding: 4px 8px;
      color: #00c;
      text-decoration: underline;
    }

    .stats {
      text-align: right;
    }

    .left {
      justify-content: center;
      text-align: center;
    }

    .material-icons {
      font-size: 2em;
      color: #fff;
    }

    .PENDING {
      background-color: #cc8200;
    }

    .PASSING {
      background-color: #0c3;
    }

    .FAILING {
      background-color: #a00;
    }

    .FLAKY {
      background-color: #609;
    }

    .ACCEPTABLE {
      background-color: #39a2ae;
    }

    .STALE {
      background-color: #808b96;
    }

    .BROKEN {
      background-color: #000;
    }
  `;
}
