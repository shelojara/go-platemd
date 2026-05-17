// slate-dom is all DOM-side helpers. The headless serialize / deserialize
// path never touches a DOM node. Re-export everything as a no-op so the
// static imports in @platejs/slate resolve.
const noopFn = () => undefined;
const noopBool = () => false;
const passthrough = (x) => x;

export const CAN_USE_DOM = false;
export const HAS_BEFORE_INPUT_SUPPORT = false;
export const IS_ANDROID = false;
export const IS_CHROME = false;
export const IS_FIREFOX = false;
export const IS_FIREFOX_LEGACY = false;
export const IS_IOS = false;
export const IS_UC_MOBILE = false;
export const IS_WEBKIT = false;
export const IS_WECHATBROWSER = false;
export const TRIPLE_CLICK = "triple_click";

export const DOMEditor = {};
export const applyStringDiff = passthrough;
export const getActiveElement = () => null;
export const getDefaultView = () => null;
export const getSelection = () => null;
export const hasShadowRoot = noopBool;
export const isAfter = noopBool;
export const isBefore = noopBool;
export const isDOMElement = noopBool;
export const isDOMNode = noopBool;
export const isDOMSelection = noopBool;
export const isElementDecorationsEqual = noopBool;
export const isPlainTextOnlyPaste = noopBool;
export const isTextDecorationsEqual = noopBool;
export const isTrackedMutation = noopBool;
export const mergeStringDiffs = passthrough;
export const normalizeDOMPoint = passthrough;
export const normalizePoint = passthrough;
export const normalizeRange = passthrough;
export const normalizeStringDiff = passthrough;
export const targetRange = noopFn;
export const verifyDiffState = noopBool;
export const withDOM = (editor) => editor;

export default {};
