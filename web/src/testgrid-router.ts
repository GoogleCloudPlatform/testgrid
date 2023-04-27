import { LitElement, html } from "lit";
import { customElement } from "lit/decorators.js";
import { Router } from "@lit-labs/router";
import './testgrid-data-content';
import './testgrid-index';

// Defines the type of params used for rendering components under different paths
interface RouteParameter {
    [key: string]: string | undefined;
}

/**
 * Class definition for the `testgrid-router` element.
 * Handles the routing logic.
 */
@customElement('testgrid-router')
export class TestgridRouter extends LitElement{
  private router = new Router(this, [
    {
      path: '/:dashboard/*', 
      render: (params: RouteParameter) => html`<testgrid-data-content .dashboardName=${params.dashboard} .tabName=${params[0]} showTab></testgrid-data-content>`,
    },
    {
      path: '/:dashboard', 
      render: (params: RouteParameter) => html`<testgrid-data-content .dashboardName=${params.dashboard}></testgrid-data-content>`,
    },
    {
      path: '/',
      render: () => html`<testgrid-index></testgrid-index>`,
    },
  ])

  /**
   * Lit-element lifecycle method.
   * Invoked when a component is added to the document's DOM.
   */
  connectedCallback(){
    super.connectedCallback();
    window.addEventListener('location-changed', () => {
      this.router.goto(location.pathname);
    });
  }

  /**
   * Lit-element lifecycle method.
   * Invoked on each update to perform rendering tasks.
   */
  render(){
    return html`${this.router.outlet()}`;
  }
}
