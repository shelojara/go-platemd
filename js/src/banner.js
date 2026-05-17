// Injected as esbuild banner — runs before any bundled module body.
// Provides the minimum browser/Node globals that headless Plate + remark
// touch at module-evaluation time. None of these are functionally needed
// for serialize / deserialize; they only need to exist.
(function installGlobals() {
  var g = globalThis;
  if (typeof g.global === "undefined") g.global = g;
  if (typeof g.self === "undefined") g.self = g;
  if (typeof g.window === "undefined") g.window = g;

  var emptyStyle = new Proxy(
    {},
    { get: function () { return ""; }, set: function () { return true; } }
  );
  var noopElement = {
    style: emptyStyle,
    setAttribute: function () {},
    getAttribute: function () { return null; },
    appendChild: function (c) { return c; },
    removeChild: function (c) { return c; },
    addEventListener: function () {},
    removeEventListener: function () {},
    classList: { add: function () {}, remove: function () {}, toggle: function () {}, contains: function () { return false; } },
    children: [],
    childNodes: [],
    cloneNode: function () { return noopElement; },
    insertBefore: function (c) { return c; },
  };
  if (typeof g.document === "undefined") {
    g.document = {
      createElement: function () { return noopElement; },
      createElementNS: function () { return noopElement; },
      createTextNode: function () { return noopElement; },
      createDocumentFragment: function () { return noopElement; },
      getElementById: function () { return null; },
      querySelector: function () { return null; },
      querySelectorAll: function () { return []; },
      addEventListener: function () {},
      removeEventListener: function () {},
      documentElement: noopElement,
      body: noopElement,
      head: noopElement,
    };
  }
  if (typeof g.navigator === "undefined") {
    g.navigator = { userAgent: "javy", platform: "wasm", language: "en-US" };
  }
  if (typeof g.location === "undefined") {
    g.location = { href: "wasm://platemd/", origin: "wasm://platemd", protocol: "wasm:", host: "platemd", pathname: "/", search: "", hash: "" };
  }
  if (typeof g.localStorage === "undefined") {
    var store = {};
    g.localStorage = {
      getItem: function (k) { return Object.prototype.hasOwnProperty.call(store, k) ? store[k] : null; },
      setItem: function (k, v) { store[k] = String(v); },
      removeItem: function (k) { delete store[k]; },
      clear: function () { store = {}; },
      key: function (i) { return Object.keys(store)[i] || null; },
      get length() { return Object.keys(store).length; },
    };
  }
  if (typeof g.sessionStorage === "undefined") g.sessionStorage = g.localStorage;
  if (typeof g.performance === "undefined") g.performance = { now: function () { return Date.now(); } };
  if (typeof g.queueMicrotask !== "function") {
    g.queueMicrotask = function (cb) {
      Promise.resolve().then(cb);
    };
  }
  if (typeof g.requestAnimationFrame !== "function") {
    g.requestAnimationFrame = function (cb) { return setTimeout(function () { cb(Date.now()); }, 16); };
    g.cancelAnimationFrame = function (id) { clearTimeout(id); };
  }
  // QuickJS in Javy provides Date but not setTimeout / setInterval. We don't
  // need actual deferred execution for headless serialize / deserialize, so
  // these are pure no-ops that never fire (and never queue microtasks).
  if (typeof g.setTimeout !== "function") {
    var nextId = 1;
    g.setTimeout = function () { return nextId++; };
    g.clearTimeout = function () {};
    g.setInterval = function () { return nextId++; };
    g.clearInterval = function () {};
  }
  if (typeof g.MutationObserver === "undefined") {
    g.MutationObserver = function () { return { observe: function () {}, disconnect: function () {}, takeRecords: function () { return []; } }; };
  }
  // nanoid (used by Slate / Plate for node ids) needs crypto.getRandomValues.
  // QuickJS / Javy has no Web Crypto API. Fill with Math.random — not
  // cryptographically secure, but ids only need to be unique within one
  // editor instance for a single serialize/deserialize call.
  if (typeof g.crypto === "undefined" || typeof g.crypto.getRandomValues !== "function") {
    var existing = g.crypto || {};
    existing.getRandomValues = function (buf) {
      for (var i = 0; i < buf.length; i++) buf[i] = Math.floor(Math.random() * 256);
      return buf;
    };
    if (typeof existing.randomUUID !== "function") {
      existing.randomUUID = function () {
        var b = new Uint8Array(16);
        existing.getRandomValues(b);
        b[6] = (b[6] & 0x0f) | 0x40;
        b[8] = (b[8] & 0x3f) | 0x80;
        var h = [];
        for (var i = 0; i < 16; i++) h.push((b[i] + 0x100).toString(16).slice(1));
        return h[0] + h[1] + h[2] + h[3] + "-" + h[4] + h[5] + "-" + h[6] + h[7] + "-" + h[8] + h[9] + "-" + h[10] + h[11] + h[12] + h[13] + h[14] + h[15];
      };
    }
    g.crypto = existing;
  }
})();
