import { LitElement, html, css } from 'lit';
// eslint-disable-next-line @typescript-eslint/no-unused-vars
import { customElement, property, state } from 'lit/decorators.js';
import { TabSummaryInfo } from './testgrid-dashboard-summary';
import { map } from 'lit/directives/map.js';

@customElement('testgrid-healthiness-summary')
export class TestgridTabTable extends LitElement {
  @state() showHealthinesSummary = false;
  @property() info?: TabSummaryInfo;

  render() {
    return html`
    <div class="dropdown-container">
        <button @click="${() => this.dropdownTable()}" class="btn">
          ${this.showHealthinesSummary ? html`- Hide Healthiness Report -`: html `- Show Healthiness Report -`}
        </button>
      ${this.showHealthinesSummary ? html`
          <table class="dropdown-menu">
            <tr>
              <th>Test Name</th>
              <th>Flakiness (Previous)</th>
              <th>Flakiness (Current)</th>
              <th>Trend</th>
              <th>Infra Failure Rate</th>
              <th>Flaky Tests Count</th>
            </tr>
            ${map(
              this.info?.healthinessSummary!.topFlakyTests,
              (test: any) => html`
                <tr>
                  <td>${test.displayName}</td>
                  <td>${this.info?.healthinessSummary?.healthinessStats.previousFlakiness}</td>
                  <td>${this.info?.healthinessSummary?.healthinessStats.averageFlakiness}</td>
                  <td>N/A</td>
                  <td>${test.flakiness}</td>
                  <td>${this.info?.healthinessSummary?.healthinessStats.numFlakyTests}</td>
                </tr>
              `)}
          </table>`
          : ''}
      </div>
    `
  }
  private dropdownTable(){
    this.showHealthinesSummary = !this.showHealthinesSummary;
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
