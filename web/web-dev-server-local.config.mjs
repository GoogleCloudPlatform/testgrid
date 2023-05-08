import { fromRollup } from '@web/dev-server-rollup';
import rollupReplace from '@rollup/plugin-replace';

const replace = fromRollup(rollupReplace);

export default ({
  open: '/',
  /** Resolve bare module imports */
  nodeResolve: {
    exportConditions: ['browser', 'development'],
  },

  /** Set appIndex to enable SPA routing */
  appIndex: 'index.html',

  plugins: [
    replace({
      'process.env.API_HOST': '"localhost"',
      'process.env.API_PORT': '3000',
    }),
  ],
});
