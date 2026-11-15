package core

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// A directory target is replaced by building the new tree next to it and swapping folders, so
// an interrupted sync leaves the old tree or the new one, never a mix (#38):
//
//	<target>.datum-new-*   the new tree being built
//	<target>.datum-old-*   the old tree, moved aside for the swap
//
// The only window without a <target> is between the two renames. RecoverDirSwap, run at the
// start of every check and fetch, restores the old tree if a run stopped there.
const (
	swapNewInfix = ".datum-new-"
	swapOldInfix = ".datum-old-"
)

// dirSwapHook lets tests interrupt ReplaceDir after a step ("moved-old", "moved-new").
var dirSwapHook = func(step string) error { return nil }

// NewSiblingDir creates an empty folder next to dest to build its replacement in.
func NewSiblingDir(dest string) (string, error) {
	parent := filepath.Dir(dest)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return "", err
	}
	built, err := os.MkdirTemp(parent, filepath.Base(dest)+swapNewInfix)
	if err != nil {
		return "", err
	}
	// MkdirTemp makes the folder private; give it the old tree's mode, or a normal folder's.
	mode := os.FileMode(0o755)
	if info, err := os.Stat(dest); err == nil && info.IsDir() {
		mode = info.Mode().Perm()
	}
	if err := os.Chmod(built, mode); err != nil {
		_ = os.RemoveAll(built)
		return "", err
	}
	return built, nil
}

// ReplaceDir swaps the fully built tree at built (from NewSiblingDir) into dest, which may not
// exist yet, then removes the old tree.
func ReplaceDir(built, dest string) error {
	old := ""
	if _, err := os.Lstat(dest); err == nil {
		old = dest + swapOldInfix + strconv.FormatInt(time.Now().UnixNano(), 36)
		if err := os.Rename(dest, old); err != nil {
			return fmt.Errorf("move old tree aside: %w", err)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := dirSwapHook("moved-old"); err != nil {
		return err
	}
	if err := os.Rename(built, dest); err != nil {
		if old != "" {
			_ = os.Rename(old, dest)
		}
		return fmt.Errorf("move new tree into place: %w", err)
	}
	if err := dirSwapHook("moved-new"); err != nil {
		return err
	}
	if old != "" {
		if err := os.RemoveAll(old); err != nil {
			return fmt.Errorf("remove old tree: %w", err)
		}
	}
	return nil
}

// RecoverDirSwap cleans up after a ReplaceDir that didn't finish: unfinished new trees are
// removed, and if dest is missing, the old tree is moved back. It reports whether it restored
// a tree. With no leftovers, it does nothing.
func RecoverDirSwap(dest string) (bool, error) {
	if dest == "" {
		return false, nil
	}
	parent, base := filepath.Dir(dest), filepath.Base(dest)
	entries, err := os.ReadDir(parent)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	var olds []string
	for _, e := range entries {
		name := e.Name()
		switch {
		case strings.HasPrefix(name, base+swapNewInfix):
			if err := os.RemoveAll(filepath.Join(parent, name)); err != nil {
				return false, err
			}
		case strings.HasPrefix(name, base+swapOldInfix):
			olds = append(olds, filepath.Join(parent, name))
		}
	}
	if len(olds) == 0 {
		return false, nil
	}
	sort.Strings(olds)
	restored := false
	if _, err := os.Lstat(dest); errors.Is(err, os.ErrNotExist) {
		// Stopped between the two renames: the newest old tree is the one that was in place.
		newest := olds[len(olds)-1]
		if err := os.Rename(newest, dest); err != nil {
			return false, err
		}
		olds = olds[:len(olds)-1]
		restored = true
	}
	for _, old := range olds {
		if err := os.RemoveAll(old); err != nil {
			return restored, err
		}
	}
	return restored, nil
}
