// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage_test

import (
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/juju/errors"
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v4/hooks"

	"github.com/juju/juju/testing"
	"github.com/juju/juju/worker/uniter/hook"
	"github.com/juju/juju/worker/uniter/storage"
)

type stateSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&stateSuite{})

func writeFile(c *gc.C, path string, content string) {
	err := ioutil.WriteFile(path, []byte(content), 0644)
	c.Assert(err, jc.ErrorIsNil)
}

func assertFileNotExist(c *gc.C, path string) {
	_, err := os.Stat(path)
	c.Assert(err, jc.Satisfies, os.IsNotExist)
}

func (s *stateSuite) TestReadAllStateFiles(c *gc.C) {
	dir := c.MkDir()
	writeFile(c, filepath.Join(dir, "data-0"), "attached: true")
	// We don't currently ever write a file with attached=false,
	// but test it for coverage in case of required changes.
	writeFile(c, filepath.Join(dir, "data-1"), "attached: false")

	states, err := storage.ReadAllStateFiles(dir)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(states, gc.HasLen, 2)

	state, ok := states[names.NewStorageTag("data/0")]
	c.Assert(ok, jc.IsTrue)
	c.Assert(storage.StateAttached(state), jc.IsTrue)

	state, ok = states[names.NewStorageTag("data/1")]
	c.Assert(ok, jc.IsTrue)
	c.Assert(storage.StateAttached(state), jc.IsFalse)
}

func (s *stateSuite) TestReadAllStateFilesJunk(c *gc.C) {
	dir := c.MkDir()
	writeFile(c, filepath.Join(dir, "data-0"), "attached: true")
	// data-extra-1 is not a valid storage ID, so it will
	// be ignored by ReadAllStateFiles.
	writeFile(c, filepath.Join(dir, "data-extra-1"), "attached: false")
	// subdirs are ignored.
	err := os.Mkdir(filepath.Join(dir, "data-1"), 0755)
	c.Assert(err, jc.ErrorIsNil)

	states, err := storage.ReadAllStateFiles(dir)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(states, gc.HasLen, 1)
	_, ok := states[names.NewStorageTag("data/0")]
	c.Assert(ok, jc.IsTrue)
}

func (s *stateSuite) TestReadAllStateFilesOneBadApple(c *gc.C) {
	dir := c.MkDir()
	writeFile(c, filepath.Join(dir, "data-0"), "rubbish")
	_, err := storage.ReadAllStateFiles(dir)
	c.Assert(err, gc.ErrorMatches, `cannot load storage state from ".*": cannot load storage "data/0" state from ".*": invalid storage state file ".*": missing 'attached'`)
}

func (s *stateSuite) TestReadAllStateFilesDirNotExist(c *gc.C) {
	dir := filepath.Join(c.MkDir(), "doesnotexist")
	states, err := storage.ReadAllStateFiles(dir)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(states, gc.HasLen, 0)
}

func (s *stateSuite) TestReadAllStateFilesDirEmpty(c *gc.C) {
	dir := c.MkDir()
	states, err := storage.ReadAllStateFiles(dir)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(states, gc.HasLen, 0)
}

func (s *stateSuite) TestReadStateFileFileNotExist(c *gc.C) {
	dir := c.MkDir()
	state, err := storage.ReadStateFile(dir, names.NewStorageTag("data/0"))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(state, gc.NotNil)

	data, err := ioutil.ReadFile(filepath.Join(dir, "data-0"))
	c.Assert(err, jc.Satisfies, os.IsNotExist)

	err = state.CommitHook(hook.Info{
		Kind:      hooks.StorageAttached,
		StorageId: "data-0",
	})
	c.Assert(err, jc.ErrorIsNil)

	data, err = ioutil.ReadFile(filepath.Join(dir, "data-0"))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(string(data), gc.Equals, "attached: true\n")
}

func (s *stateSuite) TestReadStateFileDirNotExist(c *gc.C) {
	dir := filepath.Join(c.MkDir(), "doesnotexist")
	state, err := storage.ReadStateFile(dir, names.NewStorageTag("data/0"))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(state, gc.NotNil)

	// CommitHook will fail if the directory does not exist. The uniter
	// must ensure the directory is created before committing any hooks
	// to the storage state.
	err = state.CommitHook(hook.Info{
		Kind:      hooks.StorageAttached,
		StorageId: "data-0",
	})
	c.Assert(errors.Cause(err), jc.Satisfies, os.IsNotExist)
}

func (s *stateSuite) TestReadStateFileBadFormat(c *gc.C) {
	dir := c.MkDir()
	writeFile(c, filepath.Join(dir, "data-0"), "!@#")
	_, err := storage.ReadStateFile(dir, names.NewStorageTag("data/0"))
	c.Assert(err, gc.ErrorMatches, `cannot load storage "data/0" state from ".*": invalid storage state file ".*": YAML error: did not find expected whitespace or line break`)

	writeFile(c, filepath.Join(dir, "data-0"), "icantbelieveitsnotattached: true\n")
	_, err = storage.ReadStateFile(dir, names.NewStorageTag("data/0"))
	c.Assert(err, gc.ErrorMatches, `cannot load storage "data/0" state from ".*": invalid storage state file ".*": missing 'attached'`)
}

func (s *stateSuite) TestCommitHook(c *gc.C) {
	dir := c.MkDir()
	state, err := storage.ReadStateFile(dir, names.NewStorageTag("data/0"))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(state, gc.NotNil)
	stateFile := filepath.Join(dir, "data-0")

	// CommitHook must be idempotent, so test each operation
	// twice in a row.

	for i := 0; i < 2; i++ {
		err := state.CommitHook(hook.Info{
			Kind:      hooks.StorageAttached,
			StorageId: "data-0",
		})
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(stateFile, jc.IsNonEmptyFile)
	}

	for i := 0; i < 2; i++ {
		err := state.CommitHook(hook.Info{
			Kind:      hooks.StorageDetached,
			StorageId: "data-0",
		})
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(stateFile, jc.DoesNotExist)
	}
}

func (s *stateSuite) TestValidateHook(c *gc.C) {
	const unattached = false
	const attached = true

	err := storage.ValidateHook(
		names.NewStorageTag("data/0"), unattached,
		hook.Info{Kind: hooks.StorageAttached, StorageId: "data/1"},
	)
	c.Assert(err, gc.ErrorMatches, `inappropriate "storage-attached" hook for storage "data/0": expected storage "data/0", got storage "data/1"`)

	validate := func(attached bool, kind hooks.Kind) error {
		return storage.ValidateHook(
			names.NewStorageTag("data/0"), attached,
			hook.Info{Kind: kind, StorageId: "data/0"},
		)
	}
	assertValidates := func(attached bool, kind hooks.Kind) {
		err := validate(attached, kind)
		c.Assert(err, jc.ErrorIsNil)
	}
	assertValidateFails := func(attached bool, kind hooks.Kind, expect string) {
		err := validate(attached, kind)
		c.Assert(err, gc.ErrorMatches, expect)
	}

	assertValidates(false, hooks.StorageAttached)
	assertValidates(true, hooks.StorageDetached)
	assertValidateFails(false, hooks.StorageDetached, `inappropriate "storage-detached" hook for storage "data/0": storage not attached`)
	assertValidateFails(true, hooks.StorageAttached, `inappropriate "storage-attached" hook for storage "data/0": storage already attached`)
}
