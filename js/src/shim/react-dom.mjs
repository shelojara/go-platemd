// Stub for react-dom. None of these are invoked during markdown conversion.
const noop = () => {};
export const render = noop;
export const hydrate = noop;
export const createPortal = (children) => children;
export const flushSync = (fn) => fn();
export const unstable_batchedUpdates = (fn) => fn();
export const createRoot = () => ({ render: noop, unmount: noop });
export const hydrateRoot = () => ({ render: noop, unmount: noop });
export const version = "18.3.1";
export default {
  render,
  hydrate,
  createPortal,
  flushSync,
  unstable_batchedUpdates,
  createRoot,
  hydrateRoot,
  version,
};
