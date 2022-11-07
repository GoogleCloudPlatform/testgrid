import { html } from 'lit';
import { fixture, expect } from '@open-wc/testing';

import { ExampleApp } from '../src/ExampleApp.js';
import '../src/example-app.js';

describe('ExampleApp', () => {
  let element: ExampleApp;
  beforeEach(async () => {
    element = await fixture(html`<example-app></example-app>`);
  });

  it('renders a h1', () => {
    const h1 = element.shadowRoot!.querySelector('h1')!;
    expect(h1).to.exist;
    expect(h1.textContent).to.equal('My app');
  });

  it('passes the a11y audit', async () => {
    await expect(element).shadowDom.to.be.accessible();
  });
});
