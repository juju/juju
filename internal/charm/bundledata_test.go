// Copyright 2014 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package charm_test

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	stdtesting "testing"

	"github.com/juju/tc"

	"github.com/juju/juju/internal/charm"
	"github.com/juju/juju/internal/testhelpers"
)

type bundleDataSuite struct {
	testhelpers.IsolationSuite
}

func TestBundleDataSuite(t *stdtesting.T) { tc.Run(t, &bundleDataSuite{}) }

const mediawikiBundle = `
default-base: ubuntu@20.04
applications:
    mediawiki:
        charm: "mediawiki"
        num_units: 1
        expose: true
        options:
            debug: false
            name: Please set name of wiki
            skin: vector
        annotations:
            "gui-x": 609
            "gui-y": -15
        storage:
            valid-store: 10G
        bindings:
            db: db
            website: public
        resources:
            data: 3
    mysql:
        charm: "mysql"
        num_units: 2
        to: [0, mediawiki/0]
        base: ubuntu@22.04
        options:
            "binlog-format": MIXED
            "block-size": 5.3
            "dataset-size": "80%"
            flavor: distro
            "ha-bindiface": eth0
            "ha-mcastport": 5411.1
        annotations:
            "gui-x": 610
            "gui-y": 255
        constraints: "mem=8g"
        bindings:
            db: db
        resources:
            data: "resources/data.tar"
relations:
    - ["mediawiki:db", "mysql:db"]
    - ["mysql:foo", "mediawiki:bar"]
machines:
    0:
         constraints: 'arch=amd64 mem=4g'
         annotations:
             foo: bar
tags:
    - super
    - awesome
description: |
    Everything is awesome. Everything is cool when we work as a team.
    Lovely day.
`

// Revision are an *int, create a few ints for their addresses used in tests.
var (
	five        = 5
	ten         = 10
	twentyEight = 28
)

var parseTests = []struct {
	about       string
	data        string
	expectedBD  *charm.BundleData
	expectedErr string
}{{
	about: "mediawiki",
	data:  mediawikiBundle,
	expectedBD: &charm.BundleData{
		DefaultBase: "ubuntu@20.04",
		Applications: map[string]*charm.ApplicationSpec{
			"mediawiki": {
				Charm:    "mediawiki",
				NumUnits: 1,
				Expose:   true,
				Options: map[string]interface{}{
					"debug": false,
					"name":  "Please set name of wiki",
					"skin":  "vector",
				},
				Annotations: map[string]string{
					"gui-x": "609",
					"gui-y": "-15",
				},
				Storage: map[string]string{
					"valid-store": "10G",
				},
				EndpointBindings: map[string]string{
					"db":      "db",
					"website": "public",
				},
				Resources: map[string]interface{}{
					"data": 3,
				},
			},
			"mysql": {
				Charm:    "mysql",
				NumUnits: 2,
				To:       []string{"0", "mediawiki/0"},
				Base:     "ubuntu@22.04",
				Options: map[string]interface{}{
					"binlog-format": "MIXED",
					"block-size":    5.3,
					"dataset-size":  "80%",
					"flavor":        "distro",
					"ha-bindiface":  "eth0",
					"ha-mcastport":  5411.1,
				},
				Annotations: map[string]string{
					"gui-x": "610",
					"gui-y": "255",
				},
				Constraints: "mem=8g",
				EndpointBindings: map[string]string{
					"db": "db",
				},
				Resources: map[string]interface{}{"data": "resources/data.tar"},
			},
		},
		Machines: map[string]*charm.MachineSpec{
			"0": {
				Constraints: "arch=amd64 mem=4g",
				Annotations: map[string]string{
					"foo": "bar",
				},
			},
		},
		Relations: [][]string{
			{"mediawiki:db", "mysql:db"},
			{"mysql:foo", "mediawiki:bar"},
		},
		Tags: []string{"super", "awesome"},
		Description: `Everything is awesome. Everything is cool when we work as a team.
Lovely day.
`,
	},
}, {
	about: "relations specified with hyphens",
	data: `
relations:
    - - "mediawiki:db"
      - "mysql:db"
    - - "mysql:foo"
      - "mediawiki:bar"
`,
	expectedBD: &charm.BundleData{
		Relations: [][]string{
			{"mediawiki:db", "mysql:db"},
			{"mysql:foo", "mediawiki:bar"},
		},
	},
}, {
	about: "scale alias for num_units",
	data: `
applications:
    mysql:
        charm: mysql
        scale: 1
`,
	expectedBD: &charm.BundleData{
		Applications: map[string]*charm.ApplicationSpec{
			"mysql": {
				Charm:    "mysql",
				NumUnits: 1,
			},
		},
	},
}, {
	about: "application requiring explicit trust",
	data: `
applications:
    aws-integrator:
        charm: aws-integrator
        num_units: 1
        trust: true
`,
	expectedBD: &charm.BundleData{
		Applications: map[string]*charm.ApplicationSpec{
			"aws-integrator": {
				Charm:         "aws-integrator",
				NumUnits:      1,
				RequiresTrust: true,
			},
		},
	},
}, {
	about: "application defining offers",
	data: `
applications:
    apache2:
      charm: "apache2"
      revision: 28
      num_units: 1
      offers:
        offer1:
          endpoints:
            - "apache-website"
            - "apache-proxy"
          acl:
            admin: "admin"
            foo: "consume"
        offer2:
          endpoints:
            - "apache-website"
`,
	expectedBD: &charm.BundleData{
		Applications: map[string]*charm.ApplicationSpec{
			"apache2": {
				Charm:    "apache2",
				Revision: &twentyEight,
				NumUnits: 1,
				Offers: map[string]*charm.OfferSpec{
					"offer1": {
						Endpoints: []string{
							"apache-website",
							"apache-proxy",
						},
						ACL: map[string]string{
							"admin": "admin",
							"foo":   "consume",
						},
					},
					"offer2": {
						Endpoints: []string{
							"apache-website",
						},
					},
				},
			},
		},
	},
}, {
	about: "saas offerings",
	data: `
saas:
    apache2:
        url: production:admin/info.apache
applications:
    apache2:
      charm: "apache2"
      revision: 10
      num_units: 1
`,
	expectedBD: &charm.BundleData{
		Saas: map[string]*charm.SaasSpec{
			"apache2": {
				URL: "production:admin/info.apache",
			},
		},
		Applications: map[string]*charm.ApplicationSpec{
			"apache2": {
				Charm:    "apache2",
				Revision: &ten,
				NumUnits: 1,
			},
		},
	},
}, {
	about: "saas offerings with relations",
	data: `
saas:
    mysql:
        url: production:admin/info.mysql
applications:
    wordpress:
      charm: "ch:wordpress"
      revision: 10
      num_units: 1
relations:
- - wordpress:db
  - mysql:db
`,
	expectedBD: &charm.BundleData{
		Saas: map[string]*charm.SaasSpec{
			"mysql": {
				URL: "production:admin/info.mysql",
			},
		},
		Applications: map[string]*charm.ApplicationSpec{
			"wordpress": {
				Charm:    "ch:wordpress",
				Revision: &ten,
				NumUnits: 1,
			},
		},
		Relations: [][]string{
			{"wordpress:db", "mysql:db"},
		},
	},
}, {
	about: "charm channel",
	data: `
applications:
    wordpress:
      charm: "wordpress"
      revision: 10
      channel: edge
      num_units: 1
`,
	expectedBD: &charm.BundleData{
		Applications: map[string]*charm.ApplicationSpec{
			"wordpress": {
				Charm:    "wordpress",
				Channel:  "edge",
				Revision: &ten,
				NumUnits: 1,
			},
		},
	},
}, {
	about: "charm revision and channel",
	data: `
applications:
    wordpress:
      charm: "wordpress"
      revision: 5
      channel: edge
      num_units: 1
`,
	expectedBD: &charm.BundleData{
		Applications: map[string]*charm.ApplicationSpec{
			"wordpress": {
				Charm:    "wordpress",
				Revision: &five,
				Channel:  "edge",
				NumUnits: 1,
			},
		},
	},
}}

func (*bundleDataSuite) TestParse(c *tc.C) {
	for i, test := range parseTests {
		c.Logf("test %d: %s", i, test.about)
		bd, err := charm.ReadBundleData(strings.NewReader(test.data))
		if test.expectedErr != "" {
			c.Assert(err, tc.ErrorMatches, test.expectedErr)
			continue
		}
		c.Assert(err, tc.IsNil)
		c.Assert(bd, tc.DeepEquals, test.expectedBD)
	}
}

func (*bundleDataSuite) TestCodecRoundTrip(c *tc.C) {
	for i, test := range parseTests {
		if test.expectedErr != "" {
			continue
		}
		// Check that for all the known codecs, we can
		// round-trip the bundle data through them.
		for _, codec := range codecs {

			c.Logf("Code Test %s for test %d: %s", codec.Name, i, test.about)

			data, err := codec.Marshal(test.expectedBD)
			c.Assert(err, tc.IsNil)
			var bd charm.BundleData
			err = codec.Unmarshal(data, &bd)
			c.Assert(err, tc.IsNil)

			for _, app := range bd.Applications {
				for resName, res := range app.Resources {
					if val, ok := res.(float64); ok {
						app.Resources[resName] = int(val)
					}
				}
			}

			c.Assert(&bd, tc.DeepEquals, test.expectedBD)
		}
	}
}

func (*bundleDataSuite) TestParseLocal(c *tc.C) {
	path := "internal/test-charm-repo/quanta/riak"
	data := fmt.Sprintf(`
        applications:
            dummy:
                charm: %s
                num_units: 1
    `, path)
	bd, err := charm.ReadBundleData(strings.NewReader(data))
	c.Assert(err, tc.IsNil)
	c.Assert(bd, tc.DeepEquals, &charm.BundleData{
		Applications: map[string]*charm.ApplicationSpec{
			"dummy": {
				Charm:    path,
				NumUnits: 1,
			},
		}})
}

var verifyErrorsTests = []struct {
	about  string
	data   string
	errors []string
}{{
	about: "as many errors as possible",
	data: `
default-base: "invalidbase"

saas:
    apache2:
        url: '!some-bogus/url'
    riak:
        url: production:admin/info.riak
machines:
    0:
        constraints: 'bad constraints'
        annotations:
            foo: bar
        base: 'bad base'
    bogus:
    3:
applications:
    mediawiki:
        charm: "bogus:precise/mediawiki-10"
        num_units: -4
        options:
            debug: false
            name: Please set name of wiki
            skin: vector
        annotations:
            "gui-x": 609
            "gui-y": -15
        resources:
            "": 42
            "foo":
               "not": int
    riak:
        charm: "./somepath"
    mysql:
        charm: "mysql"
        num_units: 2
        to: [0, mediawiki/0, nowhere/3, 2, "bad placement"]
        options:
            "binlog-format": MIXED
            "block-size": 5
            "dataset-size": "80%"
            flavor: distro
            "ha-bindiface": eth0
            "ha-mcastport": 5411
        annotations:
            "gui-x": 610
            "gui-y": 255
        constraints: "bad constraints"
    wordpress:
          charm: wordpress
    postgres:
        charm: "postgres"
    terracotta:
        charm: "terracotta"
        base: "ubuntu@22.04"
    ceph:
          charm: ceph
          storage:
              valid-storage: 3,10G
              no_underscores: 123
    ceph-osd:
          charm: ceph-osd
          storage:
              invalid-storage: "bad storage constraints"
relations:
    - ["mediawiki:db", "mysql:db"]
    - ["mysql:foo", "mediawiki:bar"]
    - ["arble:bar"]
    - ["arble:bar", "mediawiki:db"]
    - ["mysql:foo", "mysql:bar"]
    - ["mysql:db", "mediawiki:db"]
    - ["mediawiki/db", "mysql:db"]
    - ["wordpress", "mysql"]
    - ["wordpress:db", "riak:db"]
`,
	errors: []string{
		`bundle declares an invalid base "invalidbase"`,
		`invalid offer URL "!some-bogus/url" for SAAS apache2`,
		`invalid storage name "no_underscores" in application "ceph"`,
		`invalid storage "invalid-storage" in application "ceph-osd": bad storage constraint`,
		`machine "3" is not referred to by a placement directive`,
		`machine "bogus" is not referred to by a placement directive`,
		`invalid machine id "bogus" found in machines`,
		`invalid constraints "bad constraints" in machine "0": bad constraint`,
		`invalid charm URL in application "mediawiki": cannot parse URL "bogus:precise/mediawiki-10": schema "bogus" not valid`,
		`charm path in application "riak" does not exist: internal/test-charm-repo/bundle/somepath`,
		`invalid constraints "bad constraints" in application "mysql": bad constraint`,
		`negative number of units specified on application "mediawiki"`,
		`missing resource name on application "mediawiki"`,
		`resource revision "mediawiki" is not int or string`,
		`too many units specified in unit placement for application "mysql"`,
		`placement "nowhere/3" refers to an application not defined in this bundle`,
		`placement "mediawiki/0" specifies a unit greater than the -4 unit(s) started by the target application`,
		`placement "2" refers to a machine not defined in this bundle`,
		`relation ["arble:bar"] has 1 endpoint(s), not 2`,
		`relation ["arble:bar" "mediawiki:db"] refers to application "arble" not defined in this bundle`,
		`relation ["mysql:foo" "mysql:bar"] relates an application to itself`,
		`relation ["mysql:db" "mediawiki:db"] is defined more than once`,
		`invalid placement syntax "bad placement"`,
		`invalid relation syntax "mediawiki/db"`,
		`invalid base "bad base" for machine "0"`,
		`ambiguous relation "riak" refers to a application and a SAAS in this bundle`,
		`SAAS "riak" already exists with application "riak" name`,
		`application "riak" already exists with SAAS "riak" name`,
	},
}, {
	about: "mediawiki should be ok",
	data:  mediawikiBundle,
}, {
	about: "malformed offer and endpoint names",
	data: `
applications:
    aws-integrator:
        charm: aws-integrator
        num_units: 1
        trust: true
        offers:
          $bad-name:
            endpoints:
              - "nope!"
`,
	errors: []string{
		`invalid offer name "$bad-name" in application "aws-integrator"`,
		`invalid endpoint name "nope!" for offer "$bad-name" in application "aws-integrator"`,
	},
}, {
	about: "expose parameters provided together with expose:true",
	data: `
applications: 
  aws-integrator: 
    charm: "aws-integrator"
    expose: true
    exposed-endpoints:
      admin:
        expose-to-spaces:
          - alpha
        expose-to-cidrs:
          - 13.37.0.0/16
    num_units: 1
`,
	errors: []string{
		`exposed-endpoints cannot be specified together with "exposed:true" in application "aws-integrator" as this poses a security risk when deploying bundles to older controllers`,
	},
}, {
	about: "invalid CIDR in expose-to-cidrs parameter when the app is exposed",
	data: `
applications: 
  aws-integrator: 
    charm: "aws-integrator"
    exposed-endpoints:
      admin:
        expose-to-spaces:
          - alpha
        expose-to-cidrs:
          - not-a-cidr
    num_units: 1
`,
	errors: []string{
		`invalid CIDR "not-a-cidr" for expose to CIDRs field for endpoint "admin" in application "aws-integrator"`,
	},
}}

func (*bundleDataSuite) TestVerifyErrors(c *tc.C) {
	for i, test := range verifyErrorsTests {
		c.Logf("test %d: %s", i, test.about)
		assertVerifyErrors(c, test.data, nil, test.errors)
	}
}

func assertVerifyErrors(c *tc.C, bundleData string, charms map[string]charm.Charm, expectErrors []string) {
	bd, err := charm.ReadBundleData(strings.NewReader(bundleData))
	c.Assert(err, tc.IsNil)

	validateConstraints := func(c string) error {
		if c == "bad constraints" {
			return fmt.Errorf("bad constraint")
		}
		return nil
	}
	validateStorage := func(c string) error {
		if c == "bad storage constraints" {
			return fmt.Errorf("bad storage constraint")
		}
		return nil
	}
	validateDevices := func(c string) error {
		if c == "bad device constraints" {
			return fmt.Errorf("bad device constraint")
		}
		return nil
	}
	if charms != nil {
		err = bd.VerifyWithCharms(validateConstraints, validateStorage, validateDevices, charms)
	} else {
		err = bd.VerifyLocal("internal/test-charm-repo/bundle", validateConstraints, validateStorage, validateDevices)
	}

	if len(expectErrors) == 0 {
		if err == nil {
			return
		}
		// Let the rest of the function deal with the
		// error, so that we'll see the actual errors
		// that resulted.
	}
	c.Assert(err, tc.FitsTypeOf, (*charm.VerificationError)(nil))
	errors := err.(*charm.VerificationError).Errors
	errStrings := make([]string, len(errors))
	for i, err := range errors {
		errStrings[i] = err.Error()
	}
	sort.Strings(errStrings)
	sort.Strings(expectErrors)
	c.Assert(errStrings, tc.DeepEquals, expectErrors)
}

func (*bundleDataSuite) TestVerifyCharmURL(c *tc.C) {
	bd, err := charm.ReadBundleData(strings.NewReader(mediawikiBundle))
	c.Assert(err, tc.IsNil)
	for i, u := range []string{
		"ch:wordpress",
		"local:foo",
		"local:foo-45",
	} {
		c.Logf("test %d: %s", i, u)
		bd.Applications["mediawiki"].Charm = u
		err := bd.Verify(nil, nil, nil)
		c.Check(err, tc.IsNil, tc.Commentf("charm url %q", u))
	}
}

func (*bundleDataSuite) TestVerifyLocalCharm(c *tc.C) {
	bd, err := charm.ReadBundleData(strings.NewReader(mediawikiBundle))
	c.Assert(err, tc.IsNil)
	bundleDir := c.MkDir()
	relativeCharmDir := filepath.Join(bundleDir, "charm")
	err = os.MkdirAll(relativeCharmDir, 0700)
	c.Assert(err, tc.ErrorIsNil)
	for i, u := range []string{
		"ch:wordpress",
		"local:foo",
		"local:foo-45",
		c.MkDir(),
		"./charm",
	} {
		c.Logf("test %d: %s", i, u)
		bd.Applications["mediawiki"].Charm = u
		err := bd.VerifyLocal(bundleDir, nil, nil, nil)
		c.Check(err, tc.IsNil, tc.Commentf("charm url %q", u))
	}
}

func (s *bundleDataSuite) TestVerifyBundleUsingJujuInfoRelation(c *tc.C) {
	err := s.testPrepareAndMutateBeforeVerifyWithCharms(c, nil)
	c.Assert(err, tc.IsNil)
}

func (s *bundleDataSuite) testPrepareAndMutateBeforeVerifyWithCharms(c *tc.C, mutator func(bd *charm.BundleData)) error {
	b := readBundleDir(c, "wordpress-with-logging")
	bd := b.Data()

	charms := map[string]charm.Charm{
		"ch:wordpress": readCharmDir(c, "wordpress"),
		"ch:mysql":     readCharmDir(c, "mysql"),
		"logging":      readCharmDir(c, "logging"),
	}

	if mutator != nil {
		mutator(bd)
	}

	return bd.VerifyWithCharms(nil, nil, nil, charms)
}

func (s *bundleDataSuite) TestVerifyBundleWithUnknownEndpointBindingGiven(c *tc.C) {
	err := s.testPrepareAndMutateBeforeVerifyWithCharms(c, func(bd *charm.BundleData) {
		bd.Applications["wordpress"].EndpointBindings["foo"] = "bar"
	})
	c.Assert(err, tc.ErrorMatches,
		`application "wordpress" wants to bind endpoint "foo" to space "bar", `+
			`but the endpoint is not defined by the charm`,
	)
}

func (s *bundleDataSuite) TestVerifyBundleWithExtraBindingsSuccess(c *tc.C) {
	err := s.testPrepareAndMutateBeforeVerifyWithCharms(c, func(bd *charm.BundleData) {
		// Both of these are specified in extra-bindings.
		bd.Applications["wordpress"].EndpointBindings["admin-api"] = "internal"
		bd.Applications["wordpress"].EndpointBindings["foo-bar"] = "test"
	})
	c.Assert(err, tc.IsNil)
}

func (s *bundleDataSuite) TestVerifyBundleWithRelationNameBindingSuccess(c *tc.C) {
	err := s.testPrepareAndMutateBeforeVerifyWithCharms(c, func(bd *charm.BundleData) {
		// Both of these are specified in as relations.
		bd.Applications["wordpress"].EndpointBindings["cache"] = "foo"
		bd.Applications["wordpress"].EndpointBindings["monitoring-port"] = "bar"
	})
	c.Assert(err, tc.IsNil)
}

func (s *bundleDataSuite) TestParseKubernetesBundleType(c *tc.C) {
	data := `
bundle: kubernetes

applications:
    mariadb:
        charm: "mariadb-k8s"
        scale: 2
        placement: foo=bar
    gitlab:
        charm: "gitlab-k8s"
        num_units: 3
        to: [foo=baz]
    redis:
        charm: "redis-k8s"
        scale: 3
        to: [foo=baz]
`
	bd, err := charm.ReadBundleData(strings.NewReader(data))
	c.Assert(err, tc.IsNil)
	err = bd.Verify(nil, nil, nil)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(bd, tc.DeepEquals, &charm.BundleData{
		Type: "kubernetes",
		Applications: map[string]*charm.ApplicationSpec{
			"mariadb": {
				Charm:    "mariadb-k8s",
				To:       []string{"foo=bar"},
				NumUnits: 2,
			},
			"gitlab": {
				Charm:    "gitlab-k8s",
				To:       []string{"foo=baz"},
				NumUnits: 3,
			},
			"redis": {
				Charm:    "redis-k8s",
				To:       []string{"foo=baz"},
				NumUnits: 3,
			}},
	})
}

func (s *bundleDataSuite) TestInvalidBundleType(c *tc.C) {
	data := `
bundle: foo

applications:
    mariadb:
        charm: mariadb-k8s
        scale: 2
`
	bd, err := charm.ReadBundleData(strings.NewReader(data))
	c.Assert(err, tc.IsNil)
	err = bd.Verify(nil, nil, nil)
	c.Assert(err, tc.ErrorMatches, `bundle has an invalid type "foo"`)
}

func (s *bundleDataSuite) TestInvalidScaleAndNumUnits(c *tc.C) {
	data := `
bundle: kubernetes

applications:
    mariadb:
        charm: "mariadb-k8s"
        scale: 2
        num_units: 2
`
	_, err := charm.ReadBundleData(strings.NewReader(data))
	c.Assert(err, tc.ErrorMatches, `.*cannot specify both scale and num_units for application "mariadb"`)
}

func (s *bundleDataSuite) TestInvalidPlacementAndTo(c *tc.C) {
	data := `
bundle: kubernetes

applications:
    mariadb:
        charm: "mariadb-k8s"
        placement: foo=bar
        to: [foo=bar]
`
	_, err := charm.ReadBundleData(strings.NewReader(data))
	c.Assert(err, tc.ErrorMatches, `.*cannot specify both placement and to for application "mariadb"`)
}

func (s *bundleDataSuite) TestInvalidIAASPlacement(c *tc.C) {
	data := `
applications:
    mariadb:
        charm: "mariadb"
        placement: foo=bar
`
	_, err := charm.ReadBundleData(strings.NewReader(data))
	c.Assert(err, tc.ErrorMatches, `.*placement \(foo=bar\) not valid for non-Kubernetes application "mariadb"`)
}

func (s *bundleDataSuite) TestKubernetesBundleErrors(c *tc.C) {
	data := `
bundle: "kubernetes"

machines:
    0:

applications:
    mariadb:
        charm: "mariadb-k8s"
        scale: 2
    casandra:
        charm: "casnadra-k8s"
        to: ["foo=bar", "foo=baz"]
    hadoop:
        charm: "hadoop-k8s"
        to: ["foo"]
`
	errors := []string{
		`expected "key=value", got "foo" for application "hadoop"`,
		`bundle machines not valid for Kubernetes bundles`,
		`too many placement directives for application "casandra"`,
	}

	assertVerifyErrors(c, data, nil, errors)
}

func (*bundleDataSuite) TestRequiredCharms(c *tc.C) {
	bd, err := charm.ReadBundleData(strings.NewReader(mediawikiBundle))
	c.Assert(err, tc.IsNil)
	reqCharms := bd.RequiredCharms()

	c.Assert(reqCharms, tc.DeepEquals, []string{"mediawiki", "mysql"})
}

// testCharm returns a charm with the given name
// and relations. The relations are specified as
// a string of the form:
//
//	<provides-relations> | <requires-relations>
//
// Within each section, each white-space separated
// relation is specified as:
// /	<relation-name>:<interface>
//
// So, for example:
//
//	testCharm("wordpress", "web:http | db:mysql")
//
// is equivalent to a charm with metadata.yaml containing
//
//	name: wordpress
//	description: wordpress
//	provides:
//	    web:
//	        interface: http
//	requires:
//	    db:
//	        interface: mysql
//
// If the charm name has a "-sub" suffix, the
// returned charm will have Meta.Subordinate = true.
func testCharm(name string, relations string) charm.Charm {
	var provides, requires string
	parts := strings.Split(relations, "|")
	provides = parts[0]
	if len(parts) > 1 {
		requires = parts[1]
	}
	meta := &charm.Meta{
		Name:        name,
		Summary:     name,
		Description: name,
		Provides:    parseRelations(provides, charm.RoleProvider),
		Requires:    parseRelations(requires, charm.RoleRequirer),
	}
	if strings.HasSuffix(name, "-sub") {
		meta.Subordinate = true
	}
	configStr := `
options:
  title: {default: My Title, description: title, type: string}
  skill-level: {description: skill, type: int}
`
	config, err := charm.ReadConfig(strings.NewReader(configStr))
	if err != nil {
		panic(err)
	}
	return testCharmImpl{
		meta:   meta,
		config: config,
	}
}

func parseRelations(s string, role charm.RelationRole) map[string]charm.Relation {
	rels := make(map[string]charm.Relation)
	for _, r := range strings.Fields(s) {
		parts := strings.Split(r, ":")
		if len(parts) != 2 {
			panic(fmt.Errorf("invalid relation specifier %q", r))
		}
		name, interf := parts[0], parts[1]
		rels[name] = charm.Relation{
			Name:      name,
			Role:      role,
			Interface: interf,
			Scope:     charm.ScopeGlobal,
		}
	}
	return rels
}

type testCharmImpl struct {
	meta   *charm.Meta
	config *charm.Config
	// Implement charm.Charm, but panic if anything other than
	// Meta or Config methods are called.
	charm.Charm
}

func (c testCharmImpl) Meta() *charm.Meta {
	return c.meta
}

func (c testCharmImpl) Config() *charm.Config {
	return c.config
}

var verifyWithCharmsErrorsTests = []struct {
	about  string
	data   string
	charms map[string]charm.Charm

	errors []string
}{{
	about:  "no charms",
	data:   mediawikiBundle,
	charms: map[string]charm.Charm{},
	errors: []string{
		`application "mediawiki" refers to non-existent charm "mediawiki"`,
		`application "mysql" refers to non-existent charm "mysql"`,
	},
}, {
	about: "all present and correct",
	data: `
applications:
    application1:
        charm: "test"
    application2:
        charm: "test"
    application3:
        charm: "test"
relations:
    - ["application1:prova", "application2:reqa"]
    - ["application1:reqa", "application3:prova"]
    - ["application3:provb", "application2:reqb"]
`,
	charms: map[string]charm.Charm{
		"test": testCharm("test", "prova:a provb:b | reqa:a reqb:b"),
	},
}, {
	about: "undefined relations",
	data: `
applications:
    application1:
        charm: "test"
    application2:
        charm: "test"
relations:
    - ["application1:prova", "application2:blah"]
    - ["application1:blah", "application2:prova"]
`,
	charms: map[string]charm.Charm{
		"test": testCharm("test", "prova:a provb:b | reqa:a reqb:b"),
	},
	errors: []string{
		`charm "test" used by application "application1" does not define relation "blah"`,
		`charm "test" used by application "application2" does not define relation "blah"`,
	},
}, {
	about: "undefined applications",
	data: `
applications:
    application1:
        charm: "test"
    application2:
        charm: "test"
relations:
    - ["unknown:prova", "application2:blah"]
    - ["application1:blah", "unknown:prova"]
`,
	charms: map[string]charm.Charm{
		"test": testCharm("test", "prova:a provb:b | reqa:a reqb:b"),
	},
	errors: []string{
		`relation ["application1:blah" "unknown:prova"] refers to application "unknown" not defined in this bundle`,
		`relation ["unknown:prova" "application2:blah"] refers to application "unknown" not defined in this bundle`,
	},
}, {
	about: "equal applications",
	data: `
applications:
    application1:
        charm: "test"
    application2:
        charm: "test"
relations:
    - ["application2:prova", "application2:reqa"]
`,
	charms: map[string]charm.Charm{
		"test": testCharm("test", "prova:a provb:b | reqa:a reqb:b"),
	},
	errors: []string{
		`relation ["application2:prova" "application2:reqa"] relates an application to itself`,
	},
}, {
	about: "provider to provider relation",
	data: `
applications:
    application1:
        charm: "test"
    application2:
        charm: "test"
relations:
    - ["application1:prova", "application2:prova"]
`,
	charms: map[string]charm.Charm{
		"test": testCharm("test", "prova:a provb:b | reqa:a reqb:b"),
	},
	errors: []string{
		`relation "application1:prova" to "application2:prova" relates provider to provider`,
	},
}, {
	about: "provider to provider relation",
	data: `
applications:
    application1:
        charm: "test"
    application2:
        charm: "test"
relations:
    - ["application1:reqa", "application2:reqa"]
`,
	charms: map[string]charm.Charm{
		"test": testCharm("test", "prova:a provb:b | reqa:a reqb:b"),
	},
	errors: []string{
		`relation "application1:reqa" to "application2:reqa" relates requirer to requirer`,
	},
}, {
	about: "interface mismatch",
	data: `
applications:
    application1:
        charm: "test"
    application2:
        charm: "test"
relations:
    - ["application1:reqa", "application2:provb"]
`,
	charms: map[string]charm.Charm{
		"test": testCharm("test", "prova:a provb:b | reqa:a reqb:b"),
	},
	errors: []string{
		`mismatched interface between "application2:provb" and "application1:reqa" ("b" vs "a")`,
	},
}, {
	about: "different charms",
	data: `
applications:
    application1:
        charm: "test1"
    application2:
        charm: "test2"
relations:
    - ["application1:reqa", "application2:prova"]
`,
	charms: map[string]charm.Charm{
		"test1": testCharm("test", "prova:a provb:b | reqa:a reqb:b"),
		"test2": testCharm("test", ""),
	},
	errors: []string{
		`charm "test2" used by application "application2" does not define relation "prova"`,
	},
}, {
	about: "ambiguous relation",
	data: `
applications:
    application1:
        charm: "test1"
    application2:
        charm: "test2"
relations:
    - [application1, application2]
`,
	charms: map[string]charm.Charm{
		"test1": testCharm("test", "prova:a provb:b | reqa:a reqb:b"),
		"test2": testCharm("test", "prova:a provb:b | reqa:a reqb:b"),
	},
	errors: []string{
		`cannot infer endpoint between application1 and application2: ambiguous relation: application1 application2 could refer to "application1:prova application2:reqa"; "application1:provb application2:reqb"; "application1:reqa application2:prova"; "application1:reqb application2:provb"`,
	},
}, {
	about: "relation using juju-info",
	data: `
applications:
    application1:
        charm: "provider"
    application2:
        charm: "requirer"
relations:
    - [application1, application2]
`,
	charms: map[string]charm.Charm{
		"provider": testCharm("provider", ""),
		"requirer": testCharm("requirer", "| req:juju-info"),
	},
}, {
	about: "ambiguous when implicit relations taken into account",
	data: `
applications:
    application1:
        charm: "provider"
    application2:
        charm: "requirer"
relations:
    - [application1, application2]
`,
	charms: map[string]charm.Charm{
		"provider": testCharm("provider", "provdb:db | "),
		"requirer": testCharm("requirer", "| reqdb:db reqinfo:juju-info"),
	},
}, {
	about: "half of relation left open",
	data: `
applications:
    application1:
        charm: "provider"
    application2:
        charm: "requirer"
relations:
    - ["application1:prova2", application2]
`,
	charms: map[string]charm.Charm{
		"provider": testCharm("provider", "prova1:a prova2:a | "),
		"requirer": testCharm("requirer", "| reqa:a"),
	},
}, {
	about: "duplicate relation between open and fully-specified relations",
	data: `
applications:
    application1:
        charm: "provider"
    application2:
        charm: "requirer"
relations:
    - ["application1:prova", "application2:reqa"]
    - ["application1", "application2"]
`,
	charms: map[string]charm.Charm{
		"provider": testCharm("provider", "prova:a | "),
		"requirer": testCharm("requirer", "| reqa:a"),
	},
	errors: []string{
		`relation ["application1" "application2"] is defined more than once`,
	},
}, {
	about: "configuration options specified",
	data: `
applications:
    application1:
        charm: "test"
        options:
            title: "some title"
            skill-level: 245
    application2:
        charm: "test"
        options:
            title: "another title"
`,
	charms: map[string]charm.Charm{
		"test": testCharm("test", "prova:a provb:b | reqa:a reqb:b"),
	},
}, {
	about: "invalid type for option",
	data: `
applications:
    application1:
        charm: "test"
        options:
            title: "some title"
            skill-level: "too much"
    application2:
        charm: "test"
        options:
            title: "another title"
`,
	charms: map[string]charm.Charm{
		"test": testCharm("test", "prova:a provb:b | reqa:a reqb:b"),
	},
	errors: []string{
		`cannot validate application "application1": option "skill-level" expected int, got "too much"`,
	},
}, {
	about: "unknown option",
	data: `
applications:
    application1:
        charm: "test"
        options:
            title: "some title"
            unknown-option: 2345
`,
	charms: map[string]charm.Charm{
		"test": testCharm("test", "prova:a provb:b | reqa:a reqb:b"),
	},
	errors: []string{
		`cannot validate application "application1": configuration option "unknown-option" not found in charm "test"`,
	},
}, {
	about: "multiple config problems",
	data: `
applications:
    application1:
        charm: "test"
        options:
            title: "some title"
            unknown-option: 2345
    application2:
        charm: "test"
        options:
            title: 123
            another-unknown: 2345
`,
	charms: map[string]charm.Charm{
		"test": testCharm("test", "prova:a provb:b | reqa:a reqb:b"),
	},
	errors: []string{
		`cannot validate application "application1": configuration option "unknown-option" not found in charm "test"`,
		`cannot validate application "application2": configuration option "another-unknown" not found in charm "test"`,
		`cannot validate application "application2": option "title" expected string, got 123`,
	},
}, {
	about: "subordinate charm with more than zero units",
	data: `
applications:
    testsub:
        charm: "testsub"
        num_units: 1
`,
	charms: map[string]charm.Charm{
		"testsub": testCharm("test-sub", ""),
	},
	errors: []string{
		`application "testsub" is subordinate but has non-zero num_units`,
	},
}, {
	about: "subordinate charm with more than one unit",
	data: `
applications:
    testsub:
        charm: "testsub"
        num_units: 1
`,
	charms: map[string]charm.Charm{
		"testsub": testCharm("test-sub", ""),
	},
	errors: []string{
		`application "testsub" is subordinate but has non-zero num_units`,
	},
}, {
	about: "subordinate charm with to-clause",
	data: `
applications:
    testsub:
        charm: "testsub"
        to: [0]
machines:
    0:
`,
	charms: map[string]charm.Charm{
		"testsub": testCharm("test-sub", ""),
	},
	errors: []string{
		`application "testsub" is subordinate but specifies unit placement`,
		`too many units specified in unit placement for application "testsub"`,
	},
}, {
	about: "charm with unspecified units and more than one to: entry",
	data: `
applications:
    test:
        charm: "test"
        to: [0, 1]
machines:
    0:
    1:
`,
	errors: []string{
		`too many units specified in unit placement for application "test"`,
	},
}, {
	about: "charmhub charm revision and no channel",
	data: `
applications:
    wordpress:
      charm: "wordpress"
      revision: 5
      num_units: 1
`,
	errors: []string{
		`application "wordpress" with a revision requires a channel for future upgrades, please use channel`,
	},
}, {
	about: "charmhub charm revision in charm url",
	data: `
applications:
    wordpress:
      charm: "wordpress-9"
      num_units: 1
`,
	errors: []string{
		`cannot specify revision in "ch:wordpress-9", please use revision`,
	},
}, {
	about: "charmstore charm url revision value less than 0",
	data: `
applications:
    wordpress:
      charm: "wordpress"
      revision: -5
      channel: edge
      num_units: 1
`,
	errors: []string{
		`the revision for application "wordpress" must be zero or greater`,
	},
}}

func (*bundleDataSuite) TestVerifyWithCharmsErrors(c *tc.C) {
	for i, test := range verifyWithCharmsErrorsTests {
		c.Logf("test %d: %s", i, test.about)
		assertVerifyErrors(c, test.data, test.charms, test.errors)
	}
}

var parsePlacementTests = []struct {
	placement string
	expect    *charm.UnitPlacement
	expectErr string
}{{
	placement: "lxc:application/0",
	expect: &charm.UnitPlacement{
		ContainerType: "lxc",
		Application:   "application",
		Unit:          0,
	},
}, {
	placement: "lxc:application",
	expect: &charm.UnitPlacement{
		ContainerType: "lxc",
		Application:   "application",
		Unit:          -1,
	},
}, {
	placement: "lxc:99",
	expect: &charm.UnitPlacement{
		ContainerType: "lxc",
		Machine:       "99",
		Unit:          -1,
	},
}, {
	placement: "lxc:new",
	expect: &charm.UnitPlacement{
		ContainerType: "lxc",
		Machine:       "new",
		Unit:          -1,
	},
}, {
	placement: "application/0",
	expect: &charm.UnitPlacement{
		Application: "application",
		Unit:        0,
	},
}, {
	placement: "application",
	expect: &charm.UnitPlacement{
		Application: "application",
		Unit:        -1,
	},
}, {
	placement: "application45",
	expect: &charm.UnitPlacement{
		Application: "application45",
		Unit:        -1,
	},
}, {
	placement: "99",
	expect: &charm.UnitPlacement{
		Machine: "99",
		Unit:    -1,
	},
}, {
	placement: "new",
	expect: &charm.UnitPlacement{
		Machine: "new",
		Unit:    -1,
	},
}, {
	placement: ":0",
	expectErr: `invalid placement syntax ":0"`,
}, {
	placement: "05",
	expectErr: `invalid placement syntax "05"`,
}, {
	placement: "new/2",
	expectErr: `invalid placement syntax "new/2"`,
}}

func (*bundleDataSuite) TestParsePlacement(c *tc.C) {
	for i, test := range parsePlacementTests {
		c.Logf("test %d: %q", i, test.placement)
		up, err := charm.ParsePlacement(test.placement)
		if test.expectErr != "" {
			c.Assert(err, tc.ErrorMatches, test.expectErr)
		} else {
			c.Assert(err, tc.IsNil)
			c.Assert(up, tc.DeepEquals, test.expect)
		}
	}
}

// Tests that empty/nil applications cause an error
func (*bundleDataSuite) TestApplicationEmpty(c *tc.C) {
	tstDatas := []string{
		`
applications:
    application1:
    application2:
        charm: "test"
        plan: "testisv/test2"
`,
		`
applications:
    application1:
        charm: "test"
        plan: "testisv/test2"
    application2:
`,
		`
applications:
    application1:
        charm: "test"
        plan: "testisv/test2"
    application2: ~
`,
	}

	for _, d := range tstDatas {
		bd, err := charm.ReadBundleData(strings.NewReader(d))
		c.Assert(err, tc.IsNil)

		err = bd.Verify(nil, nil, nil)
		c.Assert(err, tc.ErrorMatches, "bundle application for key .+ is undefined")
	}
}

func (*bundleDataSuite) TestApplicationPlans(c *tc.C) {
	data := `
applications:
    application1:
        charm: "test"
        plan: "testisv/test"
    application2:
        charm: "test"
        plan: "testisv/test2"
    application3:
        charm: "test"
        plan: "default"
relations:
    - ["application1:prova", "application2:reqa"]
    - ["application1:reqa", "application3:prova"]
    - ["application3:provb", "application2:reqb"]
`

	bd, err := charm.ReadBundleData(strings.NewReader(data))
	c.Assert(err, tc.IsNil)

	c.Assert(bd.Applications, tc.DeepEquals, map[string]*charm.ApplicationSpec{
		"application1": {
			Charm: "test",
			Plan:  "testisv/test",
		},
		"application2": {
			Charm: "test",
			Plan:  "testisv/test2",
		},
		"application3": {
			Charm: "test",
			Plan:  "default",
		},
	})

}
