import { LitElement, html } from "lit";
import { customElement } from "lit/decorators.js";
import { Router } from "@lit-labs/router";
import './testgrid-dashboard-router';
import './testgrid-group-content';
import './testgrid-index';

// Defines the type of params used for rendering components under different paths
interface RouteParameter {
    [key: string]: string | undefined;
}

/**
 * Class definition for the `testgrid-root-router` element.
 * Handles the top-level routing logic.
 * Renders the index page, main container component or passes down the data to dashboard level router.
 */
@customElement('testgrid-root-router')
export class TestgridRootRouter extends LitElement{
  private router = new Router(this, [
    {
      path: '/groups/:group/*', 
      render: (params: RouteParameter) => html`<testgrid-dashboard-router .groupName=${params.group}></testgrid-dashboard-router>`,
    },
    {
      path: '/groups/:group', 
      render: (params: RouteParameter) => html`<testgrid-group-content .groupName=${params.group}></testgrid-group-content>`,
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
