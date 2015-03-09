// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/names"
	"github.com/juju/utils"
	"gopkg.in/juju/charm.v4/hooks"

	"github.com/juju/juju/worker/uniter/hook"
)

// state describes the state of a storage attachment.
type state struct {
	// storage is the tag of the storage attachment.
	storage names.StorageTag

	// attached records the uniter's knowledge of the
	// storage attachment state.
	attached bool
}

// ValidateHook returns an error if the supplied hook.Info does not represent
// a valid change to the storage state. Hooks must always be validated
// against the current state before they are run, to ensure that the system
// meets its guarantees about hook execution order.
func (s *state) ValidateHook(hi hook.Info) (err error) {
	defer errors.DeferredAnnotatef(&err, "inappropriate %q hook for storage %q", hi.Kind, s.storage.Id())
	if hi.StorageId != s.storage.Id() {
		return fmt.Errorf("expected storage %q, got storage %q", s.storage.Id(), hi.StorageId)
	}
	switch hi.Kind {
	case hooks.StorageAttached:
		if s.attached {
			return errors.New("storage already attached")
		}
	case hooks.StorageDetached: // TODO(axw) this should be "detaching"
		if !s.attached {
			return errors.New("storage not attached")
		}
	}
	return nil
}

// stateFile is a filesystem-backed representation of the state of a
// storage attachment. Concurrent modifications to the underlying state
// file will have undefined consequences.
type stateFile struct {
	// path identifies the directory holding persistent state.
	path string

	// state is the cached state of the directory, which is guaranteed
	// to be synchronized with the true state so long as no concurrent
	// changes are made to the directory.
	state
}

// readStateFile loads a stateFile from the subdirectory of dirPath named
// for the supplied storage tag. If the directory does not exist, no error
// is returned,
func readStateFile(dirPath string, tag names.StorageTag) (d *stateFile, err error) {
	filename := strings.Replace(tag.Id(), "/", "-", -1)
	d = &stateFile{
		filepath.Join(dirPath, filename),
		state{storage: tag},
	}
	defer errors.DeferredAnnotatef(&err, "cannot load storage %q state from %q", tag.Id(), d.path)
	if _, err := os.Stat(d.path); os.IsNotExist(err) {
		return d, nil
	} else if err != nil {
		return nil, err
	}
	var info diskInfo
	if err := utils.ReadYaml(d.path, &info); err != nil {
		return nil, errors.Errorf("invalid storage state file %q: %v", d.path, err)
	}
	if info.Attached == nil {
		return nil, errors.Errorf("invalid storage state file %q: missing 'attached'", d.path)
	}
	d.state.attached = *info.Attached
	return d, nil
}

// readAllStateFiles loads and returns every stateFile persisted inside
// the supplied dirPath. If dirPath does not exist, no error is returned.
func readAllStateFiles(dirPath string) (files map[names.StorageTag]*stateFile, err error) {
	defer errors.DeferredAnnotatef(&err, "cannot load storage state from %q", dirPath)
	if _, err := os.Stat(dirPath); os.IsNotExist(err) {
		return nil, nil
	} else if err != nil {
		return nil, err
	}
	fis, err := ioutil.ReadDir(dirPath)
	if err != nil {
		return nil, err
	}
	files = make(map[names.StorageTag]*stateFile)
	for _, fi := range fis {
		if fi.IsDir() {
			continue
		}
		storageId := strings.Replace(fi.Name(), "-", "/", -1)
		if !names.IsValidStorage(storageId) {
			continue
		}
		tag := names.NewStorageTag(storageId)
		f, err := readStateFile(dirPath, tag)
		if err != nil {
			return nil, err
		}
		files[tag] = f
	}
	return files, nil
}

// CommitHook atomically writes to disk the storage state change in hi.
// It must be called after the respective hook was executed successfully.
// CommitHook doesn't validate hi but guarantees that successive writes
// of the same hi are idempotent.
func (d *stateFile) CommitHook(hi hook.Info) (err error) {
	defer errors.DeferredAnnotatef(&err, "failed to write %q hook info for %q on state directory", hi.Kind, hi.StorageId)
	if hi.Kind == hooks.StorageDetached { // TODO(axw) should be detaching
		return d.Remove()
	}
	attached := true
	di := diskInfo{&attached}
	if err := utils.WriteYaml(d.path, &di); err != nil {
		return err
	}
	// If write was successful, update own state.
	d.state.attached = true
	return nil
}

// Remove removes the directory if it exists and is empty.
func (d *stateFile) Remove() error {
	if err := os.Remove(d.path); err != nil && !os.IsNotExist(err) {
		return err
	}
	// If atomic delete succeeded, update own state.
	d.state.attached = false
	return nil
}

// diskInfo defines the storage attachment data serialization.
type diskInfo struct {
	Attached *bool `yaml:"attached,omitempty"`
}
