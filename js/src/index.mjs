// JS entry compiled to WASM by Javy. Talks length-framed JSON to the Go host:
// each message is a 4-byte big-endian length prefix followed by that many
// bytes of UTF-8 JSON. The loop runs until stdin reaches EOF, so the same
// build serves both one-shot mode (Go writes one frame then closes stdin)
// and worker mode (Go keeps the pipe open for many frames).

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

// Plugin categories. Consumers can drop whole categories per call via
// `options.disable`. Math is intentionally excluded — KaTeX has DOM-load
// code that crashes under Javy.
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

const allDefaultPlugins = () => {
  const out = [];
  for (const plugins of Object.values(PLUGIN_CATEGORIES)) for (const p of plugins) out.push(p);
  return out;
};

const selectPlugins = (options) => {
  if (!options?.disable || !options.disable.length) return allDefaultPlugins();
  const disable = new Set(options.disable);
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

const buildEditor = (options) =>
  createSlateEditor({ plugins: [...selectPlugins(options), buildMarkdownPlugin(options)] });

// Built once at module load — this is the big latency win. Calls that
// pass no `options` reuse this editor across the entire process lifetime.
// The set_defaults op rebuilds it with a configured baseline.
let defaultEditor = buildEditor();
let defaultOptions = null;

const resetEditor = (editor) => {
  // Slate's value lives in editor.children. Resetting it is enough for
  // headless serialize / deserialize; selection / history are unused.
  editor.children = [];
};

const errPayload = (e) => ({
  ok: false,
  error: e && e.message ? e.message : String(e),
  stack: e && e.stack ? e.stack : undefined,
});

const handleOp = (op) => {
  // set_defaults updates the cached default editor and the stored default
  // options. Subsequent ops without per-call options use the new cache.
  if (op.op === "set_defaults") {
    defaultOptions = op.options || null;
    defaultEditor = buildEditor(defaultOptions);
    return { ok: true, data: null };
  }

  const editor = op.options ? buildEditor(op.options) : defaultEditor;
  switch (op.op) {
    case "md_to_plate": {
      resetEditor(editor);
      const value = editor.api.markdown.deserialize(op.md || "");
      return { ok: true, data: value };
    }
    case "plate_to_md": {
      editor.children = Array.isArray(op.plate) ? op.plate : [];
      const md = editor.api.markdown.serialize();
      return { ok: true, data: md };
    }
    case "ping":
      return { ok: true, data: "pong" };
    default:
      return { ok: false, error: `unknown op: ${op.op}` };
  }
};

const handleRequest = (req) => {
  if (Array.isArray(req?.ops)) {
    const results = req.ops.map((op) => {
      try { return handleOp(op); } catch (e) { return errPayload(e); }
    });
    return { ok: true, results };
  }
  try { return handleOp(req); } catch (e) { return errPayload(e); }
};

// ---------------- framing ----------------

const readExact = (n) => {
  const buf = new Uint8Array(n);
  let read = 0;
  while (read < n) {
    const got = Javy.IO.readSync(0, buf.subarray(read));
    if (got === 0) return null; // EOF
    read += got;
  }
  return buf;
};

const readFrame = () => {
  const header = readExact(4);
  if (header === null) return null;
  const len = (header[0] << 24) | (header[1] << 16) | (header[2] << 8) | header[3];
  if (len === 0) return ""; // explicit empty frame
  const body = readExact(len);
  if (body === null) return null;
  return new TextDecoder().decode(body);
};

const writeFrame = (str) => {
  const bytes = new TextEncoder().encode(str);
  const header = new Uint8Array(4);
  header[0] = (bytes.length >>> 24) & 0xff;
  header[1] = (bytes.length >>> 16) & 0xff;
  header[2] = (bytes.length >>> 8) & 0xff;
  header[3] = bytes.length & 0xff;
  Javy.IO.writeSync(1, header);
  if (bytes.length) Javy.IO.writeSync(1, bytes);
};

// ---------------- main loop ----------------

while (true) {
  const reqStr = readFrame();
  if (reqStr === null) break; // stdin closed
  let resp;
  try {
    resp = handleRequest(JSON.parse(reqStr));
  } catch (e) {
    resp = { ok: false, error: `request parse: ${e.message}` };
  }
  writeFrame(JSON.stringify(resp));
}
