import { LitElement, html, PropertyValues } from "lit";
import { map } from "lit/directives/map.js";
import { customElement, property, state } from "lit/decorators.js";
import { ListRowsResponse, ListRowsResponse_Row } from './gen/pb/api/v1/data.js';

/**
 * Class definition for `testgrid-grid-display` component.
 * Renders the test results grid.
 */
@customElement('testgrid-grid-display')
export class TestgridGridDisplay extends LitElement{

  @property({ type: String })
  dashboardName: String = '';

  @property({ type: String })
  tabName: String = '';

  @state()
  tabGridRows: Array<ListRowsResponse_Row> = [];

  /**
   * Lit-element lifecycle method.
   * Invoked when element properties are changed.
   */
  willUpdate(changedProperties: PropertyValues<this>) {
    if (changedProperties.has('tabName')){
      this.fetchTabGridRows();
    }
  }

  /**
   * Lit-element lifecycle method.
   * Invoked on each update to perform rendering tasks.
   */
  render(){
    return html`
      ${map(this.tabGridRows,
          (row: ListRowsResponse_Row) => html`<p>${row.name}</p>`
        )}
    `;
  }

  // fetch the tab data
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
}