// Minimal React stub: enough surface to satisfy Plate's static imports
// at module load time. None of these are invoked during markdown
// serialize/deserialize, so they only need to exist.

const noop = () => {};
const identity = (x) => x;
const useState = (init) => [typeof init === "function" ? init() : init, noop];
const useRef = (init) => ({ current: init });
const useMemo = (fn) => fn();
const useCallback = (fn) => fn;
const useEffect = noop;
const useLayoutEffect = noop;
const useContext = () => undefined;
const useReducer = (_r, init, initFn) => [initFn ? initFn(init) : init, noop];
const useImperativeHandle = noop;
const useDebugValue = noop;
const useId = () => "react-id";
const useSyncExternalStore = (_sub, get) => (get ? get() : undefined);
const useTransition = () => [false, (fn) => fn()];
const useDeferredValue = identity;
const useInsertionEffect = noop;

const createContext = (defaultValue) => ({
  Provider: ({ children }) => children,
  Consumer: ({ children }) =>
    typeof children === "function" ? children(defaultValue) : null,
  _currentValue: defaultValue,
  displayName: undefined,
});

const forwardRef = (render) => {
  const fn = (props, ref) => render(props, ref);
  fn.$$typeof = Symbol.for("react.forward_ref");
  return fn;
};
const memo = (component) => component;
const lazy = (loader) => ({ _loader: loader });
const Fragment = Symbol.for("react.fragment");
const StrictMode = Symbol.for("react.strict_mode");
const Suspense = Symbol.for("react.suspense");

const createElement = (type, props, ...children) => ({
  type,
  props: { ...(props || {}), children },
});
const cloneElement = (el, props, ...children) => ({
  ...el,
  props: { ...el.props, ...(props || {}), children: children.length ? children : el.props.children },
});
const isValidElement = (val) =>
  !!val && typeof val === "object" && "type" in val && "props" in val;

const Children = {
  map: (children, fn) => (Array.isArray(children) ? children.map(fn) : children == null ? children : [fn(children, 0)]),
  forEach: (children, fn) => {
    if (Array.isArray(children)) children.forEach(fn);
    else if (children != null) fn(children, 0);
  },
  count: (children) => (Array.isArray(children) ? children.length : children == null ? 0 : 1),
  only: (children) => (Array.isArray(children) ? children[0] : children),
  toArray: (children) => (Array.isArray(children) ? children.slice() : children == null ? [] : [children]),
};

const startTransition = (fn) => fn();
const version = "18.3.1";

export {
  useState,
  useRef,
  useMemo,
  useCallback,
  useEffect,
  useLayoutEffect,
  useContext,
  useReducer,
  useImperativeHandle,
  useDebugValue,
  useId,
  useSyncExternalStore,
  useTransition,
  useDeferredValue,
  useInsertionEffect,
  createContext,
  forwardRef,
  memo,
  lazy,
  Fragment,
  StrictMode,
  Suspense,
  createElement,
  cloneElement,
  isValidElement,
  Children,
  startTransition,
  version,
};

export default {
  useState,
  useRef,
  useMemo,
  useCallback,
  useEffect,
  useLayoutEffect,
  useContext,
  useReducer,
  useImperativeHandle,
  useDebugValue,
  useId,
  useSyncExternalStore,
  useTransition,
  useDeferredValue,
  useInsertionEffect,
  createContext,
  forwardRef,
  memo,
  lazy,
  Fragment,
  StrictMode,
  Suspense,
  createElement,
  cloneElement,
  isValidElement,
  Children,
  startTransition,
  version,
};
