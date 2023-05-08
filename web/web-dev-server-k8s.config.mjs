import { fromRollup } from '@web/dev-server-rollup';
import rollupReplace from '@rollup/plugin-replace';

const replace = fromRollup(rollupReplace);

export default /** @type {import('@web/dev-server').DevServerConfig} */ ({
  open: '/',
  /** Resolve bare module imports */
  nodeResolve: {
    exportConditions: ['browser', 'development'],
  },

  /** Set appIndex to enable SPA routing */
  appIndex: 'index.html',

  plugins: [
    replace({
      'process.env.API_HOST': '"testgrid-data.k8s.io"',
      'process.env.API_PORT': '80',
    }),
  ],

  // See documentation for all available options
});
