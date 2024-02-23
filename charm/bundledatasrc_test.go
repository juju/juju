// Copyright 2019 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package charm

import (
	"archive/zip"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
)

type BundleDataSourceSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&BundleDataSourceSuite{})

func (s *BundleDataSourceSuite) TestReadBundleFromLocalFile(c *gc.C) {
	path := bundleDirPath(c, "wordpress-multidoc")
	src, err := LocalBundleDataSource(filepath.Join(path, "bundle.yaml"))
	c.Assert(err, gc.IsNil)
	assertBundleSourceProcessed(c, src)
}

func (s *BundleDataSourceSuite) TestReadBundleFromExplodedArchiveFolder(c *gc.C) {
	path := bundleDirPath(c, "wordpress-multidoc")
	src, err := LocalBundleDataSource(path)
	c.Assert(err, gc.IsNil)
	assertBundleSourceProcessed(c, src)
}

func (s *BundleDataSourceSuite) TestReadBundleFromArchive(c *gc.C) {
	path := archiveBundleDirPath(c, "wordpress-multidoc")
	src, err := LocalBundleDataSource(path)
	c.Assert(err, gc.IsNil)
	assertBundleSourceProcessed(c, src)
}

func (s *BundleDataSourceSuite) TestReadBundleFromStream(c *gc.C) {
	r := strings.NewReader(`
applications:
  wordpress:
    charm: wordpress
  mysql:
    charm: mysql
    num_units: 1
relations:
  - ["wordpress:db", "mysql:server"]
--- # overlay.yaml
applications:
  wordpress:
    offers:
      offer1:
        endpoints:
          - "some-endpoint"
--- # overlay2.yaml
applications:
  wordpress:
    offers:
      offer1:
        acl:
          admin: "admin"
          foo: "consume"
`)

	src, err := StreamBundleDataSource(r, "https://example.com")
	c.Assert(err, gc.IsNil)
	assertBundleSourceProcessed(c, src)
}

func assertBundleSourceProcessed(c *gc.C, src BundleDataSource) {
	parts := src.Parts()
	c.Assert(parts, gc.HasLen, 3)
	assertFieldPresent(c, parts[1], "applications.wordpress.offers.offer1.endpoints")
	assertFieldPresent(c, parts[2], "applications.wordpress.offers.offer1.acl.admin")
}

func assertFieldPresent(c *gc.C, part *BundleDataPart, path string) {
	var (
		segments             = strings.Split(path, ".")
		next     interface{} = part.PresenceMap
	)

	for segIndex, segment := range segments {
		c.Assert(next, gc.NotNil, gc.Commentf("incomplete path: %s", strings.Join(segments[:segIndex], ".")))
		switch typ := next.(type) {
		case FieldPresenceMap:
			next = typ[segment]
			c.Assert(next, gc.NotNil, gc.Commentf("incomplete path: %s", strings.Join(segments[:segIndex+1], ".")))
		default:
			c.Fatalf("unexpected type %T at path: %s", typ, strings.Join(segments[:segIndex], "."))
		}
	}
}

func (s *BundleDataSourceSuite) TestParseBundlePartsStrict(c *gc.C) {
	r := strings.NewReader(`
applications:
  wordpress:
    charm: wordpress
    constrain: "mem=8G"
  mysql:
    charm: mysql
    num_uns: 1
relations:
  - ["wordpress:db", "mysql:server"]
--- # overlay.yaml
applications:
  wordpress:
    offers:
      offer1:
        endpoints:
          - "some-endpoint"
--- # overlay2.yaml
applications:
  wordpress:
    offer:
      offer1:
        acl:
          admin: "admin"
          foo: "consume"
`)

	parts, err := parseBundleParts(r)
	c.Assert(err, gc.IsNil)
	c.Assert(parts, gc.HasLen, 3)
	c.Assert(parts[0].UnmarshallError, gc.NotNil)
	c.Assert(parts[0].UnmarshallError.Error(), gc.Matches, ""+
		"unmarshal document 0: yaml: unmarshal errors:\n"+
		"  line 5: field constrain not found in applications\n"+
		"  line 8: field num_uns not found in applications")
	c.Assert(parts[1].UnmarshallError, jc.ErrorIsNil)
	c.Assert(parts[2].UnmarshallError, gc.NotNil)
	c.Assert(parts[2].UnmarshallError.Error(), gc.Matches, ""+
		"unmarshal document 2: yaml: unmarshal errors:\n"+
		"  line 21: field offer not found in applications")
}

func (s *BundleDataSourceSuite) TestResolveAbsoluteFileInclude(c *gc.C) {
	target, err := filepath.Abs(filepath.Join(c.MkDir(), "example"))
	c.Assert(err, gc.IsNil)

	expContent := "example content\n"
	c.Assert(ioutil.WriteFile(target, []byte(expContent), os.ModePerm), gc.IsNil)

	ds := new(resolvedBundleDataSource)

	got, err := ds.ResolveInclude(target)
	c.Assert(err, gc.IsNil)
	c.Assert(string(got), gc.Equals, expContent)
}

func (s *BundleDataSourceSuite) TestResolveRelativeFileInclude(c *gc.C) {
	relTo := c.MkDir()
	target, err := filepath.Abs(filepath.Join(relTo, "example"))
	c.Assert(err, gc.IsNil)

	expContent := "example content\n"
	c.Assert(ioutil.WriteFile(target, []byte(expContent), os.ModePerm), gc.IsNil)

	ds := &resolvedBundleDataSource{
		basePath: relTo,
	}

	got, err := ds.ResolveInclude("./example")
	c.Assert(err, gc.IsNil)
	c.Assert(string(got), gc.Equals, expContent)
}

func (s *BundleDataSourceSuite) TestResolveIncludeErrors(c *gc.C) {
	cwd, err := os.Getwd()
	c.Assert(err, gc.IsNil)

	tmpDir := c.MkDir()
	specs := []struct {
		descr   string
		incPath string
		exp     string
	}{
		{
			descr:   "abs path does not exist",
			incPath: "/some/invalid/path",
			exp:     `include file "/some/invalid/path" not found`,
		},
		{
			descr:   "rel path does not exist",
			incPath: "./missing",
			exp:     `include file "` + cwd + `/missing" not found`,
		},
		{
			descr:   "path points to directory",
			incPath: tmpDir,
			exp:     fmt.Sprintf("include path %q resolves to a folder", tmpDir),
		},
	}

	ds := new(resolvedBundleDataSource)
	for specIndex, spec := range specs {
		c.Logf("[test %d] %s", specIndex, spec.descr)

		_, err := ds.ResolveInclude(spec.incPath)
		c.Assert(err, gc.Not(gc.IsNil))

		c.Assert(err.Error(), gc.Equals, spec.exp)
	}
}

func bundleDirPath(c *gc.C, name string) string {
	path := filepath.Join("internal/test-charm-repo/bundle", name)
	assertIsDir(c, path)
	return path
}

func assertIsDir(c *gc.C, path string) {
	info, err := os.Stat(path)
	c.Assert(err, gc.IsNil)
	c.Assert(info.IsDir(), gc.Equals, true)
}

func archiveBundleDirPath(c *gc.C, name string) string {
	src := filepath.Join("internal/test-charm-repo/bundle", name, "bundle.yaml")
	srcYaml, err := ioutil.ReadFile(src)
	c.Assert(err, gc.IsNil)

	dstPath := filepath.Join(c.MkDir(), "bundle.zip")
	f, err := os.Create(dstPath)
	c.Assert(err, gc.IsNil)
	defer func() { c.Assert(f.Close(), gc.IsNil) }()

	zw := zip.NewWriter(f)
	defer func() { c.Assert(zw.Close(), gc.IsNil) }()
	w, err := zw.Create("bundle.yaml")
	c.Assert(err, gc.IsNil)
	_, err = w.Write(srcYaml)
	c.Assert(err, gc.IsNil)

	return dstPath
}
