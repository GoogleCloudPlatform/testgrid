import {
  html,
  fixture,
  defineCE,
  unsafeStatic,
  expect,
} from '@open-wc/testing';

import { ExampleApp } from '../src/ExampleApp.js';

describe('ExampleApp', () => {
  let element: ExampleApp;
  beforeEach(async () => {
    // Need to wrap an element to apply its properties (ex. @customElement)
    // See https://open-wc.org/docs/testing/helpers/#test-a-custom-class-with-properties
    const tagName = defineCE(class extends ExampleApp {});
    const tag = unsafeStatic(tagName);
    element = await fixture(html`<${tag}></${tag}>`);
  });

  it('renders a button', async () => {
    const h1 = element.shadowRoot!.querySelector('mwc-button')!;
    expect(h1).to.exist;
  });

  it('passes the a11y audit', async () => {
    await expect(element).shadowDom.to.be.accessible();
  });
});
