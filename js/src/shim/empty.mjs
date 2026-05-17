// Generic empty-module stub. Any named import resolves to a no-op function
// or no-op object via Proxy. Used to cut MDX / DOM / unused modules out of
// the bundle without breaking import statements.
const noop = function () {};
const handler = {
  get(_target, prop) {
    if (prop === "__esModule") return true;
    if (prop === Symbol.toPrimitive) return () => "";
    return noop;
  },
};
const stub = new Proxy(noop, handler);
export default stub;
export const __isEmptyShim = true;
// Anything imported by name falls through to the Proxy via re-export trick:
// esbuild handles "import { X } from 'pkg'" by reading the property X of
// the module namespace. That cannot be proxied with ESM, so individual
// callers will see `undefined` for unknown names. Most code under stub
// here is import-only (not called). If a stubbed module ends up being
// invoked it'll throw — that's the signal to un-stub it.
