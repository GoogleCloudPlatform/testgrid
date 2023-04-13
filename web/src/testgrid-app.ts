import { LitElement, html } from "lit";
import { customElement } from "lit/decorators.js";
import './testgrid-router'

@customElement('testgrid-app')
export class TestgridApp extends LitElement{

    render(){
        return html`<testgrid-router></testgrid-router>`;
    }
}
