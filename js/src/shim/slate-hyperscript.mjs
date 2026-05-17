// slate-hyperscript is for JSX-like editor construction in tests. The
// markdown plugin doesn't actually run jsx() during serialize / deserialize;
// it just imports the name. Stub to a no-op.
const jsx = () => ({});
const createEditor = () => ({ children: [] });
export { jsx, createEditor };
export default { jsx, createEditor };
