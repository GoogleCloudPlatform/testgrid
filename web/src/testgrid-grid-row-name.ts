import { LitElement, html, css } from "lit";
import { customElement, property } from "lit/decorators.js";

@customElement('testgrid-grid-row-name')
export class TestgridGridRowName extends LitElement{
    static styles = css`
    :host {
      text-align: left;
      font-family: Roboto, Verdana, sans-serif;
      display: inline-block;
      background-color: #ccd;
      color: #224;
      min-height: 1.2em;
      max-width: 300px;
      width:200px;
      padding: .1em .3em;
      box-sizing: border-box;
      min-height: 22px;
      max-height: 22px;
      white-space:nowrap;
      min-width: 300px;
      overflow-x: hidden;
      text-overflow: ellipsis;
    }
  `;

    @property() name: String;

    render(){
        return html`${this.name}`;
    }
}
