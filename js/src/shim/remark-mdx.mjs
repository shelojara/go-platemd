// We never accept MDX (markdown + JSX) input. A real remark-mdx
// installs a unified plugin that adds MDX parsing rules. Returning a
// no-op unified plugin keeps the plugin pipeline happy without pulling
// in acorn + the entire micromark-mdx-* extension family.
const remarkMdx = function remarkMdx() {
  // unified plugins return a transformer function (tree, file) => tree | void.
  return function transform() {};
};
export default remarkMdx;
