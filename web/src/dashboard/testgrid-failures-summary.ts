import { LitElement, html, css } from 'lit';
// eslint-disable-next-line @typescript-eslint/no-unused-vars
import { customElement, property, state } from 'lit/decorators.js';
import { TabSummaryInfo } from './testgrid-dashboard-summary';
import { map } from 'lit/directives/map.js';

@customElement('testgrid-failures-summary')
export class TestgridFailuresSummary extends LitElement {
  @state() showFailureSummary = false;
  @property() info?: TabSummaryInfo;

  render() {
    return html`
    <div class="dropdown-container">
        <button @click="${() => this.dropdownTable()}" class="btn">
          ${this.showFailureSummary ? html`- Hide Alerts -`: html `- Show Alerts -`}
        </button>
      ${this.showFailureSummary ? html`
          <table class="dropdown-menu">
            <tr>
              <th>Test Name</th>
              <th># Fails</th>
              <th>First Failed</th>
              <th>Last Passed</th>
            </tr>
            ${map(
              this.info?.failuresSummary!.topFailingTests,
              (test: any) => html`
                <tr>
                  <td>${test.displayName}</td>
                  <td>${test.failCount}</td>
                  <td>${test.passTimestamp}</td>
                  <td>${test.failTimestamp}</td>
                </tr>
              `)}
          </table>`
          : ''}
      </div>
    `
  }
  private dropdownTable(){
    this.showFailureSummary = !this.showFailureSummary;
  }

  static styles = css`
    .dropdown-container {
      border-left: 1px solid #6b90da;
      border-right: 1px solid #6b90da;
      border-bottom: 1px solid #6b90da;
      border-radius: 0 0 6px 6px;
      color: #000;
      display: block;
      position: relative;
    }

    .dropdown-menu {
      position: relative;
      width: 100%;
    }

    .btn {
      display: grid;
      border-radius: var(--radius);
      border: none;
      cursor: pointer;
      position: relative;
      width: 100%;
    }

    th {
      text-align: left;
    }
  `
}
