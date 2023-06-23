import { LitElement, html, css } from "lit";
import { customElement, property } from "lit/decorators.js";

@customElement('testgrid-grid-cell')
export class TestgridGridCell extends LitElement{
    // Styling for status attribute corresponds to test_status.proto enum.
    static styles = css`
    :host {
      min-width: 80px;
      width: 80px;
      min-height: 22px;
      max-height: 22px;
      color: #000;
      background-color: #ccc;
      text-align: center;
      font-family: Roboto, Verdana, sans-serif;
      font-weight: bold;
      display: flex;
      justify-content: center;
      align-content: center;
      flex-direction: column;
      box-sizing: border-box;
      font-size: 12px;
    }

    :host([status="NO_RESULT"]) {
        background-color: transparent;
    }

    :host([status="PASS"]), :host([status="PASS_WITH_ERRORS"]) {
        background-color: #4d7;
        color: #273;
    }

    :host([status="PASS_WITH_SKIPS"]), :host([status="BUILD_PASSED"]) {
        background-color: #bfd;
        color: #465;
    }

    :host([status="RUNNING"]), :host([status="CATEGORIZED_ABORT"]), :host([status="UNKNOWN"]), :host([status="CANCEL"]), :host([status="BLOCKED"]) {
        background-color: #ccd;
        color: #446;
    }

    :host([status="TIMED_OUT"]), :host([status="CATEGORIZED_FAIL"]), :host([status="FAIL"]), :host([status="FLAKY"]) {
        background-color: #a24;
        color: #fdd;
    }

    :host([status="BUILD_FAIL"]) {
        background-color: #111;
        color: #ddd;
    }

    :host([status="FLAKY"]) {
        background-color: #63a;
        color: #dcf;
    }
  `;

    @property({reflect: true, attribute: 'status'}) status: String;
    @property() icon: String;

    render(){
        return html`${this.icon}`;
    }
}
