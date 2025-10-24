package file

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"

	"example.com/datum/internal/core"
	"example.com/datum/internal/registry"
)

type handler struct{}

func New() *handler             { return &handler{} }
func (h *handler) Name() string { return "file" }

func (h *handler) Fingerprint(ctx context.Context, src registry.Source) (string, error) {
	if src.Path == "" {
		return "", errors.New("file: missing source.path")
	}
	hh, err := core.HashFile(src.Path) // use exported HashFile function
	if err != nil {
		return "", err
	}
	return "sha256:" + hh, nil
}

func (h *handler) Fetch(ctx context.Context, src registry.Source, dest string) error {
	if src.Path == "" {
		return errors.New("file: missing source.path")
	}
	in, err := os.Open(src.Path)
	if err != nil {
		return err
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	tmp := dest + ".tmp"
	out, err := os.Create(tmp)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		_ = os.Remove(tmp)
		return err
	}
	if err := out.Close(); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, dest)
}

func init() {
	registry.Register(New())
}
