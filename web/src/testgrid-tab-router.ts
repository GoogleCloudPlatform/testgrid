import { LitElement, html } from "lit";
import { customElement, property } from "lit/decorators.js";
import { Routes } from "@lit-labs/router";
import './testgrid-group-content';

// Defines the type of params used for rendering components under different paths
interface RouteParameter {
    [key: string]: string | undefined;
}

/**
 * Class definition for the `testgrid-tab-router` element.
 * Handles the tab level routing logic.
 * Renders the grid view within dashboard-content within group-content
 */
@customElement('testgrid-tab-router')
export class TestgridTabRouter extends LitElement{

  @property({ type: String })
  groupName = '';

  @property({ type: String })
  dashboardName = '';

  private routes = new Routes(this, [
    {
      path: 'tabs/:tab', 
      render: (params: RouteParameter) => html`<testgrid-group-content .groupName=${this.groupName} .dashboardName=${this.dashboardName} .tabName=${params.tab}></testgrid-group-content>`,
    },
  ])


  /**
   * Lit-element lifecycle method.
   * Invoked on each update to perform rendering tasks.
   */
  render(){
    return html`${this.routes.outlet()}`;
  }
}
