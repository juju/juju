// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package tools_test

import (
	"bytes"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/agent/tools"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/dummy"
	envtesting "launchpad.net/juju-core/environs/testing"
	"launchpad.net/juju-core/testing"
	"launchpad.net/juju-core/version"
)

type StorageSuite struct {
	env environs.Environ
	testing.LoggingSuite
	dataDir string
}

var _ = gc.Suite(&StorageSuite{})

func (s *StorageSuite) SetUpTest(c *gc.C) {
	s.LoggingSuite.SetUpTest(c)
	env, err := environs.NewFromAttrs(map[string]interface{}{
		"name":            "test",
		"type":            "dummy",
		"state-server":    false,
		"authorized-keys": "i-am-a-key",
		"ca-cert":         testing.CACert,
		"ca-private-key":  "",
	})
	c.Assert(err, gc.IsNil)
	s.env = env
	s.dataDir = c.MkDir()
}

func (s *StorageSuite) TearDownTest(c *gc.C) {
	dummy.Reset()
	s.LoggingSuite.TearDownTest(c)
}

func (s *StorageSuite) TestStorageName(c *gc.C) {
	vers := version.MustParseBinary("1.2.3-precise-amd64")
	path := tools.StorageName(vers)
	c.Assert(path, gc.Equals, "tools/juju-1.2.3-precise-amd64.tgz")
}

func (s *StorageSuite) TestSetToolPrefix(c *gc.C) {
	vers := version.MustParseBinary("1.2.3-precise-amd64")
	tools.SetToolPrefix("test_prefix/juju-")
	path := tools.StorageName(vers)
	c.Assert(path, gc.Equals, "test_prefix/juju-1.2.3-precise-amd64.tgz")
	tools.SetToolPrefix(tools.DefaultToolPrefix)
	path = tools.StorageName(vers)
	c.Assert(path, gc.Equals, "tools/juju-1.2.3-precise-amd64.tgz")
}

func (s *StorageSuite) TestReadListEmpty(c *gc.C) {
	store := s.env.Storage()
	_, err := tools.ReadList(store, 2)
	c.Assert(err, gc.Equals, tools.ErrNoTools)
}

func (s *StorageSuite) TestReadList(c *gc.C) {
	store := s.env.Storage()
	v001 := version.MustParseBinary("0.0.1-precise-amd64")
	t001 := envtesting.UploadFakeToolsVersion(c, store, v001)
	v100 := version.MustParseBinary("1.0.0-precise-amd64")
	t100 := envtesting.UploadFakeToolsVersion(c, store, v100)
	v101 := version.MustParseBinary("1.0.1-precise-amd64")
	t101 := envtesting.UploadFakeToolsVersion(c, store, v101)

	for i, t := range []struct {
		majorVersion int
		list         tools.List
	}{{
		0, tools.List{t001},
	}, {
		1, tools.List{t100, t101},
	}, {
		2, nil,
	}} {
		c.Logf("test %d", i)
		list, err := tools.ReadList(store, t.majorVersion)
		if t.list != nil {
			c.Assert(err, gc.IsNil)
			c.Assert(list, gc.DeepEquals, t.list)
		} else {
			c.Assert(err, gc.Equals, tools.ErrNoMatches)
		}
	}
}

func (s *StorageSuite) TestUpload(c *gc.C) {
	t, err := tools.Upload(s.env.Storage(), nil)
	c.Assert(err, gc.IsNil)
	c.Assert(t.Version, gc.Equals, version.Current)
	c.Assert(t.URL, gc.Not(gc.Equals), "")
	dir := downloadTools(c, t)
	out, err := exec.Command(filepath.Join(dir, "jujud"), "version").CombinedOutput()
	c.Assert(err, gc.IsNil)
	c.Assert(string(out), gc.Equals, version.Current.String()+"\n")
}

func (s *StorageSuite) TestUploadFakeSeries(c *gc.C) {
	t, err := tools.Upload(s.env.Storage(), nil, "sham", "fake")
	c.Assert(err, gc.IsNil)
	c.Assert(t.Version, gc.Equals, version.Current)
	expectRaw := downloadToolsRaw(c, t)

	list, err := tools.ReadList(s.env.Storage(), version.Current.Major)
	c.Assert(err, gc.IsNil)
	c.Assert(list, gc.HasLen, 3)
	expectSeries := []string{"fake", "sham", version.CurrentSeries()}
	sort.Strings(expectSeries)
	c.Assert(list.Series(), gc.DeepEquals, expectSeries)
	for _, t := range list {
		c.Logf("checking %s", t.URL)
		c.Assert(t.Version.Number, gc.Equals, version.CurrentNumber())
		actualRaw := downloadToolsRaw(c, t)
		c.Assert(string(actualRaw), gc.Equals, string(expectRaw))
	}
}

func (s *StorageSuite) TestUploadAndForceVersion(c *gc.C) {
	// This test actually tests three things:
	//   the writing of the FORCE-VERSION file;
	//   the reading of the FORCE-VERSION file by the version package;
	//   and the reading of the version from jujud.
	vers := version.Current
	vers.Patch++
	t, err := tools.Upload(s.env.Storage(), &vers.Number)
	c.Assert(err, gc.IsNil)
	c.Assert(t.Version, gc.Equals, vers)
}

// Test that the upload procedure fails correctly
// when the build process fails (because of a bad Go source
// file in this case).
func (s *StorageSuite) TestUploadBadBuild(c *gc.C) {
	gopath := c.MkDir()
	join := append([]string{gopath, "src"}, strings.Split("launchpad.net/juju-core/cmd/broken", "/")...)
	pkgdir := filepath.Join(join...)
	err := os.MkdirAll(pkgdir, 0777)
	c.Assert(err, gc.IsNil)

	err = ioutil.WriteFile(filepath.Join(pkgdir, "broken.go"), []byte("nope"), 0666)
	c.Assert(err, gc.IsNil)

	defer os.Setenv("GOPATH", os.Getenv("GOPATH"))
	os.Setenv("GOPATH", gopath)

	t, err := tools.Upload(s.env.Storage(), nil)
	c.Assert(t, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, `build command "go" failed: exit status 1; can't load package:(.|\n)*`)
}

// downloadTools downloads the supplied tools and extracts them into a
// new directory.
func downloadTools(c *gc.C, t *tools.Tools) string {
	resp, err := http.Get(t.URL)
	c.Assert(err, gc.IsNil)
	defer resp.Body.Close()
	cmd := exec.Command("tar", "xz")
	cmd.Dir = c.MkDir()
	cmd.Stdin = resp.Body
	out, err := cmd.CombinedOutput()
	c.Assert(err, gc.IsNil, gc.Commentf(string(out)))
	return cmd.Dir
}

// downloadToolsRaw downloads the supplied tools and returns the raw bytes.
func downloadToolsRaw(c *gc.C, t *tools.Tools) []byte {
	resp, err := http.Get(t.URL)
	c.Assert(err, gc.IsNil)
	defer resp.Body.Close()
	c.Assert(resp.StatusCode, gc.Equals, http.StatusOK)
	var buf bytes.Buffer
	_, err = io.Copy(&buf, resp.Body)
	c.Assert(err, gc.IsNil)
	return buf.Bytes()
}

var setenvTests = []struct {
	set    string
	expect []string
}{
	{"foo=1", []string{"foo=1", "arble="}},
	{"foo=", []string{"foo=", "arble="}},
	{"arble=23", []string{"foo=bar", "arble=23"}},
	{"zaphod=42", []string{"foo=bar", "arble=", "zaphod=42"}},
}

func (*StorageSuite) TestSetenv(c *gc.C) {
	env0 := []string{"foo=bar", "arble="}
	for i, t := range setenvTests {
		c.Logf("test %d", i)
		env := make([]string, len(env0))
		copy(env, env0)
		env = tools.Setenv(env, t.set)
		c.Check(env, gc.DeepEquals, t.expect)
	}
}
