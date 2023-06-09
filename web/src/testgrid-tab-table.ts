import { LitElement, html, css } from 'lit';
// eslint-disable-next-line @typescript-eslint/no-unused-vars
import { customElement, property } from 'lit/decorators.js';
import {TabSummaryInfo} from './testgrid-dashboard-summary';

@customElement('testgrid-tab-table')
export class TestgridTabTable extends LitElement {
  @property() clicked = '';
  @property() visible = false;
  info?: TabSummaryInfo;

  render() {
    return html`
    <div class="dropdown-container">
        <button @click="${(e: Event) => this.dropdownTable()}" class="btn">
          ${this.visible ? html`- Hide Alerts -`: html `- Show Alerts -`}
        </button>
      ${this.visible ? html`
          <table class="dropdown-menu">
            <tr>
              <th>Test Name</th>
              <th># Fails</th>
              <th>First Failed</th>
              <th>Last Passed</th>
            </tr>
          </table>`
          : ''}
      </div>
    `
  }
  private dropdownTable(){
    this.visible = !this.visible;
    this.dispatchEvent(new CustomEvent('visibleChange', { detail: this.visible }));
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
  `
}
