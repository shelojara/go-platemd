package platemd_test

import (
	"context"
	"reflect"
	"testing"
	"time"

	platemd "github.com/shelojara/go-platemd-wasm"
)

func TestConverter_DefaultOptions(t *testing.T) {
	c, err := platemd.New(platemd.WithDefaultOptions(platemd.Options{
		Disable: []string{"lists"},
	}))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// With lists disabled by default, deserialize should produce the
	// older ul/li/lic shape (the list plugin isn't registered).
	v, err := c.MarkdownToPlate(ctx, "- one\n- two\n", nil)
	if err != nil {
		t.Fatalf("MarkdownToPlate: %v", err)
	}
	if got, want := v[0]["type"], "ul"; got != want {
		t.Errorf("with default disable=[lists], top-level type = %v, want %v", got, want)
	}
}

func TestConverter_PerCallOverridesDefaults(t *testing.T) {
	c, err := platemd.New(platemd.WithDefaultOptions(platemd.Options{
		Disable: []string{"lists"},
	}))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Per-call Disable replaces the default — pass an empty slice to
	// explicitly disable nothing.
	v, err := c.MarkdownToPlate(ctx, "- one\n- two\n", &platemd.Options{
		Disable: []string{},
	})
	if err != nil {
		t.Fatalf("MarkdownToPlate: %v", err)
	}
	// With the list plugin enabled, the markdown plugin emits the
	// indent-based list shape (type=p, listStyleType="disc").
	if v[0]["type"] != "p" {
		t.Errorf("with per-call override, top-level type = %v, want p", v[0]["type"])
	}
	if v[0]["listStyleType"] == nil {
		t.Errorf("with per-call override, expected listStyleType to be set: %+v", v[0])
	}
}

func TestWorker_InheritsDefaults(t *testing.T) {
	c, err := platemd.New(platemd.WithDefaultOptions(platemd.Options{
		Disable: []string{"lists"},
	}))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	w, err := c.NewWorker(ctx)
	if err != nil {
		t.Fatalf("NewWorker: %v", err)
	}
	t.Cleanup(func() { _ = w.Close() })

	// Defaults from the Converter should apply via the JS-side
	// set_defaults op.
	v, err := w.MarkdownToPlate(ctx, "- one\n- two\n", nil)
	if err != nil {
		t.Fatalf("MarkdownToPlate: %v", err)
	}
	if v[0]["type"] != "ul" {
		t.Errorf("worker should inherit Disable=[lists] default; got top-level type = %v", v[0]["type"])
	}

	// Override per-call: list plugin re-enabled for this call.
	v, err = w.MarkdownToPlate(ctx, "- one\n- two\n", &platemd.Options{
		Disable: []string{},
	})
	if err != nil {
		t.Fatalf("MarkdownToPlate (override): %v", err)
	}
	if v[0]["type"] != "p" {
		t.Errorf("per-call override should re-enable lists; got top-level type = %v", v[0]["type"])
	}

	// Back to a default-only call: cached editor should still reflect defaults.
	v, err = w.MarkdownToPlate(ctx, "- one\n- two\n", nil)
	if err != nil {
		t.Fatalf("MarkdownToPlate (back to default): %v", err)
	}
	if v[0]["type"] != "ul" {
		t.Errorf("after per-call override, default should be restored; got %v", v[0]["type"])
	}
}

func TestConverter_DefaultsAccessor(t *testing.T) {
	want := platemd.Options{Disable: []string{"tables"}}
	c, err := platemd.New(platemd.WithDefaultOptions(want))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer c.Close()
	got := c.Defaults()
	if got == nil {
		t.Fatal("Defaults() returned nil")
	}
	if !reflect.DeepEqual(got.Disable, want.Disable) {
		t.Errorf("Defaults().Disable = %v, want %v", got.Disable, want.Disable)
	}
	// Confirm it's a copy: mutating the returned value doesn't affect
	// the Converter.
	got.Disable = append(got.Disable, "media")
	again := c.Defaults()
	if reflect.DeepEqual(again.Disable, got.Disable) {
		t.Errorf("Defaults() returned shared slice; mutation leaked: %v", again.Disable)
	}
}

func TestNew_NoDefaults(t *testing.T) {
	c, err := platemd.New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer c.Close()
	if c.Defaults() != nil {
		t.Errorf("Defaults() = %+v, want nil", c.Defaults())
	}
}
