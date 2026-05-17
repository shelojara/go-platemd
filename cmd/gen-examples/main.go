// gen-examples writes paired markdown / JSON fixtures into examples/.
// Run with `go run ./cmd/gen-examples` from the repo root.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	platemd "github.com/shelojara/go-platemd-wasm"
)

type example struct {
	name string
	md   string
}

func main() {
	examples := []example{
		{
			name: "01-formatting",
			md: "# Welcome\n\n" +
				"This paragraph has **bold**, *italic*, and `inline code` text.\n",
		},
		{
			name: "02-lists",
			md: "## Shopping list\n\n" +
				"- apples\n" +
				"- bread\n" +
				"- coffee\n\n" +
				"## Steps\n\n" +
				"1. Wake up\n" +
				"2. Make coffee\n" +
				"3. Ship it\n",
		},
		{
			name: "03-link-quote-code",
			md: "Read the [Plate docs](https://platejs.org/) for more.\n\n" +
				"> Markdown is just plain text with conventions.\n\n" +
				"```go\n" +
				"package main\n\n" +
				"func main() {\n" +
				"\tfmt.Println(\"hi\")\n" +
				"}\n" +
				"```\n",
		},
		{
			name: "04-image-and-rule",
			md: "Here is a logo:\n\n" +
				"![Go gopher](https://go.dev/images/gophers/ladder.svg)\n\n" +
				"---\n\n" +
				"And a paragraph after the rule.\n",
		},
		{
			name: "05-table",
			md: "| Lang | Year |\n" +
				"| ---- | ---- |\n" +
				"| Go   | 2009 |\n" +
				"| Rust | 2010 |\n",
		},
	}

	c, err := platemd.New()
	if err != nil {
		die("New: %v", err)
	}
	defer c.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	w, err := c.NewWorker(ctx)
	if err != nil {
		die("NewWorker: %v", err)
	}
	defer w.Close()

	outDir := "examples"
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		die("mkdir %s: %v", outDir, err)
	}

	for _, ex := range examples {
		value, err := w.MarkdownToPlate(ctx, ex.md, nil)
		if err != nil {
			die("%s: md->plate: %v", ex.name, err)
		}
		pretty, err := json.MarshalIndent(value, "", "  ")
		if err != nil {
			die("%s: marshal: %v", ex.name, err)
		}
		mdPath := filepath.Join(outDir, ex.name+".md")
		jsonPath := filepath.Join(outDir, ex.name+".json")
		if err := os.WriteFile(mdPath, []byte(ex.md), 0o644); err != nil {
			die("write %s: %v", mdPath, err)
		}
		if err := os.WriteFile(jsonPath, append(pretty, '\n'), 0o644); err != nil {
			die("write %s: %v", jsonPath, err)
		}
		fmt.Printf("wrote %s (%d nodes) and %s\n", mdPath, len(value), jsonPath)
	}
}

func die(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
