// @platejs/markdown lists `marked` as a dep but parses via remark/unified.
// Stub marked to a function that throws if anyone actually calls it.
const marked = (s) => {
  throw new Error("marked() stubbed out in platemd WASM build; report this if you hit it");
};
marked.parse = marked;
marked.lexer = marked;
marked.Renderer = function () {};
marked.use = () => {};
export { marked };
export default marked;
