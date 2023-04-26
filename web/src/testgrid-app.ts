import { LitElement, html } from "lit";
import { customElement } from "lit/decorators.js";
import './testgrid-router'

/**
 * Class definition for the `testgrid-app` element.
 * Application root element.
 */
@customElement('testgrid-app')
export class TestgridApp extends LitElement{
  /**
   * Lit-element lifecycle method.
   * Invoked on each update to perform rendering tasks.
   */
  render(){
    return html`<testgrid-router></testgrid-router>`;
  }
}
