package registry

import "context"

type Source struct {
    Type string `yaml:"type"`
    URL  string `yaml:"url,omitempty"`
    Path string `yaml:"path,omitempty"`
    Ref  string `yaml:"ref,omitempty"`

    FingerprintCmd string `yaml:"fingerprint_cmd,omitempty"`
    FetchCmd       string `yaml:"fetch_cmd,omitempty"`
}

type Fetcher interface {
    Name() string
    Fingerprint(ctx context.Context, src Source) (string, error)
    Fetch(ctx context.Context, src Source, dest string) error
}

var fetchers = map[string]Fetcher{}

func Register(f Fetcher) { fetchers[f.Name()] = f }
func Get(kind string) (Fetcher, bool) { f, ok := fetchers[kind]; return f, ok }
