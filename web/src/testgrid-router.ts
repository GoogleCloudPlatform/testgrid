
import { html, LitElement } from "lit";
import { customElement } from "lit/decorators.js";
import { Router } from "@lit-labs/router";
import './testgrid-index';
import './testgrid-dashboard-summary';

@customElement('testgrid-router')
export class TestgridRouter extends LitElement{


    protected firstUpdated() {
    window.addEventListener('location-changed', () => {
        this.router.goto(location.pathname);
    });
    }

    private router = new Router(
        this,
        [
            {
                path: '/dashboards',
                render: () => html`<testgrid-dashboard-summary></testgrid-dashboard-summary>`,
            },
            {
                path: '/',
                render: () => html`<testgrid-index></testgrid-index>`,
            },
        ]
    )

    render() {
        console.log(location.href)
        return html`${this.router.outlet()}`;
    }
}
