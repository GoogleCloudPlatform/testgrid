import { LitElement, html, css } from "lit";
import { map } from "lit/directives/map.js";
import { customElement, property } from "lit/decorators.js";
import { ListRowsResponse_Row } from './gen/pb/api/v1/data.js';
import { TestStatus } from './gen/pb/test_status/test_status';

interface Cell {
  status: String;
  icon: String;
}

@customElement('testgrid-grid-row')
export class TestgridGridRow extends LitElement {
  static styles = css`
    :host {
      text-align: center;
      font-family: Roboto, Verdana, sans-serif;
      display: flex;
      flex-flow: row nowrap;
      gap: 0px 2px;
      margin: 2px;
    }
  `;

  @property() name: String;
  @property() rowData: ListRowsResponse_Row;

  render() {
    if (this.rowData && this.rowData.cells) {
      return html`<testgrid-grid-row-header .name="${this.name}">
        </testgrid-grid-row-header>
        ${map(this.rowData.cells,
        (cell) => html`<testgrid-grid-cell .icon="${cell.icon}" .status="${TestStatus[cell.result]}"></testgrid-grid-cell>`
      )}
        `;
    }
    return html`
    <testgrid-grid-row-header .name="${this.name}"></testgrid-grid-row-header>
    `;
  }
}
