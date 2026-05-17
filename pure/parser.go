package pure

import (
	"net/url"
	"regexp"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	extast "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/text"
)

// parseMarkdown is the entry point: goldmark parses the source, then
// walkBlocks lowers the AST to the Plate node shape.
func parseMarkdown(md string, opts *Options) []Node {
	src := []byte(md)
	parser := goldmark.New(goldmark.WithExtensions(extension.GFM)).Parser()
	doc := parser.Parse(text.NewReader(src))

	w := walker{src: src, opts: opts}
	out := w.blocks(doc, 0)
	if out == nil {
		return []Node{}
	}
	return out
}

// walker holds parser-call-scoped state: source bytes and option flags.
type walker struct {
	src  []byte
	opts *Options
}

// blocks walks the children of n as block-level nodes. indent is the
// current list nesting depth (0 outside any list).
func (w *walker) blocks(n ast.Node, indent int) []Node {
	var out []Node
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		out = append(out, w.block(c, indent)...)
	}
	return out
}

func (w *walker) block(n ast.Node, indent int) []Node {
	switch v := n.(type) {
	case *ast.Heading:
		return []Node{w.heading(v)}
	case *ast.Paragraph:
		return w.paragraph(v, indent)
	case *ast.Blockquote:
		return []Node{w.blockquote(v)}
	case *ast.ThematicBreak:
		return []Node{{"type": "hr", "children": []any{textNode("")}}}
	case *ast.FencedCodeBlock:
		if disabled(w.opts, "code") {
			return []Node{w.codeBlockAsParagraph(v)}
		}
		return []Node{w.fencedCode(v)}
	case *ast.CodeBlock:
		if disabled(w.opts, "code") {
			return []Node{w.codeBlockAsParagraph(v)}
		}
		return []Node{w.indentedCode(v)}
	case *ast.List:
		if disabled(w.opts, "lists") {
			return []Node{w.nestedList(v)}
		}
		return w.flatList(v, indent)
	case *extast.Table:
		if disabled(w.opts, "tables") {
			// Tables degrade to a stack of paragraphs.
			return w.tableFallback(v)
		}
		return []Node{w.table(v)}
	case *ast.HTMLBlock:
		// Preserve raw HTML as a paragraph with text content.
		return []Node{{"type": "p", "children": []any{textNode(string(w.linesText(v)))}}}
	}
	return nil
}

// ----- block builders -----

func (w *walker) heading(h *ast.Heading) Node {
	level := h.Level
	if level < 1 {
		level = 1
	} else if level > 6 {
		level = 6
	}
	if disabled(w.opts, "basic") {
		// Without the heading plugin, the markdown plugin in Plate
		// falls back to a paragraph.
		return Node{"type": "p", "children": w.inlines(h)}
	}
	return Node{"type": "h" + string(rune('0'+level)), "children": w.inlines(h)}
}

func (w *walker) paragraph(p *ast.Paragraph, indent int) []Node {
	// A paragraph whose only inline child is a single image becomes a
	// block-level `img` node in Plate's media plugin output.
	if img := singleImage(p); img != nil && !disabled(w.opts, "media") {
		return []Node{w.imageBlock(img)}
	}

	node := Node{"type": "p", "children": w.inlines(p)}
	if indent > 0 {
		node["indent"] = indent
	}
	return []Node{node}
}

func (w *walker) blockquote(bq *ast.Blockquote) Node {
	if disabled(w.opts, "basic") {
		// No blockquote plugin: emit children as plain paragraphs. The
		// blockquote wrapper is dropped.
		return Node{"type": "p", "children": w.inlines(bq)}
	}
	children := w.blocks(bq, 0)
	if len(children) == 0 {
		children = []Node{{"type": "p", "children": []any{textNode("")}}}
	}
	return Node{"type": "blockquote", "children": nodesAsAny(children)}
}

func (w *walker) fencedCode(fc *ast.FencedCodeBlock) Node {
	lang := string(fc.Language(w.src))
	lines := splitCodeLines(w.linesText(fc))
	codeLines := make([]any, 0, len(lines))
	for _, line := range lines {
		codeLines = append(codeLines, Node{
			"type":     "code_line",
			"children": []any{textNode(line)},
		})
	}
	if len(codeLines) == 0 {
		codeLines = []any{Node{"type": "code_line", "children": []any{textNode("")}}}
	}
	out := Node{"type": "code_block", "children": codeLines}
	if lang != "" {
		out["lang"] = lang
	}
	return out
}

func (w *walker) indentedCode(cb *ast.CodeBlock) Node {
	lines := splitCodeLines(w.linesText(cb))
	codeLines := make([]any, 0, len(lines))
	for _, line := range lines {
		codeLines = append(codeLines, Node{
			"type":     "code_line",
			"children": []any{textNode(line)},
		})
	}
	if len(codeLines) == 0 {
		codeLines = []any{Node{"type": "code_line", "children": []any{textNode("")}}}
	}
	return Node{"type": "code_block", "children": codeLines}
}

// codeBlockAsParagraph is the fallback when the code plugin is disabled:
// emit the code as a single paragraph with its text content.
func (w *walker) codeBlockAsParagraph(n ast.Node) Node {
	return Node{"type": "p", "children": []any{textNode(string(w.linesText(n)))}}
}

// flatList emits the indent-based shape used by the BaseListPlugin: each
// item is a top-level `p` with `indent`, `listStyleType`, and (for
// ordered lists) `listStart`. Nested lists recurse with a higher indent.
func (w *walker) flatList(l *ast.List, parentIndent int) []Node {
	style := "disc"
	if l.IsOrdered() {
		style = "decimal"
	}
	indent := parentIndent + 1
	counter := l.Start
	if counter == 0 {
		counter = 1
	}

	var out []Node
	for li := l.FirstChild(); li != nil; li = li.NextSibling() {
		first := true
		for c := li.FirstChild(); c != nil; c = c.NextSibling() {
			if subList, ok := c.(*ast.List); ok {
				out = append(out, w.flatList(subList, indent)...)
				continue
			}
			node := w.listItemBody(c, indent, style, counter, l.IsOrdered() && first)
			if node != nil {
				out = append(out, node)
				first = false
			}
		}
		counter++
	}
	return out
}

// listItemBody wraps the body of one list-item child (which is usually
// a paragraph or text block) into the flat list-item shape.
func (w *walker) listItemBody(child ast.Node, indent int, style string, listStart int, withStart bool) Node {
	node := Node{
		"type":          "p",
		"indent":        indent,
		"listStyleType": style,
	}
	if withStart {
		node["listStart"] = listStart
	}
	switch v := child.(type) {
	case *ast.Paragraph, *ast.TextBlock:
		node["children"] = w.inlines(v)
	default:
		// Promote non-paragraph block content (e.g. code blocks inside
		// a list item) by emitting its text in a paragraph node. This
		// is a simplification — the WASM backend would emit the actual
		// nested block, but mixed block types inside list items are
		// rare and Plate's indent shape doesn't have a clean
		// representation for them.
		node["children"] = []any{textNode(string(w.linesText(v)))}
	}
	return node
}

// nestedList is the older ul/li/lic shape used when the list plugin is
// disabled.
func (w *walker) nestedList(l *ast.List) Node {
	listType := "ul"
	if l.IsOrdered() {
		listType = "ol"
	}
	var items []any
	for li := l.FirstChild(); li != nil; li = li.NextSibling() {
		var lic []any
		for c := li.FirstChild(); c != nil; c = c.NextSibling() {
			switch v := c.(type) {
			case *ast.Paragraph, *ast.TextBlock:
				lic = append(lic, Node{"type": "lic", "children": w.inlines(v)})
			case *ast.List:
				lic = append(lic, w.nestedList(v))
			}
		}
		if lic == nil {
			lic = []any{Node{"type": "lic", "children": []any{textNode("")}}}
		}
		items = append(items, Node{"type": "li", "children": lic})
	}
	return Node{"type": listType, "children": items}
}

func (w *walker) imageBlock(img *ast.Image) Node {
	caption := w.inlines(img)
	if len(caption) == 0 {
		caption = []any{textNode("")}
	}
	return Node{
		"type":     "img",
		"url":      string(img.Destination),
		"caption":  caption,
		"children": []any{textNode("")},
	}
}

// ----- tables -----

func (w *walker) table(t *extast.Table) Node {
	var rows []any
	for r := t.FirstChild(); r != nil; r = r.NextSibling() {
		isHeader := false
		switch r.(type) {
		case *extast.TableHeader:
			isHeader = true
		}
		cellType := "td"
		if isHeader {
			cellType = "th"
		}
		var cells []any
		for c := r.FirstChild(); c != nil; c = c.NextSibling() {
			cell, ok := c.(*extast.TableCell)
			if !ok {
				continue
			}
			cells = append(cells, Node{
				"type": cellType,
				"children": []any{
					Node{"type": "p", "children": w.inlines(cell)},
				},
			})
		}
		if cells == nil {
			cells = []any{}
		}
		rows = append(rows, Node{"type": "tr", "children": cells})
	}
	if rows == nil {
		rows = []any{}
	}
	return Node{"type": "table", "children": rows}
}

func (w *walker) tableFallback(t *extast.Table) []Node {
	var out []Node
	for r := t.FirstChild(); r != nil; r = r.NextSibling() {
		var parts []string
		for c := r.FirstChild(); c != nil; c = c.NextSibling() {
			cell, ok := c.(*extast.TableCell)
			if !ok {
				continue
			}
			var sb strings.Builder
			collectPlainText(cell, w.src, &sb)
			parts = append(parts, sb.String())
		}
		out = append(out, Node{
			"type":     "p",
			"children": []any{textNode(strings.Join(parts, " | "))},
		})
	}
	return out
}

// ----- inlines -----

func (w *walker) inlines(parent ast.Node) []any {
	marks := map[string]bool{}
	var out []any
	w.walkInlines(parent, marks, &out)
	out = mergeAdjacentText(out)
	if out == nil {
		out = []any{textNode("")}
	}
	return out
}

// mergeAdjacentText concatenates consecutive plain-text nodes that
// carry the same set of marks. goldmark's inline parser often emits
// multiple Text nodes for a single contiguous span (one per
// word/segment boundary it tried as a delimiter); the WASM backend
// merges these via remark/mdast, so we do the same here for parity.
func mergeAdjacentText(in []any) []any {
	if len(in) < 2 {
		return in
	}
	out := make([]any, 0, len(in))
	for _, n := range in {
		m, ok := n.(map[string]any)
		if !ok || !isPlainTextNode(m) {
			out = append(out, n)
			continue
		}
		if len(out) > 0 {
			prev, ok := out[len(out)-1].(map[string]any)
			if ok && isPlainTextNode(prev) && sameMarks(prev, m) {
				prev["text"] = prev["text"].(string) + m["text"].(string)
				continue
			}
		}
		out = append(out, n)
	}
	return out
}

func isPlainTextNode(m map[string]any) bool {
	if _, ok := m["text"].(string); !ok {
		return false
	}
	// A node with a `type` (e.g. links) is not a plain-text node.
	if _, ok := m["type"]; ok {
		return false
	}
	return true
}

func sameMarks(a, b map[string]any) bool {
	count := func(m map[string]any) int {
		n := 0
		for k := range m {
			if k != "text" {
				n++
			}
		}
		return n
	}
	if count(a) != count(b) {
		return false
	}
	for k, v := range a {
		if k == "text" {
			continue
		}
		if b[k] != v {
			return false
		}
	}
	return true
}

func (w *walker) walkInlines(n ast.Node, marks map[string]bool, out *[]any) {
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		switch v := c.(type) {
		case *ast.Text:
			txt := string(v.Segment.Value(w.src))
			if v.SoftLineBreak() {
				txt += "\n"
			} else if v.HardLineBreak() {
				txt += "\n"
			}
			if txt != "" {
				w.appendTextOrMentions(out, txt, marks)
			}
		case *ast.String:
			txt := string(v.Value)
			if txt != "" {
				w.appendTextOrMentions(out, txt, marks)
			}
		case *ast.Emphasis:
			markName := "italic"
			if v.Level >= 2 {
				markName = "bold"
			}
			if disabled(w.opts, "marks") {
				w.walkInlines(v, marks, out)
				continue
			}
			already := marks[markName]
			marks[markName] = true
			w.walkInlines(v, marks, out)
			if !already {
				delete(marks, markName)
			}
		case *ast.CodeSpan:
			var sb strings.Builder
			collectPlainText(v, w.src, &sb)
			if disabled(w.opts, "marks") {
				if sb.Len() > 0 {
					*out = append(*out, textWithMarks(sb.String(), marks))
				}
				continue
			}
			withCode := copyMarks(marks)
			withCode["code"] = true
			*out = append(*out, textWithMarks(sb.String(), withCode))
		case *extast.Strikethrough:
			if disabled(w.opts, "marks") {
				w.walkInlines(v, marks, out)
				continue
			}
			already := marks["strikethrough"]
			marks["strikethrough"] = true
			w.walkInlines(v, marks, out)
			if !already {
				delete(marks, "strikethrough")
			}
		case *ast.Link:
			urlStr := string(v.Destination)
			if strings.HasPrefix(urlStr, "mention:") && !disabled(w.opts, "mentions") {
				*out = append(*out, w.mentionFromLink(v, urlStr))
				continue
			}
			children := w.inlines(v)
			if disabled(w.opts, "links") {
				// Drop the link, keep the inner text inline.
				*out = append(*out, children...)
				continue
			}
			link := Node{"type": "a", "url": urlStr, "children": children}
			*out = append(*out, link)
		case *ast.AutoLink:
			url := string(v.URL(w.src))
			if disabled(w.opts, "links") {
				*out = append(*out, textWithMarks(url, marks))
				continue
			}
			*out = append(*out, Node{
				"type":     "a",
				"url":      url,
				"children": []any{textNode(url)},
			})
		case *ast.Image:
			// Inline images appearing mid-paragraph are flattened to
			// their alt text. Block-level images (image-only paragraph)
			// are handled in paragraph() before we ever get here.
			var sb strings.Builder
			collectPlainText(v, w.src, &sb)
			if sb.Len() == 0 {
				sb.WriteString(string(v.Destination))
			}
			*out = append(*out, textWithMarks(sb.String(), marks))
		case *ast.RawHTML:
			var sb strings.Builder
			for i := 0; i < v.Segments.Len(); i++ {
				seg := v.Segments.At(i)
				sb.Write(seg.Value(w.src))
			}
			if sb.Len() > 0 {
				*out = append(*out, textWithMarks(sb.String(), marks))
			}
		case *extast.TaskCheckBox:
			// Render as a sentinel; the surrounding paragraph already
			// captures the rest of the item text.
			marker := "[ ] "
			if v.IsChecked {
				marker = "[x] "
			}
			*out = append(*out, textWithMarks(marker, marks))
		default:
			// Fall through into children to avoid silently dropping
			// content from unknown node types.
			w.walkInlines(v, marks, out)
		}
	}
}

// ----- helpers -----

func textNode(s string) Node {
	return Node{"text": s}
}

func textWithMarks(s string, marks map[string]bool) Node {
	n := Node{"text": s}
	for k, v := range marks {
		if v {
			n[k] = true
		}
	}
	return n
}

func copyMarks(m map[string]bool) map[string]bool {
	out := make(map[string]bool, len(m)+1)
	for k, v := range m {
		out[k] = v
	}
	return out
}

func nodesAsAny(in []Node) []any {
	out := make([]any, len(in))
	for i, n := range in {
		out[i] = n
	}
	return out
}

// linesText concatenates the source bytes covered by a block node's
// Lines() segments. Used for fenced code / indented code / raw HTML.
func (w *walker) linesText(n ast.Node) []byte {
	lines := n.Lines()
	if lines == nil {
		return nil
	}
	var buf []byte
	for i := 0; i < lines.Len(); i++ {
		seg := lines.At(i)
		buf = append(buf, seg.Value(w.src)...)
	}
	return buf
}

// splitCodeLines splits raw code content into lines, dropping a single
// trailing empty line (so a fence ending in `\n` doesn't produce a blank
// extra code_line).
func splitCodeLines(b []byte) []string {
	s := string(b)
	if s == "" {
		return nil
	}
	if strings.HasSuffix(s, "\n") {
		s = s[:len(s)-1]
	}
	return strings.Split(s, "\n")
}

// singleImage returns the single Image child of p, or nil. Used to
// detect image-only paragraphs that should become block `img` nodes.
func singleImage(p *ast.Paragraph) *ast.Image {
	c := p.FirstChild()
	if c == nil {
		return nil
	}
	if c.NextSibling() != nil {
		return nil
	}
	img, _ := c.(*ast.Image)
	return img
}

// ----- mentions -----

// mentionPattern mirrors the regex used by @platejs/markdown's
// remark-mention: an `@` must follow start-of-string or whitespace, and
// the username (alphanumerics, `_`, `-`) must be followed by whitespace,
// end-of-string, or one of `.,;:!?)`. Trailing context is checked
// manually because Go's regexp doesn't support lookahead.
var mentionPattern = regexp.MustCompile(`(^|\s)@([A-Za-z0-9_-]+)`)

func isMentionTrailingByte(c byte) bool {
	switch c {
	case ' ', '\t', '\n', '\r', '.', ',', ';', ':', '!', '?', ')':
		return true
	}
	return false
}

// appendTextOrMentions splits txt at `@user` patterns and appends the
// resulting text + mention nodes onto out, preserving marks on the text
// segments. When the mentions category is disabled, the text is emitted
// untouched.
func (w *walker) appendTextOrMentions(out *[]any, txt string, marks map[string]bool) {
	if disabled(w.opts, "mentions") {
		*out = append(*out, textWithMarks(txt, marks))
		return
	}
	matches := mentionPattern.FindAllStringSubmatchIndex(txt, -1)
	if matches == nil {
		*out = append(*out, textWithMarks(txt, marks))
		return
	}
	cursor := 0
	for _, m := range matches {
		// m[0:2] = whole match, m[2:4] = leading (^|\s), m[4:6] = username
		usernameEnd := m[5]
		if usernameEnd < len(txt) && !isMentionTrailingByte(txt[usernameEnd]) {
			// Trailing context fails — leave this @x in text and try the next.
			continue
		}
		// Start the mention at the `@` (skip the leading whitespace, if any,
		// so it stays with the surrounding text segment).
		atStart := m[0]
		if m[3] > m[2] {
			atStart++
		}
		if atStart > cursor {
			*out = append(*out, textWithMarks(txt[cursor:atStart], marks))
		}
		username := txt[m[4]:m[5]]
		*out = append(*out, Node{
			"type":     "mention",
			"value":    username,
			"children": []any{textNode("")},
		})
		cursor = usernameEnd
	}
	if cursor < len(txt) {
		*out = append(*out, textWithMarks(txt[cursor:], marks))
	}
}

// mentionFromLink turns a `[label](mention:id)` link into a Plate mention
// node. The mention `key` is only set when the encoded id and the display
// label diverge, matching @platejs/markdown's `mention` rule.
func (w *walker) mentionFromLink(v *ast.Link, dest string) Node {
	rawID := strings.TrimPrefix(dest, "mention:")
	id, err := url.QueryUnescape(rawID)
	if err != nil {
		id = rawID
	}
	var sb strings.Builder
	collectPlainText(v, w.src, &sb)
	display := sb.String()
	if display == "" {
		display = id
	}
	n := Node{
		"type":     "mention",
		"value":    display,
		"children": []any{textNode("")},
	}
	if display != id {
		n["key"] = id
	}
	return n
}

// collectPlainText recursively concatenates Text/String values under n,
// ignoring all formatting. Used for code spans and link/image alt text.
func collectPlainText(n ast.Node, src []byte, sb *strings.Builder) {
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		switch v := c.(type) {
		case *ast.Text:
			sb.Write(v.Segment.Value(src))
		case *ast.String:
			sb.Write(v.Value)
		default:
			collectPlainText(v, src, sb)
		}
	}
}
