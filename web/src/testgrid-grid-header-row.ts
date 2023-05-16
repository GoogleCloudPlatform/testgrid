import { LitElement, html, css } from 'lit';
import { map } from 'lit/directives/map.js';
import { customElement, property } from 'lit/decorators.js';
import { ListHeadersResponse } from './gen/pb/api/v1/data.js';
import './testgrid-grid-row-name';
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
    // TODO(michelle192837): Replace row name component with more appropriate one.
    return html`
      <testgrid-grid-row-name></testgrid-grid-row-name>
      ${map(
        this.headers?.headers,
        header =>
          html`<testgrid-grid-column-header
            .name="${header.build}"
          ></testgrid-grid-column-header>`
      )}
    `;
  }
}
