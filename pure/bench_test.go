package pure_test

import (
	"context"
	"testing"

	"github.com/shelojara/go-platemd-wasm/pure"
)

const smallMd = `# Title

This paragraph has **bold**, *italic*, and ` + "`inline code`" + ` text.

- one
- two
- three
`

func BenchmarkPure_MdToPlate_Small(b *testing.B) {
	c, _ := pure.New()
	defer c.Close()
	ctx := context.Background()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := c.MarkdownToPlate(ctx, smallMd, nil); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkPure_PlateToMd_Small(b *testing.B) {
	c, _ := pure.New()
	defer c.Close()
	ctx := context.Background()
	value, err := c.MarkdownToPlate(ctx, smallMd, nil)
	if err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := c.PlateToMarkdown(ctx, value, nil); err != nil {
			b.Fatal(err)
		}
	}
}
