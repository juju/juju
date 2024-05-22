// Copyright 2011, 2012, 2013 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package charm_test

import (
	"bytes"
	"io"
	"os"
	"path/filepath"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/yaml.v2"

	"github.com/juju/juju/internal/charm"
	"github.com/juju/juju/internal/fs"
)

type CharmSuite struct {
	testing.CleanupSuite
}

var _ = gc.Suite(&CharmSuite{})

func (s *CharmSuite) TestReadCharm(c *gc.C) {
	ch, err := charm.ReadCharm(charmDirPath(c, "dummy"))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ch.Meta().Name, gc.Equals, "dummy")

	bPath := archivePath(c, readCharmDir(c, "dummy"))
	ch, err = charm.ReadCharm(bPath)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ch.Meta().Name, gc.Equals, "dummy")
}

func (s *CharmSuite) TestReadCharmDirEmptyError(c *gc.C) {
	ch, err := charm.ReadCharm(c.MkDir())
	c.Assert(err, gc.NotNil)
	c.Assert(ch, gc.Equals, nil)
}

func (s *CharmSuite) TestReadCharmArchiveError(c *gc.C) {
	path := filepath.Join(c.MkDir(), "path")
	err := os.WriteFile(path, []byte("foo"), 0644)
	c.Assert(err, jc.ErrorIsNil)
	ch, err := charm.ReadCharm(path)
	c.Assert(err, gc.NotNil)
	c.Assert(ch, gc.Equals, nil)
}

type FormatSuite struct {
	testing.CleanupSuite
}

var _ = gc.Suite(&FormatSuite{})

func (FormatSuite) TestFormatV2Containers(c *gc.C) {
	ch, err := charm.ReadCharm(charmDirPath(c, "format-containers"))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ch, gc.NotNil)

	err = charm.CheckMeta(ch)
	c.Assert(err, jc.ErrorIsNil)

	f := charm.MetaFormat(ch)
	c.Assert(f, gc.Equals, charm.FormatV2)
}

func (FormatSuite) TestFormatV2ContainersManifest(c *gc.C) {
	ch, err := charm.ReadCharm(charmDirPath(c, "format-containersmanifest"))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ch, gc.NotNil)

	err = charm.CheckMeta(ch)
	c.Assert(err, jc.ErrorIsNil)

	f := charm.MetaFormat(ch)
	c.Assert(f, gc.Equals, charm.FormatV2)
}

func checkDummy(c *gc.C, f charm.Charm, path string) {
	c.Assert(f.Revision(), gc.Equals, 1)
	c.Assert(f.Meta().Name, gc.Equals, "dummy")
	c.Assert(f.Config().Options["title"].Default, gc.Equals, "My Title")
	c.Assert(f.Actions(), jc.DeepEquals,
		&charm.Actions{
			ActionSpecs: map[string]charm.ActionSpec{
				"snapshot": {
					Description: "Take a snapshot of the database.",
					Params: map[string]interface{}{
						"type":        "object",
						"description": "Take a snapshot of the database.",
						"title":       "snapshot",
						"properties": map[string]interface{}{
							"outfile": map[string]interface{}{
								"description": "The file to write out to.",
								"type":        "string",
								"default":     "foo.bz2",
							}},
						"additionalProperties": false}}}})
	lpc, ok := f.(charm.LXDProfiler)
	c.Assert(ok, jc.IsTrue)
	c.Assert(lpc.LXDProfile(), jc.DeepEquals, &charm.LXDProfile{
		Config: map[string]string{
			"security.nesting":    "true",
			"security.privileged": "true",
		},
		Description: "sample lxdprofile for testing",
		Devices: map[string]map[string]string{
			"tun": {
				"path": "/dev/net/tun",
				"type": "unix-char",
			},
		},
	})
	switch f := f.(type) {
	case *charm.CharmArchive:
		c.Assert(f.Path, gc.Equals, path)
	case *charm.CharmDir:
		c.Assert(f.Path, gc.Equals, path)
	}
}

type YamlHacker map[interface{}]interface{}

func ReadYaml(r io.Reader) YamlHacker {
	data, err := io.ReadAll(r)
	if err != nil {
		panic(err)
	}
	m := make(map[interface{}]interface{})
	err = yaml.Unmarshal(data, m)
	if err != nil {
		panic(err)
	}
	return YamlHacker(m)
}

func (yh YamlHacker) Reader() io.Reader {
	data, err := yaml.Marshal(yh)
	if err != nil {
		panic(err)
	}
	return bytes.NewBuffer(data)
}

// charmDirPath returns the path to the charm with the
// given name in the testing repository.
func charmDirPath(c *gc.C, name string) string {
	path := filepath.Join("internal/test-charm-repo/quantal", name)
	assertIsDir(c, path)
	return path
}

// bundleDirPath returns the path to the bundle with the
// given name in the testing repository.
func bundleDirPath(c *gc.C, name string) string {
	path := filepath.Join("internal/test-charm-repo/bundle", name)
	assertIsDir(c, path)
	return path
}

func assertIsDir(c *gc.C, path string) {
	info, err := os.Stat(path)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(info.IsDir(), gc.Equals, true)
}

// readCharmDir returns the charm with the given
// name from the testing repository.
func readCharmDir(c *gc.C, name string) *charm.CharmDir {
	path := charmDirPath(c, name)
	ch, err := charm.ReadCharmDir(path)
	c.Assert(err, jc.ErrorIsNil)
	return ch
}

// readBundleDir returns the bundle with the
// given name from the testing repository.
func readBundleDir(c *gc.C, name string) *charm.BundleDir {
	path := bundleDirPath(c, name)
	ch, err := charm.ReadBundleDir(path)
	c.Assert(err, jc.ErrorIsNil)
	return ch
}

type ArchiverTo interface {
	ArchiveTo(w io.Writer) error
}

// archivePath archives the given charm or bundle
// to a newly created file and returns the path to the
// file.
func archivePath(c *gc.C, a ArchiverTo) string {
	dir := c.MkDir()
	path := filepath.Join(dir, "archive")
	file, err := os.Create(path)
	c.Assert(err, jc.ErrorIsNil)
	defer file.Close()
	err = a.ArchiveTo(file)
	c.Assert(err, jc.ErrorIsNil)
	return path
}

// cloneDir recursively copies the path directory
// into a new directory and returns the path
// to it.
func cloneDir(c *gc.C, path string) string {
	newPath := filepath.Join(c.MkDir(), filepath.Base(path))
	err := fs.Copy(path, newPath)
	c.Assert(err, jc.ErrorIsNil)
	return newPath
}
