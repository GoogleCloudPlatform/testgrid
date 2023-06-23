import { LitElement, html } from "lit";
import { customElement, property } from "lit/decorators.js";
import { Routes } from "@lit-labs/router";
import '../group/testgrid-group-content';
import './testgrid-tab-router';

// Defines the type of params used for rendering components under different paths
interface RouteParameter {
    [key: string]: string | undefined;
}

/**
 * Class definition for the `testgrid-dashboard-router` element.
 * Handles the dashboard level routing logic.
 * Renders the dashboard summary view within group-content element or continues
 * with the next level router.
 */
@customElement('testgrid-dashboard-router')
export class TestgridDashboardRouter extends LitElement{

  // passed down from root router to perform correct navigation
  @property({ type: String })
  groupName = '';

  private routes = new Routes(this, [
    {
      path: 'dashboards/:dashboard', 
      render: (params: RouteParameter) => html`<testgrid-group-content .groupName=${this.groupName} .dashboardName=${params.dashboard}></testgrid-group-content>`,
    },
    {
      path: 'dashboards/:dashboard/*', 
      render: (params: RouteParameter) => html`<testgrid-tab-router .groupName=${this.groupName} .dashboardName=${params.dashboard}></testgrid-tab-router>`,
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
