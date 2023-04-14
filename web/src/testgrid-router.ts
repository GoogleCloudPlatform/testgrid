import { LitElement, html } from "lit";
import { customElement } from "lit/decorators.js";
import {Router} from "@lit-labs/router";
import './dashboard-summary';
import './testgrid-index';

@customElement('testgrid-router')
export class TestgridRouter extends LitElement{
    private router = new Router(this, [
        {
            path: '/dashboards', 
            render: () => html`<dashboard-summary></dashboard-summary>`,
        },
        {
            path: '/',
            render: () => html`<testgrid-index></testgrid-index>`,
        },
    ])

    connectedCallback() {
        super.connectedCallback();
        window.addEventListener('location-changed', () => {
            this.router.goto(location.pathname);
        });
    }

    render(){
        return html`${this.router.outlet()}`;
    }
}
