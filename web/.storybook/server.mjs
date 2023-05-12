import { storybookPlugin } from '@web/dev-server-storybook';
import baseConfig from '../web-dev-server-local.config.mjs';

export default /** @type {import('@web/dev-server').DevServerConfig} */ ({
  ...baseConfig,
  open: '/',
  appIndex: null,
  plugins: [
    storybookPlugin({ type: 'web-components' }),
    ...baseConfig.plugins,
  ],
});
