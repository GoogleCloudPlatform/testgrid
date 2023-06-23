import { LitElement, html, PropertyValues } from "lit";
import { map } from "lit/directives/map.js";
import { customElement, property, state } from "lit/decorators.js";
import { ListHeadersResponse, ListRowsResponse, ListRowsResponse_Row } from '../gen/pb/api/v1/data.js';
import './testgrid-grid-row';
import './testgrid-grid-header-row';

/**
 * Class definition for `testgrid-grid` component.
 * Renders the test results grid.
 */
@customElement('testgrid-grid')
export class TestgridGrid extends LitElement {

  @property({ type: String, reflect: true })
  dashboardName: String = '';

  @property({ type: String, reflect: true })
  tabName: String = '';

  @state()
  tabGridRows: Array<ListRowsResponse_Row> = [];

  @state()
  tabGridHeaders: ListHeadersResponse;

  /**
   * Lit-element lifecycle method.
   * Invoked when element properties are changed.
   */
  willUpdate(changedProperties: PropertyValues<this>) {
    if (changedProperties.has('tabName')) {
      this.fetchTabGrid();
    }
  }

  /**
   * Lit-element lifecycle method.
   * Invoked on each update to perform rendering tasks.
   */
  render() {
    return html`
    <testgrid-grid-header-row .headers="${this.tabGridHeaders}"></testgrid-grid-header-row>
    ${map(this.tabGridRows,
      (row: ListRowsResponse_Row) => html`<testgrid-grid-row .name="${row.name}" .rowData="${row}"></testgrid-grid-row>`
    )}
    `;
  }

  private async fetchTabGrid() {
    this.fetchTabGridRows();
    this.fetchTabGridHeaders();
  }

  private async fetchTabGridRows() {
    this.tabGridRows = [];
    try {
      const response = await fetch(
        `http://${process.env.API_HOST}:${process.env.API_PORT}/api/v1/dashboards/${this.dashboardName}/tabs/${this.tabName}/rows`
      );
      if (!response.ok) {
        throw new Error(`HTTP error: ${response.status}`);
      }
      const data = ListRowsResponse.fromJson(await response.json());
      var rows: Array<ListRowsResponse_Row> = [];
      data.rows.forEach(row => rows.push(row));
      this.tabGridRows = rows;
    } catch (error) {
      console.error(`Could not get grid rows: ${error}`);
    }
  }

  private async fetchTabGridHeaders() {
    try {
      const response = await fetch(
        `http://${process.env.API_HOST}:${process.env.API_PORT}/api/v1/dashboards/${this.dashboardName}/tabs/${this.tabName}/headers`
      );
      if (!response.ok) {
        throw new Error(`HTTP error: ${response.status}`);
      }
      const data = ListHeadersResponse.fromJson(await response.json());
      this.tabGridHeaders = data;
    } catch (error) {
      console.error(`Could not get grid headers: ${error}`);
    }
  }
}
