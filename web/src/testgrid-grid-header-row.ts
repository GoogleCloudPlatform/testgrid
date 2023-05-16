import { LitElement, html, css } from "lit";
import { map } from "lit/directives/map.js";
import { customElement, property } from "lit/decorators.js";
import { ListHeadersResponse } from './gen/pb/api/v1/data.js';
import './testgrid-grid-row-id';
import './testgrid-grid-column-header';

@customElement('testgrid-grid-header-row')
export class TestgridGridHeaderRow extends LitElement {
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

  @property() headers: ListHeadersResponse;

  render() {
    if (this.headers && this.headers.headers) {
      return html`
        <testgrid-grid-row-id></testgrid-grid-row-id>
        ${map(this.headers.headers,
        (header) => html`<testgrid-grid-column-header .name="${header.build}"></testgrid-grid-column-header>`
      )}
        `;
    }
    return html`
    <testgrid-grid-row-id></testgrid-grid-row-id>
    `;
  }
}
