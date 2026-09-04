// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups_test

import (
	"context"
	"io"
	"os"
	"path/filepath"
	stdtesting "testing"

	"github.com/juju/tc"

	"github.com/juju/juju/core/backups"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/testing"
)

type dumpSuite struct {
	testing.BaseSuite
}

func TestDumpSuite(t *stdtesting.T) {
	tc.Run(t, &dumpSuite{})
}

// TestStageDumps verifies that staged dumps land on disk as readable files
// whose contents match what the export functions streamed, that the staging
// handle reports their total size and feeds them to Create as dump entries,
// and that Close removes the staging directory.
func (s *dumpSuite) TestStageDumps(c *tc.C) {
	dir := c.MkDir()
	dumps := []backups.NamedDump{{
		Name:   "controller.yaml",
		Export: backups.YAMLDump(map[string]string{"controller": "data"}),
	}, {
		Name:   "models/model.yaml",
		Export: backups.YAMLDump(map[string]string{"model": "data"}),
	}}

	staging, err := backups.StageDumps(c.Context(), dir, dumps)
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(staging.Entries(), tc.HasLen, 2)
	c.Check(staging.Entries()[0].Name, tc.Equals, "controller.yaml")
	c.Check(staging.Entries()[1].Name, tc.Equals, "models/model.yaml")

	contents, err := io.ReadAll(staging.Entries()[0].Reader)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(string(contents), tc.Equals, "controller: data\n")
	c.Check(staging.Size() > 0, tc.IsTrue, tc.Commentf("size must be positive"))

	staging.Close()
	entries, err := os.ReadDir(dir)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(entries, tc.HasLen, 0)
}

// TestStageDumpsExportError verifies that an export failure aborts staging
// and leaves no partial dumps behind.
func (s *dumpSuite) TestStageDumpsExportError(c *tc.C) {
	dir := c.MkDir()
	dumps := []backups.NamedDump{{
		Name:   "controller.yaml",
		Export: backups.YAMLDump(map[string]string{"controller": "data"}),
	}, {
		Name: "broken.yaml",
		Export: func(context.Context, io.Writer) error {
			return errors.New("export failed")
		},
	}}

	_, err := backups.StageDumps(c.Context(), dir, dumps)
	c.Assert(err, tc.ErrorMatches, "export failed")

	entries, err := os.ReadDir(dir)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(entries, tc.HasLen, 0)
}

// TestWalkDumps verifies that WalkDumps visits each staged file with its
// archive-relative name and contents.
func (s *dumpSuite) TestWalkDumps(c *tc.C) {
	dir := c.MkDir()
	c.Assert(os.WriteFile(filepath.Join(dir, "controller.yaml"), []byte("controller: data\n"), 0644), tc.ErrorIsNil)
	c.Assert(os.MkdirAll(filepath.Join(dir, "models"), 0755), tc.ErrorIsNil)
	c.Assert(os.WriteFile(filepath.Join(dir, "models", "model.yaml"), []byte("model: data\n"), 0644), tc.ErrorIsNil)

	visited := map[string]string{}
	err := backups.WalkDumps(c.Context(), dir, func(name string, reader io.Reader) error {
		contents, err := io.ReadAll(reader)
		if err != nil {
			return err
		}
		visited[name] = string(contents)
		return nil
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(visited, tc.DeepEquals, map[string]string{
		"controller.yaml":   "controller: data\n",
		"models/model.yaml": "model: data\n",
	})
}
