package core

import (
	"context"
	"testing"

	"example.com/pinup/internal/registry"
)

type fakeFetcher struct {
	fp    string
	fetch bool
}

func (f *fakeFetcher) Name() string { return "fake" }
func (f *fakeFetcher) Fingerprint(ctx context.Context, src registry.Source) (string, error) {
	return f.fp, nil
}
func (f *fakeFetcher) Fetch(ctx context.Context, src registry.Source, dest string) error {
	f.fetch = true
	return nil
}

func TestFirstNonEmpty(t *testing.T) {
	if got := firstNonEmpty("", "b"); got != "b" {
		t.Fatalf("want b got %s", got)
	}
}
