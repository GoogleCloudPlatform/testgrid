import { LitElement, html, css } from "lit";
import { customElement, property } from "lit/decorators.js";

@customElement('testgrid-grid-column-header')
export class TestgridGridColumnHeader extends LitElement{
    // TODO(michelle192837): Collate column headers with the same value.
    static styles = css`
    :host {
      text-align: center;
      font-family: Roboto, Verdana, sans-serif;
      display: inline-block;
      background-color: #ccd;
      color: #224;
      min-height: 22px;
      max-height: 22px;
      max-width: 80px;
      width:80px;
      min-width: 80px;
      padding: .1em .3em;
      box-sizing: border-box;
      white-space:nowrap;
      overflow-x: hidden;
      text-overflow: ellipsis;
    }
  `;

    @property() name: String;

    render(){
        return html`${this.name}`;
    }
}
