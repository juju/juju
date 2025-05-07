// Copyright 2019 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package charm

import (
	"archive/zip"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/juju/tc"

	"github.com/juju/juju/internal/testhelpers"
)

type BundleDataSourceSuite struct {
	testhelpers.IsolationSuite
}

var _ = tc.Suite(&BundleDataSourceSuite{})

var bundlePath = "internal/test-charm-repo/bundle/wordpress-multidoc/bundle.yaml"

func (s *BundleDataSourceSuite) TestReadBundleFromLocalFile(c *tc.C) {
	path := bundleDirPath(c, "wordpress-multidoc")
	src, err := LocalBundleDataSource(filepath.Join(path, "bundle.yaml"))
	c.Assert(err, tc.IsNil)

	raw, err := os.ReadFile(bundlePath)
	c.Assert(err, tc.ErrorIsNil)
	assertBundleSourceProcessed(c, src, string(raw))
}

func (s *BundleDataSourceSuite) TestReadBundleFromExplodedArchiveFolder(c *tc.C) {
	path := bundleDirPath(c, "wordpress-multidoc")
	src, err := LocalBundleDataSource(path)
	c.Assert(err, tc.IsNil)

	raw, err := os.ReadFile(bundlePath)
	c.Assert(err, tc.ErrorIsNil)
	assertBundleSourceProcessed(c, src, string(raw))
}

func (s *BundleDataSourceSuite) TestReadBundleFromArchive(c *tc.C) {
	path := archiveBundleDirPath(c, "wordpress-multidoc")
	src, err := LocalBundleDataSource(path)
	c.Assert(err, tc.IsNil)

	raw, err := os.ReadFile(bundlePath)
	c.Assert(err, tc.ErrorIsNil)
	assertBundleSourceProcessed(c, src, string(raw))
}

func (s *BundleDataSourceSuite) TestReadBundleFromStream(c *tc.C) {
	bundle := `
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
`

	src, err := StreamBundleDataSource(strings.NewReader(bundle), "https://example.com")
	c.Assert(err, tc.IsNil)
	assertBundleSourceProcessed(c, src, bundle)
}

func assertBundleSourceProcessed(c *tc.C, src BundleDataSource, bundle string) {
	parts := src.Parts()
	c.Assert(parts, tc.HasLen, 3)
	assertFieldPresent(c, parts[1], "applications.wordpress.offers.offer1.endpoints")
	assertFieldPresent(c, parts[2], "applications.wordpress.offers.offer1.acl.admin")
	c.Assert(string(src.BundleBytes()), tc.Equals, bundle)
}

func assertFieldPresent(c *tc.C, part *BundleDataPart, path string) {
	var (
		segments             = strings.Split(path, ".")
		next     interface{} = part.PresenceMap
	)

	for segIndex, segment := range segments {
		c.Assert(next, tc.NotNil, tc.Commentf("incomplete path: %s", strings.Join(segments[:segIndex], ".")))
		switch typ := next.(type) {
		case FieldPresenceMap:
			next = typ[segment]
			c.Assert(next, tc.NotNil, tc.Commentf("incomplete path: %s", strings.Join(segments[:segIndex+1], ".")))
		default:
			c.Fatalf("unexpected type %T at path: %s", typ, strings.Join(segments[:segIndex], "."))
		}
	}
}

func (s *BundleDataSourceSuite) TestParseBundlePartsStrict(c *tc.C) {
	b := []byte(`
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

	parts, err := parseBundleParts(b)
	c.Assert(err, tc.IsNil)
	c.Assert(parts, tc.HasLen, 3)
	c.Assert(parts[0].UnmarshallError, tc.NotNil)
	c.Assert(parts[0].UnmarshallError.Error(), tc.Matches, ""+
		"unmarshal document 0: yaml: unmarshal errors:\n"+
		"  line 5: field constrain not found in applications\n"+
		"  line 8: field num_uns not found in applications")
	c.Assert(parts[1].UnmarshallError, tc.ErrorIsNil)
	c.Assert(parts[2].UnmarshallError, tc.NotNil)
	c.Assert(parts[2].UnmarshallError.Error(), tc.Matches, ""+
		"unmarshal document 2: yaml: unmarshal errors:\n"+
		"  line 21: field offer not found in applications")
}

func (s *BundleDataSourceSuite) TestResolveAbsoluteFileInclude(c *tc.C) {
	target, err := filepath.Abs(filepath.Join(c.MkDir(), "example"))
	c.Assert(err, tc.IsNil)

	expContent := "example content\n"
	c.Assert(os.WriteFile(target, []byte(expContent), os.ModePerm), tc.IsNil)

	ds := new(resolvedBundleDataSource)

	got, err := ds.ResolveInclude(target)
	c.Assert(err, tc.IsNil)
	c.Assert(string(got), tc.Equals, expContent)
}

func (s *BundleDataSourceSuite) TestResolveRelativeFileInclude(c *tc.C) {
	relTo := c.MkDir()
	target, err := filepath.Abs(filepath.Join(relTo, "example"))
	c.Assert(err, tc.IsNil)

	expContent := "example content\n"
	c.Assert(os.WriteFile(target, []byte(expContent), os.ModePerm), tc.IsNil)

	ds := &resolvedBundleDataSource{
		basePath: relTo,
	}

	got, err := ds.ResolveInclude("./example")
	c.Assert(err, tc.IsNil)
	c.Assert(string(got), tc.Equals, expContent)
}

func (s *BundleDataSourceSuite) TestResolveIncludeErrors(c *tc.C) {
	cwd, err := os.Getwd()
	c.Assert(err, tc.IsNil)

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
		c.Assert(err, tc.Not(tc.IsNil))

		c.Assert(err.Error(), tc.Equals, spec.exp)
	}
}

func bundleDirPath(c *tc.C, name string) string {
	path := filepath.Join("internal/test-charm-repo/bundle", name)
	assertIsDir(c, path)
	return path
}

func assertIsDir(c *tc.C, path string) {
	info, err := os.Stat(path)
	c.Assert(err, tc.IsNil)
	c.Assert(info.IsDir(), tc.Equals, true)
}

func archiveBundleDirPath(c *tc.C, name string) string {
	src := filepath.Join("internal/test-charm-repo/bundle", name, "bundle.yaml")
	srcYaml, err := os.ReadFile(src)
	c.Assert(err, tc.IsNil)

	dstPath := filepath.Join(c.MkDir(), "bundle.zip")
	f, err := os.Create(dstPath)
	c.Assert(err, tc.IsNil)
	defer func() { c.Assert(f.Close(), tc.IsNil) }()

	zw := zip.NewWriter(f)
	defer func() { c.Assert(zw.Close(), tc.IsNil) }()
	w, err := zw.Create("bundle.yaml")
	c.Assert(err, tc.IsNil)
	_, err = w.Write(srcYaml)
	c.Assert(err, tc.IsNil)

	return dstPath
}
