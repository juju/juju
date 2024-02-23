// Copyright 2019 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package charm_test

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/yaml.v2"

	"github.com/juju/juju/charm"
)

type bundleDataOverlaySuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&bundleDataOverlaySuite{})

func (*bundleDataOverlaySuite) TestEmptyBaseApplication(c *gc.C) {
	data := `
applications:
  apache2:
---
series: trusty
applications:
  apache2:
    charm: cs:apache2-42
    series: bionic
`[1:]

	_, err := charm.ReadAndMergeBundleData(mustCreateStringDataSource(c, data))
	c.Assert(err, gc.ErrorMatches, `base application "apache2" has no body`)
}

func (*bundleDataOverlaySuite) TestExtractBaseAndOverlayParts(c *gc.C) {
	data := `
applications:
  apache2:
    charm: apache2
    exposed-endpoints:
      www:
        expose-to-spaces:
          - dmz
        expose-to-cidrs:
          - 13.37.0.0/16
    offers:
      my-offer:
        endpoints:
        - apache-website
        - website-cache
        acl:
          admin: admin
          foo: consume
      my-other-offer:
        endpoints:
        - apache-website
saas:
    apache2:
        url: production:admin/info.apache
series: bionic
`[1:]

	expBase := `
applications:
  apache2:
    charm: apache2
saas:
  apache2:
    url: production:admin/info.apache
series: bionic
`[1:]

	expOverlay := `
applications:
  apache2:
    exposed-endpoints:
      www:
        expose-to-spaces:
        - dmz
        expose-to-cidrs:
        - 13.37.0.0/16
    offers:
      my-offer:
        endpoints:
        - apache-website
        - website-cache
        acl:
          admin: admin
          foo: consume
      my-other-offer:
        endpoints:
        - apache-website
`[1:]

	bd, err := charm.ReadBundleData(strings.NewReader(data))
	c.Assert(err, gc.IsNil)

	base, overlay, err := charm.ExtractBaseAndOverlayParts(bd)
	c.Assert(err, jc.ErrorIsNil)

	baseYaml, err := yaml.Marshal(base)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(string(baseYaml), gc.Equals, expBase)

	overlayYaml, err := yaml.Marshal(overlay)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(string(overlayYaml), gc.Equals, expOverlay)
}

func (*bundleDataOverlaySuite) TestExtractBaseAndOverlayPartsWithNoOverlayFields(c *gc.C) {
	data := `
bundle: kubernetes
applications:
  mysql:
    charm: cs:mysql
    scale: 1
  wordpress:
    charm: cs:wordpress
    scale: 2
relations:
- - wordpress:db
  - mysql:mysql
`[1:]

	expBase := `
bundle: kubernetes
applications:
  mysql:
    charm: cs:mysql
    num_units: 1
  wordpress:
    charm: cs:wordpress
    num_units: 2
relations:
- - wordpress:db
  - mysql:mysql
`[1:]

	expOverlay := `
{}
`[1:]

	bd, err := charm.ReadBundleData(strings.NewReader(data))
	c.Assert(err, gc.IsNil)

	base, overlay, err := charm.ExtractBaseAndOverlayParts(bd)
	c.Assert(err, jc.ErrorIsNil)

	baseYaml, err := yaml.Marshal(base)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(string(baseYaml), gc.Equals, expBase)

	overlayYaml, err := yaml.Marshal(overlay)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(string(overlayYaml), gc.Equals, expOverlay)
}

func (*bundleDataOverlaySuite) TestExtractAndMergeWithMixedOverlayBits(c *gc.C) {
	// In this example, mysql defines an offer whereas wordpress does not.
	//
	// When the visitor code examines the application map, it should report
	// back that the filtered "mysql" application key should be retained
	// but the "wordpress" application key should NOT be retained. The
	// applications map should be retained because at least one of its keys
	// has to be retained. However, the "wordpress" entry must be removed.
	// If not, it would be encoded as an empty object which the overlay
	// merge code would mis-interpret as a request to delete the "wordpress"
	// application from the base bundle!
	data := `
bundle: kubernetes
applications:
  mysql:
    charm: cs:mysql
    scale: 1
    offers:
      my-offer:
        endpoints:
        - apache-website
        - website-cache
        acl:
          admin: admin
          foo: consume
  wordpress:
    charm: cs:wordpress
    channel: edge
    scale: 2
    options:
      foo: bar
relations:
- - wordpress:db
  - mysql:mysql
`[1:]

	expBase := `
bundle: kubernetes
applications:
  mysql:
    charm: cs:mysql
    num_units: 1
  wordpress:
    charm: cs:wordpress
    channel: edge
    num_units: 2
    options:
      foo: bar
relations:
- - wordpress:db
  - mysql:mysql
`[1:]

	expOverlay := `
applications:
  mysql:
    offers:
      my-offer:
        endpoints:
        - apache-website
        - website-cache
        acl:
          admin: admin
          foo: consume
`[1:]

	bd, err := charm.ReadBundleData(strings.NewReader(data))
	c.Assert(err, gc.IsNil)

	base, overlay, err := charm.ExtractBaseAndOverlayParts(bd)
	c.Assert(err, jc.ErrorIsNil)

	baseYaml, err := yaml.Marshal(base)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(string(baseYaml), gc.Equals, expBase)

	overlayYaml, err := yaml.Marshal(overlay)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(string(overlayYaml), gc.Equals, expOverlay)

	// Check that merging the output back into a bundle yields the original
	r := strings.NewReader(string(baseYaml) + "\n---\n" + string(overlayYaml))
	ds, err := charm.StreamBundleDataSource(r, "")
	c.Assert(err, jc.ErrorIsNil)

	newBd, err := charm.ReadAndMergeBundleData(ds)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(newBd, gc.DeepEquals, bd)
}

func (*bundleDataOverlaySuite) TestVerifyNoOverlayFieldsPresent(c *gc.C) {
	data := `
applications:
  apache2:
    charm: apache2
    offers:
      my-offer:
        endpoints:
        - apache-website
        - website-cache
        acl:
          admin: admin
          foo: consume
      my-other-offer:
        endpoints:
        - apache-website
saas:
    apache2:
        url: production:admin/info.apache
series: bionic
`

	bd, err := charm.ReadBundleData(strings.NewReader(data))
	c.Assert(err, gc.IsNil)

	static, overlay, err := charm.ExtractBaseAndOverlayParts(bd)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(charm.VerifyNoOverlayFieldsPresent(static), gc.Equals, nil)

	expErrors := []string{
		"applications.apache2.offers can only appear in an overlay section",
		"applications.apache2.offers.my-offer.endpoints can only appear in an overlay section",
		"applications.apache2.offers.my-offer.acl can only appear in an overlay section",
		"applications.apache2.offers.my-other-offer.endpoints can only appear in an overlay section",
	}
	err = charm.VerifyNoOverlayFieldsPresent(overlay)
	c.Assert(err, gc.FitsTypeOf, (*charm.VerificationError)(nil))
	errors := err.(*charm.VerificationError).Errors
	errStrings := make([]string, len(errors))
	for i, err := range errors {
		errStrings[i] = err.Error()
	}
	sort.Strings(errStrings)
	sort.Strings(expErrors)
	c.Assert(errStrings, jc.DeepEquals, expErrors)
}

func (*bundleDataOverlaySuite) TestVerifyNoOverlayFieldsPresentOnNilOptionValue(c *gc.C) {
	data := `
# ssl_ca is left uninitialized so it resolves to nil
ssl_ca: &ssl_ca

applications:
  apache2:
    options:
      foo: bar
      ssl_ca: *ssl_ca
series: bionic
`

	bd, err := charm.ReadBundleData(strings.NewReader(data))
	c.Assert(err, gc.IsNil)

	static, _, err := charm.ExtractBaseAndOverlayParts(bd)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(charm.VerifyNoOverlayFieldsPresent(static), gc.Equals, nil)
}

func (*bundleDataOverlaySuite) TestOverrideCharmAndSeries(c *gc.C) {
	testBundleMergeResult(c, `
applications:
  apache2:
    charm: apache2
    num_units: 1
---
series: trusty
applications:
  apache2:
    charm: cs:apache2-42
    series: bionic
`, `
applications:
  apache2:
    charm: cs:apache2-42
    series: bionic
    num_units: 1
series: trusty
`,
	)
}

func (*bundleDataOverlaySuite) TestOverrideScale(c *gc.C) {
	testBundleMergeResult(c, `
applications:
  apache2:
    charm: apache2
    scale: 1
---
applications:
  apache2:
    scale: 2
`, `
applications:
  apache2:
    charm: apache2
    num_units: 2
`,
	)
}

func (*bundleDataOverlaySuite) TestOverrideScaleWithNumUnits(c *gc.C) {
	// This shouldn't be allowed, but the code does, so we should test it!
	// Notice that scale doesn't exist.
	testBundleMergeResult(c, `
applications:
  apache2:
    charm: apache2
    scale: 1
---
applications:
  apache2:
    num_units: 2
`, `
applications:
  apache2:
    charm: apache2
    num_units: 2
`,
	)
}

func (*bundleDataOverlaySuite) TestMultipleOverrideScale(c *gc.C) {
	testBundleMergeResult(c, `
applications:
  apache2:
    charm: apache2
    scale: 1
---
applications:
  apache2:
    scale: 50
---
applications:
  apache2:
    scale: 3
`, `
applications:
  apache2:
    charm: apache2
    num_units: 3
`,
	)
}

func (*bundleDataOverlaySuite) TestOverrideScaleWithZero(c *gc.C) {
	testBundleMergeResult(c, `
applications:
  apache2:
    charm: apache2
    scale: 1
---
applications:
  apache2:
    scale: 0
`, `
applications:
  apache2:
    charm: apache2
    num_units: 1
`,
	)
}

func (*bundleDataOverlaySuite) TestAddAndOverrideResourcesStorageDevicesAndBindings(c *gc.C) {
	testBundleMergeResult(c, `
applications:
  apache2:
    charm: apache2
    resources:
      res1: foo
    storage:
      dsk0: dsk0
    devices:
      dev0: dev0
---
applications:
  apache2:
    resources:
      res1: bar
      res2: new
    storage:
      dsk0: vol0
      dsk1: new
    devices:
      dev0: net
      dev1: new
    bindings:
      bnd0: new
`, `
applications:
  apache2:
    charm: apache2
    resources:
      res1: bar
      res2: new
    storage:
      dsk0: vol0
      dsk1: new
    devices:
      dev0: net
      dev1: new
    bindings:
      bnd0: new
`,
	)
}

func (*bundleDataOverlaySuite) TestAddAndOverrideOptionsAndAnnotations(c *gc.C) {
	testBundleMergeResult(c, `
applications:
  apache2:
    charm: apache2
    options:
      opt1: foo
      opt1: bar
      mapOpt:
        foo: bar
---
applications:
  apache2:
    options:
      opt1: foo
      opt2: ""
      mapOpt:
    annotations:
      ann1: new
`, `
applications:
  apache2:
    charm: apache2
    options:
      opt1: foo
      opt2: ""
    annotations:
      ann1: new
`,
	)
}

func (*bundleDataOverlaySuite) TestOverrideUnitsTrustConstraintsAndExposeFlags(c *gc.C) {
	testBundleMergeResult(c, `
applications:
  apache2:
    charm: apache2
---
applications:
  apache2:
    num_units: 4
    to:
    - lxd/0
    - lxd/1
    - lxd/2
    - lxd/3
    expose: true
`, `
applications:
  apache2:
    charm: apache2
    num_units: 4
    to:
    - lxd/0
    - lxd/1
    - lxd/2
    - lxd/3
    expose: true
`,
	)
}

func (*bundleDataOverlaySuite) TestAddModifyAndRemoveApplicationsAndRelations(c *gc.C) {
	testBundleMergeResult(c, `
applications:
  apache2:
    charm: apache2
  wordpress:
    charm: wordpress
  dummy:
    charm: dummy
relations:
- - wordpress:www
  - apache2:www
---
applications:
  apache2:
    charm: apache2
  wordpress: 
relations:
- - dummy:www
  - apache2:www
`, `
applications:
  apache2:
    charm: apache2
  dummy:
    charm: dummy
relations:
- - dummy:www
  - apache2:www
`,
	)
}

func (*bundleDataOverlaySuite) TestAddModifyAndRemoveSaasBlocksAndRelations(c *gc.C) {
	testBundleMergeResult(c, `
saas:
  postgres:
    url: jaas:admin/default.postgres
applications:
  wordpress:
    charm: wordpress
relations:
- - wordpress:db
  - postgres:db
---
saas:
  postgres: 
  cockroachdb:
    url: jaas:admin/default.cockroachdb
`, `
applications:
  wordpress:
    charm: wordpress
saas:
  cockroachdb:
    url: jaas:admin/default.cockroachdb
`,
	)
}

func (*bundleDataOverlaySuite) TestAddAndRemoveOffers(c *gc.C) {
	testBundleMergeResult(c, `
applications:
  apache2:
    charm: apache2
    channel: stable
    revision: 26
--- # offer blocks are overlay-specific
applications:
  apache2:
    offers:
      my-offer:
        endpoints:
        - apache-website
        - website-cache
        acl:
          admin: admin
          foo: consume
      my-other-offer:
        endpoints:
        - apache-website
--- 
applications:
  apache2:
    offers:
      my-other-offer:
`, `
applications:
  apache2:
    charm: apache2
    channel: stable
    revision: 26
    offers:
      my-offer:
        endpoints:
        - apache-website
        - website-cache
        acl:
          admin: admin
          foo: consume
`,
	)
}

func (*bundleDataOverlaySuite) TestAddAndRemoveMachines(c *gc.C) {
	testBundleMergeResult(c, `
applications:
  apache2:
    charm: apache2
    channel: stable
    revision: 26
machines:
  "0": {}
  "1": {}
---
machines:
  "2": {}
`, `
applications:
  apache2:
    charm: apache2
    channel: stable
    revision: 26
machines:
  "2": {}
`,
	)
}

func (*bundleDataOverlaySuite) TestYAMLInterpolation(c *gc.C) {
	base := `
applications:
    django:
        expose: true
        charm: django
        num_units: 1
        options:
            general: good
        annotations:
            key1: value1
            key2: value2
        to: [1]
    memcached:
        charm: mem
        revision: 47
        series: trusty
        num_units: 1
        options:
            key: value
relations:
    - - "django"
      - "memcached"
machines:
    1:
        annotations: {foo: bar}`

	removeDjango := `
applications:
    django:
`

	addWiki := `
defaultwiki: &DEFAULTWIKI
    charm: "mediawiki"
    revision: 5
    series: trusty
    num_units: 1
    options: &WIKIOPTS
        debug: false
        name: Please set name of wiki
        skin: vector

applications:
    wiki:
        <<: *DEFAULTWIKI
        options:
            <<: *WIKIOPTS
            name: The name override
relations:
    - - "wiki"
      - "memcached"
`

	bd, err := charm.ReadAndMergeBundleData(
		mustCreateStringDataSource(c, base),
		mustCreateStringDataSource(c, removeDjango),
		mustCreateStringDataSource(c, addWiki),
	)
	c.Assert(err, gc.IsNil)

	merged, err := yaml.Marshal(bd)
	c.Assert(err, gc.IsNil)

	exp := `
applications:
  memcached:
    charm: mem
    revision: 47
    series: trusty
    num_units: 1
    options:
      key: value
  wiki:
    charm: mediawiki
    revision: 5
    series: trusty
    num_units: 1
    options:
      debug: false
      name: The name override
      skin: vector
machines:
  "1":
    annotations:
      foo: bar
relations:
- - wiki
  - memcached
`

	c.Assert("\n"+string(merged), gc.Equals, exp)
}

func (*bundleDataOverlaySuite) TestReadAndMergeBundleDataWithIncludes(c *gc.C) {
	data := `
applications:
  apache2:
    options:
      opt-raw: include-file://foo
      opt-b64: include-base64://foo
      opt-other:
        some: value
    annotations:
      anno-raw: include-file://foo
      anno-b64: include-base64://foo
      anno-other: value
machines:
  "0": {}
  "1":
    annotations:
      anno-raw: include-file://foo
      anno-b64: include-base64://foo
      anno-other: value
`

	ds := srcWithFakeIncludeResolver{
		src: mustCreateStringDataSource(c, data),
		resolveMap: map[string][]byte{
			"foo": []byte("lorem$ipsum$"),
		},
	}

	bd, err := charm.ReadAndMergeBundleData(ds)
	c.Assert(err, gc.IsNil)

	merged, err := yaml.Marshal(bd)
	c.Assert(err, gc.IsNil)

	exp := `
applications:
  apache2:
    options:
      opt-b64: bG9yZW0kaXBzdW0k
      opt-other:
        some: value
      opt-raw: lorem$ipsum$
    annotations:
      anno-b64: bG9yZW0kaXBzdW0k
      anno-other: value
      anno-raw: lorem$ipsum$
machines:
  "0": {}
  "1":
    annotations:
      anno-b64: bG9yZW0kaXBzdW0k
      anno-other: value
      anno-raw: lorem$ipsum$
`

	c.Assert("\n"+string(merged), gc.Equals, exp)
}

func (*bundleDataOverlaySuite) TestBundleDataSourceRelativeIncludes(c *gc.C) {
	base := `
applications:
  django:
    charm: cs:django
    options:
      opt1: include-file://relative-to-base.txt
`

	overlays := `
applications:
  django:
    charm: cs:django
    options:
      opt2: include-file://relative-to-overlay.txt
---
applications:
  django:
    charm: cs:django
    options:
      opt3: include-file://relative-to-overlay.txt
`

	baseDir := c.MkDir()
	mustWriteFile(c, filepath.Join(baseDir, "bundle.yaml"), base)
	mustWriteFile(c, filepath.Join(baseDir, "relative-to-base.txt"), "lorem ipsum")

	ovlDir := c.MkDir()
	mustWriteFile(c, filepath.Join(ovlDir, "overlays.yaml"), overlays)
	mustWriteFile(c, filepath.Join(ovlDir, "relative-to-overlay.txt"), "dolor")

	bd, err := charm.ReadAndMergeBundleData(
		mustCreateLocalDataSource(c, filepath.Join(baseDir, "bundle.yaml")),
		mustCreateLocalDataSource(c, filepath.Join(ovlDir, "overlays.yaml")),
	)
	c.Assert(err, gc.IsNil)

	merged, err := yaml.Marshal(bd)
	c.Assert(err, gc.IsNil)

	exp := `
applications:
  django:
    charm: cs:django
    options:
      opt1: lorem ipsum
      opt2: dolor
      opt3: dolor
`

	c.Assert("\n"+string(merged), gc.Equals, exp)
}

func (*bundleDataOverlaySuite) TestBundleDataSourceWithEmptyOverlay(c *gc.C) {
	base := `
applications:
  django:
    charm: cs:django
`

	overlays := `
---
`

	baseDir := c.MkDir()
	mustWriteFile(c, filepath.Join(baseDir, "bundle.yaml"), base)

	ovlDir := c.MkDir()
	mustWriteFile(c, filepath.Join(ovlDir, "overlays.yaml"), overlays)

	bd, err := charm.ReadAndMergeBundleData(
		mustCreateLocalDataSource(c, filepath.Join(baseDir, "bundle.yaml")),
		mustCreateLocalDataSource(c, filepath.Join(ovlDir, "overlays.yaml")),
	)
	c.Assert(err, gc.IsNil)

	merged, err := yaml.Marshal(bd)
	c.Assert(err, gc.IsNil)

	exp := `
applications:
  django:
    charm: cs:django
`

	c.Assert("\n"+string(merged), gc.Equals, exp)
}

func (*bundleDataOverlaySuite) TestReadAndMergeBundleDataWithRelativeCharmPaths(c *gc.C) {
	base := `
applications:
  apache2:
    charm: ./apache
  mysql:
    charm: cs:mysql
  varnish:
    charm: /some/absolute/path
`

	overlay := `
applications:
  wordpress:
    charm: ./wordpress
`
	bd, err := charm.ReadAndMergeBundleData(
		mustCreateStringDataSourceWithBasePath(c, base, "/tmp/base"),
		mustCreateStringDataSourceWithBasePath(c, overlay, "/overlay"),
	)
	c.Assert(err, gc.IsNil)

	merged, err := yaml.Marshal(bd)
	c.Assert(err, gc.IsNil)

	exp := `
applications:
  apache2:
    charm: /tmp/base/apache
  mysql:
    charm: cs:mysql
  varnish:
    charm: /some/absolute/path
  wordpress:
    charm: /overlay/wordpress
`[1:]

	c.Assert(string(merged), gc.Equals, exp)
}

type srcWithFakeIncludeResolver struct {
	src        charm.BundleDataSource
	resolveMap map[string][]byte
}

func (s srcWithFakeIncludeResolver) Parts() []*charm.BundleDataPart {
	return s.src.Parts()
}

func (s srcWithFakeIncludeResolver) BasePath() string {
	return s.src.BasePath()
}

func (s srcWithFakeIncludeResolver) ResolveInclude(path string) ([]byte, error) {
	var (
		data  []byte
		found bool
	)

	if s.resolveMap != nil {
		data, found = s.resolveMap[path]
	}

	if !found {
		return nil, errors.NotFoundf(path)
	}

	return data, nil
}

// testBundleMergeResult reads and merges the bundle and any overlays in src,
// serializes the merged bundle back to yaml and compares it with exp.
func testBundleMergeResult(c *gc.C, src, exp string) {
	bd, err := charm.ReadAndMergeBundleData(mustCreateStringDataSource(c, src))
	c.Assert(err, gc.IsNil)

	merged, err := yaml.Marshal(bd)
	c.Assert(err, gc.IsNil)
	c.Assert("\n"+string(merged), gc.Equals, exp)
}

func mustWriteFile(c *gc.C, path, content string) {
	err := ioutil.WriteFile(path, []byte(content), os.ModePerm)
	c.Assert(err, gc.IsNil)
}

func mustCreateLocalDataSource(c *gc.C, path string) charm.BundleDataSource {
	ds, err := charm.LocalBundleDataSource(path)
	c.Assert(err, gc.IsNil, gc.Commentf(path))
	return ds
}

func mustCreateStringDataSource(c *gc.C, data string) charm.BundleDataSource {
	ds, err := charm.StreamBundleDataSource(strings.NewReader(data), "")
	c.Assert(err, gc.IsNil)
	return ds
}

func mustCreateStringDataSourceWithBasePath(c *gc.C, data, basePath string) charm.BundleDataSource {
	ds, err := charm.StreamBundleDataSource(strings.NewReader(data), basePath)
	c.Assert(err, gc.IsNil)
	return ds
}
