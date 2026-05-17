package platemd

// Option configures a *Converter at construction time. Currently used to
// set baseline Options that apply to every call unless the caller passes
// per-call overrides.
type Option func(*converterConfig)

type converterConfig struct {
	defaults *Options
}

// WithDefaultOptions sets a baseline Options that applies to every call
// on the Converter (and every Worker created from it). Per-call options
// passed to MarkdownToPlate / PlateToMarkdown / Batch override the
// baseline field-by-field: a non-nil Disable on the per-call value
// replaces the default Disable; a non-nil Markdown map replaces the
// default Markdown map. Pass-through, not merge — keeping the rule
// simple is more predictable than per-field deep-merge.
func WithDefaultOptions(o Options) Option {
	return func(c *converterConfig) {
		copy := o
		c.defaults = &copy
	}
}

// mergeOptions returns the effective Options for a call: per-call wins
// on every non-nil field, defaults fill in the rest. Returns nil only
// when both inputs are nil — in that case no `options` payload is sent
// to the JS side and it uses its cached default editor.
func mergeOptions(defaults, perCall *Options) *Options {
	if defaults == nil {
		return perCall
	}
	if perCall == nil {
		out := *defaults
		return &out
	}
	out := *defaults
	if perCall.Disable != nil {
		out.Disable = perCall.Disable
	}
	if perCall.Markdown != nil {
		out.Markdown = perCall.Markdown
	}
	return &out
}
