package pure

import (
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"
)

// serializeMarkdown is the entry point. It mirrors the remark-stringify
// defaults used by the WASM backend:
//   - italic uses `_…_`
//   - unordered lists use `* `
//   - ordered lists use `N. ` with preserved listStart
//   - a trailing newline is always present
func serializeMarkdown(value []Node, opts *Options) string {
	s := &serializer{opts: opts}
	s.blocks(value)
	out := s.b.String()
	// Always end with exactly one trailing newline (remark default).
	out = strings.TrimRight(out, "\n")
	if out == "" {
		return ""
	}
	return out + "\n"
}

type serializer struct {
	b    strings.Builder
	opts *Options
}

// blocks walks a slice of top-level Plate nodes, inserting blank lines
// between block separators and grouping consecutive list-item nodes.
func (s *serializer) blocks(value []Node) {
	for i := 0; i < len(value); i++ {
		n := value[i]
		// Group runs of flat list items (same indent root) so we emit
		// them together without blank lines in between.
		if isListItem(n) {
			j := i
			for j < len(value) && isListItem(value[j]) {
				j++
			}
			if i > 0 {
				s.writeBlankLine()
			}
			s.flatList(value[i:j])
			i = j - 1
			continue
		}
		if i > 0 {
			s.writeBlankLine()
		}
		s.block(n, "")
	}
}

// block emits a single non-list block, prefixing each emitted line with
// linePrefix (used inside blockquotes).
func (s *serializer) block(n Node, linePrefix string) {
	typ, _ := n["type"].(string)
	switch typ {
	case "h1", "h2", "h3", "h4", "h5", "h6":
		level := int(typ[1] - '0')
		s.writePrefix(linePrefix)
		s.b.WriteString(strings.Repeat("#", level))
		s.b.WriteByte(' ')
		s.inlines(asAny(n["children"]))
		s.b.WriteByte('\n')
	case "p", "":
		s.writePrefix(linePrefix)
		s.inlines(asAny(n["children"]))
		s.b.WriteByte('\n')
	case "blockquote":
		s.blockquote(n, linePrefix)
	case "hr":
		s.writePrefix(linePrefix)
		s.b.WriteString("---\n")
	case "code_block":
		s.codeBlock(n, linePrefix)
	case "img":
		s.writePrefix(linePrefix)
		s.image(n)
		s.b.WriteByte('\n')
	case "table":
		s.table(n, linePrefix)
	case "ul", "ol":
		s.nestedList(n, linePrefix, 0)
	default:
		// Unknown block: fall back to flattening its text.
		s.writePrefix(linePrefix)
		s.inlines(asAny(n["children"]))
		s.b.WriteByte('\n')
	}
}

// writeBlankLine emits "\n" iff the buffer doesn't already end with two
// newlines (one blank line of separation).
func (s *serializer) writeBlankLine() {
	cur := s.b.String()
	if !strings.HasSuffix(cur, "\n") {
		s.b.WriteByte('\n')
	}
	s.b.WriteByte('\n')
}

func (s *serializer) writePrefix(p string) {
	if p != "" {
		s.b.WriteString(p)
	}
}

// blockquote writes its children with a `> ` prefix on each line. We
// re-serialize via a child serializer so nested blocks (paragraphs,
// nested quotes) keep their structure, then prefix every produced line.
func (s *serializer) blockquote(n Node, linePrefix string) {
	inner := &serializer{opts: s.opts}
	children := childrenAsNodes(n)
	inner.blocks(children)
	text := strings.TrimRight(inner.b.String(), "\n")
	if text == "" {
		s.writePrefix(linePrefix)
		s.b.WriteString("> \n")
		return
	}
	for _, line := range strings.Split(text, "\n") {
		s.writePrefix(linePrefix)
		if line == "" {
			s.b.WriteString(">\n")
		} else {
			s.b.WriteString("> ")
			s.b.WriteString(line)
			s.b.WriteByte('\n')
		}
	}
}

func (s *serializer) codeBlock(n Node, linePrefix string) {
	lang, _ := n["lang"].(string)
	s.writePrefix(linePrefix)
	s.b.WriteString("```")
	if lang != "" {
		s.b.WriteString(lang)
	}
	s.b.WriteByte('\n')
	for _, c := range asAny(n["children"]) {
		line, _ := c.(map[string]any)
		if line == nil {
			continue
		}
		var sb strings.Builder
		for _, t := range asAny(line["children"]) {
			tm, _ := t.(map[string]any)
			if tm == nil {
				continue
			}
			if txt, ok := tm["text"].(string); ok {
				sb.WriteString(txt)
			}
		}
		s.writePrefix(linePrefix)
		s.b.WriteString(sb.String())
		s.b.WriteByte('\n')
	}
	s.writePrefix(linePrefix)
	s.b.WriteString("```\n")
}

func (s *serializer) image(n Node) {
	url, _ := n["url"].(string)
	var alt string
	for _, c := range asAny(n["caption"]) {
		cm, _ := c.(map[string]any)
		if cm == nil {
			continue
		}
		if t, ok := cm["text"].(string); ok {
			alt += t
		}
	}
	fmt.Fprintf(&s.b, "![%s](%s)", alt, url)
}

// flatList serializes a run of flat list-item paragraphs. items must
// all be list items (isListItem == true). Nesting is determined by
// `indent`.
func (s *serializer) flatList(items []Node) {
	for _, item := range items {
		indent := intField(item, "indent")
		if indent < 1 {
			indent = 1
		}
		style, _ := item["listStyleType"].(string)
		marker := "* "
		if style == "decimal" {
			start := intField(item, "listStart")
			if start <= 0 {
				start = 1
			}
			marker = strconv.Itoa(start) + ". "
		}
		pad := strings.Repeat("  ", indent-1)
		s.b.WriteString(pad)
		s.b.WriteString(marker)
		s.inlines(asAny(item["children"]))
		s.b.WriteByte('\n')
	}
}

// nestedList handles the ul/ol/li/lic shape (used when the lists plugin
// is disabled).
func (s *serializer) nestedList(n Node, linePrefix string, depth int) {
	ordered := n["type"] == "ol"
	counter := 1
	for _, c := range asAny(n["children"]) {
		li, _ := c.(map[string]any)
		if li == nil {
			continue
		}
		marker := "* "
		if ordered {
			marker = strconv.Itoa(counter) + ". "
			counter++
		}
		pad := strings.Repeat("  ", depth)
		s.b.WriteString(linePrefix)
		s.b.WriteString(pad)
		s.b.WriteString(marker)
		// li children are usually a single lic + optional nested list.
		licWritten := false
		for _, cc := range asAny(li["children"]) {
			cm, _ := cc.(map[string]any)
			if cm == nil {
				continue
			}
			switch cm["type"] {
			case "lic":
				if licWritten {
					s.b.WriteByte('\n')
					s.b.WriteString(linePrefix)
					s.b.WriteString(strings.Repeat("  ", depth+1))
				}
				s.inlines(asAny(cm["children"]))
				s.b.WriteByte('\n')
				licWritten = true
			case "ul", "ol":
				s.nestedList(cm, linePrefix, depth+1)
			}
		}
		if !licWritten {
			s.b.WriteByte('\n')
		}
	}
}

// ----- tables -----

func (s *serializer) table(n Node, linePrefix string) {
	rows := asAny(n["children"])
	if len(rows) == 0 {
		return
	}
	// Materialize cell text per row, plus per-column widths.
	var matrix [][]string
	var widths []int
	for _, r := range rows {
		rm, _ := r.(map[string]any)
		if rm == nil {
			continue
		}
		var rowCells []string
		for _, c := range asAny(rm["children"]) {
			cm, _ := c.(map[string]any)
			if cm == nil {
				continue
			}
			cellText := s.tableCellText(cm)
			rowCells = append(rowCells, cellText)
		}
		matrix = append(matrix, rowCells)
		for i, cell := range rowCells {
			w := utf8.RuneCountInString(cell)
			if i >= len(widths) {
				widths = append(widths, w)
			} else if w > widths[i] {
				widths[i] = w
			}
		}
	}
	// Minimum width 3 for the separator row to render properly.
	for i := range widths {
		if widths[i] < 3 {
			widths[i] = 3
		}
	}
	// Emit header (first row), separator, then body.
	if len(matrix) == 0 {
		return
	}
	s.writeTableRow(matrix[0], widths, linePrefix)
	s.writeTableSeparator(widths, linePrefix)
	for _, row := range matrix[1:] {
		s.writeTableRow(row, widths, linePrefix)
	}
}

func (s *serializer) tableCellText(cell Node) string {
	// Cell children are usually a single `p` wrapping inline content.
	var sb strings.Builder
	for _, c := range asAny(cell["children"]) {
		cm, _ := c.(map[string]any)
		if cm == nil {
			continue
		}
		if cm["type"] == "p" || cm["type"] == nil || cm["type"] == "" {
			// inline render into the buffer
			inner := &serializer{opts: s.opts}
			inner.inlines(asAny(cm["children"]))
			sb.WriteString(inner.b.String())
		} else if txt, ok := cm["text"].(string); ok {
			sb.WriteString(txt)
		}
	}
	return sb.String()
}

func (s *serializer) writeTableRow(cells []string, widths []int, linePrefix string) {
	s.b.WriteString(linePrefix)
	s.b.WriteString("|")
	for i, w := range widths {
		s.b.WriteByte(' ')
		var cell string
		if i < len(cells) {
			cell = cells[i]
		}
		s.b.WriteString(cell)
		pad := w - utf8.RuneCountInString(cell)
		if pad > 0 {
			s.b.WriteString(strings.Repeat(" ", pad))
		}
		s.b.WriteString(" |")
	}
	s.b.WriteByte('\n')
}

func (s *serializer) writeTableSeparator(widths []int, linePrefix string) {
	s.b.WriteString(linePrefix)
	s.b.WriteString("|")
	for _, w := range widths {
		s.b.WriteByte(' ')
		s.b.WriteString(strings.Repeat("-", w))
		s.b.WriteString(" |")
	}
	s.b.WriteByte('\n')
}

// ----- inlines -----

// inlines emits a run of inline nodes (text + links). Marks are wrapped
// per text node; runs of same-mark text are not currently merged.
func (s *serializer) inlines(children []any) {
	for _, c := range children {
		cm, _ := c.(map[string]any)
		if cm == nil {
			continue
		}
		if t, ok := cm["text"]; ok {
			s.writeTextWithMarks(asString(t), cm)
			continue
		}
		switch cm["type"] {
		case "a":
			s.writeLink(cm)
		default:
			// Unknown inline: emit its inline children.
			s.inlines(asAny(cm["children"]))
		}
	}
}

func (s *serializer) writeTextWithMarks(t string, attrs map[string]any) {
	if asBool(attrs["code"]) {
		// Inline code: no other marks combine with backticks in remark.
		s.b.WriteByte('`')
		s.b.WriteString(t)
		s.b.WriteByte('`')
		return
	}
	// Order chosen to match remark-stringify output: bold outermost,
	// then strikethrough, then italic innermost. Combining is rare
	// enough that exact byte parity here isn't a goal.
	var pre, post strings.Builder
	if asBool(attrs["bold"]) {
		pre.WriteString("**")
		post.WriteString("**")
	}
	if asBool(attrs["strikethrough"]) {
		pre.WriteString("~~")
		post.WriteString("~~")
	}
	if asBool(attrs["italic"]) {
		pre.WriteByte('_')
		post.WriteByte('_')
	}
	s.b.WriteString(pre.String())
	s.b.WriteString(t)
	// post wrappers must be reversed
	s.b.WriteString(reverseString(post.String()))
	// Trailing underline support: Plate has BaseUnderlinePlugin but
	// markdown has no underline syntax. remark-stringify drops it; we
	// do too.
}

func (s *serializer) writeLink(n Node) {
	url, _ := n["url"].(string)
	s.b.WriteByte('[')
	s.inlines(asAny(n["children"]))
	s.b.WriteString("](")
	s.b.WriteString(url)
	s.b.WriteByte(')')
}

// ----- small helpers -----

func asAny(v any) []any {
	if v == nil {
		return nil
	}
	if a, ok := v.([]any); ok {
		return a
	}
	// Sometimes children may be typed []Node — convert.
	if ns, ok := v.([]Node); ok {
		out := make([]any, len(ns))
		for i, n := range ns {
			out[i] = n
		}
		return out
	}
	return nil
}

func asString(v any) string {
	s, _ := v.(string)
	return s
}

func asBool(v any) bool {
	b, _ := v.(bool)
	return b
}

func intField(n Node, key string) int {
	switch v := n[key].(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	case float32:
		return int(v)
	}
	return 0
}

func isListItem(n Node) bool {
	if n == nil {
		return false
	}
	if n["type"] != "p" {
		return false
	}
	style, _ := n["listStyleType"].(string)
	return style != ""
}

func childrenAsNodes(n Node) []Node {
	var out []Node
	for _, c := range asAny(n["children"]) {
		if m, ok := c.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out
}

func reverseString(s string) string {
	// Operating on bytes is safe: post-wrappers above are all ASCII.
	if s == "" {
		return ""
	}
	out := make([]byte, len(s))
	for i := 0; i < len(s); i++ {
		out[i] = s[len(s)-1-i]
	}
	return string(out)
}
