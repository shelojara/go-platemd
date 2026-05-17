// Headless build: editor never sees a keyboard event, so hotkey matching
// is unreachable. These functions only need to exist as imports.
const stub = () => false;
export const isHotkey = stub;
export const isKeyHotkey = stub;
export default stub;
