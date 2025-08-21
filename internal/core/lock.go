package core

import (
    "os"
    "time"

    "gopkg.in/yaml.v3"
)

type Lock struct {
    Version     int                 `yaml:"version"`
    LastChecked *time.Time          `yaml:"last_checked,omitempty"`
    Items       map[string]*LockItem `yaml:"items"`
}

type LockItem struct {
    LocalSHA256       string     `yaml:"local_sha256,omitempty"`
    RemoteFingerprint string     `yaml:"remote_fingerprint,omitempty"`
    CheckedAt         *time.Time `yaml:"checked_at,omitempty"`
}

func readLock(path string) (*Lock, error) {
    b, err := os.ReadFile(path)
    if err != nil {
        return &Lock{Version: 1, Items: map[string]*LockItem{}}, nil
    }
    var l Lock
    if err := yaml.Unmarshal(b, &l); err != nil {
        return nil, err
    }
    if l.Items == nil {
        l.Items = map[string]*LockItem{}
    }
    return &l, nil
}

func writeLock(path string, l *Lock) error {
    b, err := yaml.Marshal(l)
    if err != nil {
        return err
    }
    tmp := path + ".tmp"
    if err := os.WriteFile(tmp, b, 0o644); err != nil {
        return err
    }
    return os.Rename(tmp, path)
}
