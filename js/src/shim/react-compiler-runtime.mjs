// Stub for react-compiler-runtime. The compiler emits calls to c() to memoize;
// in our headless build we just return a no-op cache that recomputes every time.
export const c = (_size) => new Array(_size).fill(undefined);
export default { c };
