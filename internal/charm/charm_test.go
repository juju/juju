// Copyright 2011, 2012, 2013 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package charm_test

import (
	"bytes"
	"io"
	"os"
	"path/filepath"

	"github.com/juju/tc"
	jc "github.com/juju/testing/checkers"
	"gopkg.in/yaml.v2"

	"github.com/juju/juju/internal/charm"
	charmtesting "github.com/juju/juju/internal/charm/testing"
	"github.com/juju/juju/internal/fs"
)

func checkDummy(c *tc.C, f charm.Charm) {
	c.Assert(f.Revision(), tc.Equals, 1)
	c.Assert(f.Meta().Name, tc.Equals, "dummy")
	c.Assert(f.Config().Options["title"].Default, tc.Equals, "My Title")
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
func charmDirPath(c *tc.C, name string) string {
	path := filepath.Join("internal/test-charm-repo/quantal", name)
	assertIsDir(c, path)
	return path
}

// bundleDirPath returns the path to the bundle with the
// given name in the testing repository.
func bundleDirPath(c *tc.C, name string) string {
	path := filepath.Join("internal/test-charm-repo/bundle", name)
	assertIsDir(c, path)
	return path
}

func assertIsDir(c *tc.C, path string) {
	info, err := os.Stat(path)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(info.IsDir(), tc.Equals, true)
}

// readCharmDir returns the charm with the given
// name from the testing repository.
func readCharmDir(c *tc.C, name string) *charmtesting.CharmDir {
	path := charmDirPath(c, name)
	ch, err := charmtesting.ReadCharmDir(path)
	c.Assert(err, jc.ErrorIsNil)
	return ch
}

// readBundleDir returns the bundle with the
// given name from the testing repository.
func readBundleDir(c *tc.C, name string) *charmtesting.BundleDir {
	path := bundleDirPath(c, name)
	ch, err := charmtesting.ReadBundleDir(path)
	c.Assert(err, jc.ErrorIsNil)
	return ch
}

type ArchiverTo interface {
	ArchiveTo(w io.Writer) error
}

// archivePath archives the given charm or bundle
// to a newly created file and returns the path to the
// file.
func archivePath(c *tc.C, a ArchiverTo) string {
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
func cloneDir(c *tc.C, path string) string {
	newPath := filepath.Join(c.MkDir(), filepath.Base(path))
	err := fs.Copy(path, newPath)
	c.Assert(err, jc.ErrorIsNil)
	return newPath
}
