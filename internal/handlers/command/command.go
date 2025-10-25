package command

import (
	"context"
	"errors"
	"strings"

	"example.com/datum/internal/registry"
	runrt "example.com/datum/internal/runtime"
)

type handler struct{}

func New() *handler             { return &handler{} }
func (h *handler) Name() string { return "command" }

func (h *handler) Fingerprint(ctx context.Context, src registry.Source) (string, error) {
	if strings.TrimSpace(src.FingerprintCmd) == "" {
		return "", errors.New("command: missing fingerprint_cmd")
	}
	cmd := substitute(src.FingerprintCmd, src, "")
	out, err := runrt.RunShell(ctx, cmd, nil)
	return strings.TrimSpace(out), err
}

func (h *handler) Fetch(ctx context.Context, src registry.Source, dest string) error {
	if strings.TrimSpace(src.FetchCmd) == "" {
		return errors.New("command: missing fetch_cmd")
	}
	env := []string{"DEST=" + dest}
	cmd := substitute(src.FetchCmd, src, dest)
	_, err := runrt.RunShell(ctx, cmd, env)
	return err
}

func substitute(tmpl string, src registry.Source, dest string) string {
	r := strings.NewReplacer(
		"{{url}}", src.URL,
		"{{path}}", src.Path,
		"{{ref}}", src.Ref,
		"{{dest}}", dest,
	)
	return r.Replace(tmpl)
}

func init() {
	registry.Register(New())
}
