// JS entry compiled to WASM by Javy. Communicates with the Go host via JSON
// on stdin/stdout. One invocation processes one top-level request, which is
// either a single op `{op, ...}` or a batch `{ops: [...]}`.

import { createSlateEditor } from "platejs";
import { MarkdownPlugin } from "@platejs/markdown";
import {
  BaseBasicBlocksPlugin,
  BaseHeadingPlugin,
  BaseBlockquotePlugin,
  BaseHorizontalRulePlugin,
  BaseBasicMarksPlugin,
  BaseBoldPlugin,
  BaseItalicPlugin,
  BaseUnderlinePlugin,
  BaseStrikethroughPlugin,
  BaseCodePlugin,
} from "@platejs/basic-nodes";
import { BaseListPlugin } from "@platejs/list";
import { BaseLinkPlugin } from "@platejs/link";
import {
  BaseCodeBlockPlugin,
  BaseCodeLinePlugin,
  BaseCodeSyntaxPlugin,
} from "@platejs/code-block";
import {
  BaseTablePlugin,
  BaseTableRowPlugin,
  BaseTableCellPlugin,
  BaseTableCellHeaderPlugin,
} from "@platejs/table";
import {
  BaseImagePlugin,
  BaseAudioPlugin,
  BaseVideoPlugin,
  BaseFilePlugin,
  BaseMediaEmbedPlugin,
} from "@platejs/media";

// The default plugin set covers everything @platejs/markdown's built-in rules
// can serialize / deserialize. A consumer can disable a category via
// `options.disable = ["lists", "tables", ...]` or pass plugin-specific
// configuration via `options.pluginOptions = { ... }`.
const PLUGIN_CATEGORIES = {
  basic: [BaseBasicBlocksPlugin, BaseHeadingPlugin, BaseBlockquotePlugin, BaseHorizontalRulePlugin],
  marks: [
    BaseBasicMarksPlugin,
    BaseBoldPlugin,
    BaseItalicPlugin,
    BaseUnderlinePlugin,
    BaseStrikethroughPlugin,
    BaseCodePlugin,
  ],
  lists: [BaseListPlugin],
  links: [BaseLinkPlugin],
  code: [BaseCodeBlockPlugin, BaseCodeLinePlugin, BaseCodeSyntaxPlugin],
  tables: [BaseTablePlugin, BaseTableRowPlugin, BaseTableCellPlugin, BaseTableCellHeaderPlugin],
  media: [BaseImagePlugin, BaseAudioPlugin, BaseVideoPlugin, BaseFilePlugin, BaseMediaEmbedPlugin],
};

const readAllStdin = () => {
  const chunks = [];
  const buf = new Uint8Array(8192);
  while (true) {
    const n = Javy.IO.readSync(0, buf);
    if (n === 0) break;
    chunks.push(buf.slice(0, n));
  }
  let total = 0;
  for (const c of chunks) total += c.length;
  const all = new Uint8Array(total);
  let off = 0;
  for (const c of chunks) {
    all.set(c, off);
    off += c.length;
  }
  return new TextDecoder().decode(all);
};

const writeStdout = (str) => {
  Javy.IO.writeSync(1, new TextEncoder().encode(str));
};

const errPayload = (e) => ({
  ok: false,
  error: e && e.message ? e.message : String(e),
  stack: e && e.stack ? e.stack : undefined,
});

const selectPlugins = (options) => {
  const disable = new Set(Array.isArray(options?.disable) ? options.disable : []);
  const out = [];
  for (const [name, plugins] of Object.entries(PLUGIN_CATEGORIES)) {
    if (disable.has(name)) continue;
    for (const p of plugins) out.push(p);
  }
  return out;
};

const buildMarkdownPlugin = (options) => {
  const mdOpts = options?.markdown;
  return mdOpts ? MarkdownPlugin.configure({ options: mdOpts }) : MarkdownPlugin;
};

const buildEditor = (options) => {
  const plugins = [...selectPlugins(options), buildMarkdownPlugin(options)];
  return createSlateEditor({ plugins });
};

const handleOp = (op) => {
  switch (op.op) {
    case "md_to_plate": {
      const editor = buildEditor(op.options);
      const value = editor.api.markdown.deserialize(op.md || "");
      return { ok: true, data: value };
    }
    case "plate_to_md": {
      const editor = buildEditor(op.options);
      if (Array.isArray(op.plate)) editor.children = op.plate;
      const md = editor.api.markdown.serialize();
      return { ok: true, data: md };
    }
    case "ping": {
      return { ok: true, data: "pong" };
    }
    default:
      return { ok: false, error: `unknown op: ${op.op}` };
  }
};

(() => {
  let req;
  try {
    req = JSON.parse(readAllStdin());
  } catch (e) {
    writeStdout(JSON.stringify({ ok: false, error: `request parse: ${e.message}` }));
    return;
  }

  if (Array.isArray(req?.ops)) {
    const results = req.ops.map((op) => {
      try {
        return handleOp(op);
      } catch (e) {
        return errPayload(e);
      }
    });
    writeStdout(JSON.stringify({ ok: true, results }));
    return;
  }

  let result;
  try {
    result = handleOp(req);
  } catch (e) {
    result = errPayload(e);
  }
  writeStdout(JSON.stringify(result));
})();
