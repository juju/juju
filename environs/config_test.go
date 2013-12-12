// Copyright 2011, 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package environs_test

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	gc "launchpad.net/gocheck"
	"launchpad.net/loggo"

	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/provider/dummy"
	"launchpad.net/juju-core/testing"
	jc "launchpad.net/juju-core/testing/checkers"
	"launchpad.net/juju-core/testing/testbase"
)

type suite struct {
	testbase.LoggingSuite
}

var _ = gc.Suite(&suite{})

func (s *suite) TearDownTest(c *gc.C) {
	dummy.Reset()
	s.LoggingSuite.TearDownTest(c)
}

var invalidConfigTests = []struct {
	env string
	err string
}{
	{"'", "YAML error:.*"},
	{`
default: unknown
environments:
    only:
        type: unknown
`, `default environment .* does not exist`,
	},
}

func (*suite) TestInvalidConfig(c *gc.C) {
	for i, t := range invalidConfigTests {
		c.Logf("running test %v", i)
		_, err := environs.ReadEnvironsBytes([]byte(t.env))
		c.Check(err, gc.ErrorMatches, t.err)
	}
}

var invalidEnvTests = []struct {
	env  string
	name string
	err  string
}{
	{`
environments:
    only:
        foo: bar
`, "", `environment "only" has no type`,
	}, {`
environments:
    only:
        foo: bar
`, "only", `environment "only" has no type`,
	}, {`
environments:
    only:
        foo: bar
        type: crazy
`, "only", `environment "only" has an unknown provider type "crazy"`,
	},
}

func (*suite) TestInvalidEnv(c *gc.C) {
	defer testing.MakeFakeHomeNoEnvironments(c, "only").Restore()
	for i, t := range invalidEnvTests {
		c.Logf("running test %v", i)
		es, err := environs.ReadEnvironsBytes([]byte(t.env))
		c.Check(err, gc.IsNil)
		cfg, err := es.Config(t.name)
		c.Check(err, gc.ErrorMatches, t.err)
		c.Check(cfg, gc.IsNil)
	}
}

func (*suite) TestNoHomeBeforeConfig(c *gc.C) {
	// Test that we don't actually need HOME set until we call envs.Config()
	// Because of this, we intentionally do *not* call testing.MakeFakeHomeNoEnvironments()
	content := `
environments:
    valid:
        type: dummy
    amazon:
        type: ec2
`
	_, err := environs.ReadEnvironsBytes([]byte(content))
	c.Check(err, gc.IsNil)
}

func (*suite) TestNoEnv(c *gc.C) {
	defer testing.MakeFakeHomeNoEnvironments(c).Restore()
	es, err := environs.ReadEnvirons("")
	c.Assert(es, gc.IsNil)
	c.Assert(err, jc.Satisfies, environs.IsNoEnv)
}

var configTests = []struct {
	env   string
	check func(c *gc.C, envs *environs.Environs)
}{
	{`
environments:
    only:
        type: dummy
        state-server: false
`, func(c *gc.C, envs *environs.Environs) {
		cfg, err := envs.Config("")
		c.Assert(err, gc.IsNil)
		c.Assert(cfg.Name(), gc.Equals, "only")
	}}, {`
default:
    invalid
environments:
    valid:
        type: dummy
        state-server: false
    invalid:
        type: crazy
`, func(c *gc.C, envs *environs.Environs) {
		cfg, err := envs.Config("")
		c.Assert(err, gc.ErrorMatches, `environment "invalid" has an unknown provider type "crazy"`)
		c.Assert(cfg, gc.IsNil)
		cfg, err = envs.Config("valid")
		c.Assert(err, gc.IsNil)
		c.Assert(cfg.Name(), gc.Equals, "valid")
	}}, {`
environments:
    one:
        type: dummy
        state-server: false
    two:
        type: dummy
        state-server: false
`, func(c *gc.C, envs *environs.Environs) {
		cfg, err := envs.Config("")
		c.Assert(err, gc.ErrorMatches, `no default environment found`)
		c.Assert(cfg, gc.IsNil)
	}},
}

func (*suite) TestConfig(c *gc.C) {
	defer testing.MakeFakeHomeNoEnvironments(c, "only", "valid", "one", "two").Restore()
	for i, t := range configTests {
		c.Logf("running test %v", i)
		envs, err := environs.ReadEnvironsBytes([]byte(t.env))
		c.Assert(err, gc.IsNil)
		t.check(c, envs)
	}
}

func (*suite) TestDefaultConfigFile(c *gc.C) {
	defer testing.MakeEmptyFakeHome(c).Restore()

	env := `
environments:
    only:
        type: dummy
        state-server: false
        authorized-keys: i-am-a-key
`
	outfile, err := environs.WriteEnvirons("", env)
	c.Assert(err, gc.IsNil)
	path := testing.HomePath(".juju", "environments.yaml")
	c.Assert(path, gc.Equals, outfile)

	envs, err := environs.ReadEnvirons("")
	c.Assert(err, gc.IsNil)
	cfg, err := envs.Config("")
	c.Assert(err, gc.IsNil)
	c.Assert(cfg.Name(), gc.Equals, "only")
}

func (*suite) TestConfigPerm(c *gc.C) {
	defer testing.MakeSampleHome(c).Restore()

	path := testing.HomePath(".juju")
	info, err := os.Lstat(path)
	c.Assert(err, gc.IsNil)
	oldPerm := info.Mode().Perm()
	env := `
environments:
    only:
        type: dummy
        state-server: false
        authorized-keys: i-am-a-key
`
	outfile, err := environs.WriteEnvirons("", env)
	c.Assert(err, gc.IsNil)

	info, err = os.Lstat(outfile)
	c.Assert(err, gc.IsNil)
	c.Assert(info.Mode().Perm(), gc.Equals, os.FileMode(0600))

	info, err = os.Lstat(filepath.Dir(outfile))
	c.Assert(err, gc.IsNil)
	c.Assert(info.Mode().Perm(), gc.Equals, oldPerm)

}

func (*suite) TestNamedConfigFile(c *gc.C) {
	defer testing.MakeFakeHomeNoEnvironments(c, "only").Restore()

	env := `
environments:
    only:
        type: dummy
        state-server: false
        authorized-keys: i-am-a-key
`
	path := filepath.Join(c.MkDir(), "a-file")
	outfile, err := environs.WriteEnvirons(path, env)
	c.Assert(err, gc.IsNil)
	c.Assert(path, gc.Equals, outfile)

	envs, err := environs.ReadEnvirons(path)
	c.Assert(err, gc.IsNil)
	cfg, err := envs.Config("")
	c.Assert(err, gc.IsNil)
	c.Assert(cfg.Name(), gc.Equals, "only")
}

func inMap(attrs testing.Attrs, attr string) bool {
	_, ok := attrs[attr]
	return ok
}

func (*suite) TestBootstrapConfig(c *gc.C) {
	defer testing.MakeFakeHomeNoEnvironments(c, "bladaam").Restore()
	attrs := dummySampleConfig().Merge(testing.Attrs{
		"agent-version": "1.2.3",
	})
	c.Assert(inMap(attrs, "secret"), jc.IsTrue)
	c.Assert(inMap(attrs, "ca-private-key"), jc.IsTrue)
	c.Assert(inMap(attrs, "admin-secret"), jc.IsTrue)

	cfg, err := config.New(config.NoDefaults, attrs)
	c.Assert(err, gc.IsNil)
	c.Assert(err, gc.IsNil)

	cfg1, err := environs.BootstrapConfig(cfg)
	c.Assert(err, gc.IsNil)

	expect := cfg.AllAttrs()
	delete(expect, "secret")
	expect["admin-secret"] = ""
	expect["ca-private-key"] = ""
	c.Assert(cfg1.AllAttrs(), gc.DeepEquals, expect)
}

type ConfigDeprecationSuite struct {
	suite
	writer *loggo.TestWriter
}

var _ = gc.Suite(&ConfigDeprecationSuite{})

func (s *ConfigDeprecationSuite) setupLogger(c *gc.C) func() {
	var err error
	s.writer = &loggo.TestWriter{}
	err = loggo.RegisterWriter("test", s.writer, loggo.WARNING)
	c.Assert(err, gc.IsNil)
	return func() {
		_, _, err := loggo.RemoveWriter("test")
		c.Assert(err, gc.IsNil)
	}
}

func (s *ConfigDeprecationSuite) TestDeprecationWarnings(c *gc.C) {
	defer testing.MakeFakeHomeNoEnvironments(c, "only").Restore()
	content := `
environments:
    deprecated:
        type: dummy
        state-server: false
`
	for _, attr := range []string{
		"tools-url",
	} {
		restore := s.setupLogger(c)
		defer restore()

		envs, err := environs.ReadEnvironsBytes([]byte(content))
		c.Check(err, gc.IsNil)
		environs.UpdateEnvironAttrs(envs, "deprecated", testing.Attrs{
			attr: "aknowndeprecatedfield",
		})
		_, err = envs.Config("deprecated")
		c.Check(err, gc.IsNil)
		c.Assert(s.writer.Log, gc.HasLen, 1)
		stripped := strings.Replace(s.writer.Log[0].Message, "\n", "", -1)
		expected := fmt.Sprintf(`.*Config attribute "%s" \(aknowndeprecatedfield\) is deprecated.*`, attr)
		c.Assert(stripped, gc.Matches, expected)
	}
}

type configTest struct {
	about string
	attrs map[string]interface{}
}

var toolsURLTests = []configTest{
	{
		about: "No tools urls used",
		attrs: testing.Attrs{},
	}, {
		about: "Deprecated tools metadata URL used",
		attrs: testing.Attrs{
			"tools-url": "tools-metadata-url-value",
		},
	}, {
		about: "Deprecated tools metadata URL ignored",
		attrs: testing.Attrs{
			"tools-metadata-url": "tools-metadata-url-value",
			"tools-url":          "ignore-me",
		},
	},
}

func (s *ConfigDeprecationSuite) TestToolsURLDeprecation(c *gc.C) {
	defer testing.MakeFakeHomeNoEnvironments(c, "only").Restore()
	content := `
environments:
    deprecated:
        type: dummy
        state-server: false
`
	for i, test := range toolsURLTests {
		c.Logf("test %d. %s", i, test.about)

		envs, err := environs.ReadEnvironsBytes([]byte(content))
		c.Check(err, gc.IsNil)
		environs.UpdateEnvironAttrs(envs, "deprecated", test.attrs)
		cfg, err := envs.Config("deprecated")
		c.Check(err, gc.IsNil)
		toolsURL, urlPresent := cfg.ToolsURL()
		oldToolsURL, oldURLPresent := cfg.AllAttrs()["tools-url"]
		oldToolsURLAttrValue, oldURLAttrPresent := test.attrs["tools-url"]
		expectedToolsURLValue := test.attrs["tools-metadata-url"]
		if expectedToolsURLValue == nil {
			expectedToolsURLValue = oldToolsURLAttrValue
		}
		if expectedToolsURLValue != nil && expectedToolsURLValue != "" {
			c.Assert(expectedToolsURLValue, gc.Equals, "tools-metadata-url-value")
			c.Assert(toolsURL, gc.Equals, expectedToolsURLValue)
			c.Assert(urlPresent, jc.IsTrue)
			c.Assert(oldToolsURL, gc.Equals, expectedToolsURLValue)
			c.Assert(oldURLPresent, jc.IsTrue)
		} else {
			c.Assert(urlPresent, jc.IsFalse)
			c.Assert(oldURLAttrPresent, jc.IsFalse)
			c.Assert(oldToolsURL, gc.Equals, "")
		}
	}
}

func (s *ConfigDeprecationSuite) TestNoWarningForDeprecatedButUnusedEnv(c *gc.C) {
	// This tests that a config that has a deprecated field doesn't
	// generate a Warning if we don't actually ask for that environment.
	// However, we can only really trigger that when we have a deprecated
	// field. If support for the field is removed entirely, another
	// mechanism will need to be used
	defer testing.MakeFakeHomeNoEnvironments(c, "only").Restore()
	content := `
environments:
    valid:
        type: dummy
        state-server: false
    deprecated:
        type: dummy
        state-server: false
        tools-url: aknowndeprecatedfield
`
	restore := s.setupLogger(c)
	defer restore()

	envs, err := environs.ReadEnvironsBytes([]byte(content))
	c.Check(err, gc.IsNil)
	names := envs.Names()
	sort.Strings(names)
	c.Check(names, gc.DeepEquals, []string{"deprecated", "valid"})
	// There should be no warning in the log
	c.Check(s.writer.Log, gc.HasLen, 0)
	// Now we actually grab the 'valid' entry
	_, err = envs.Config("valid")
	c.Check(err, gc.IsNil)
	// And still we have no warnings
	c.Check(s.writer.Log, gc.HasLen, 0)
	// Only once we grab the deprecated one do we see any warnings
	_, err = envs.Config("deprecated")
	c.Check(err, gc.IsNil)
	c.Check(s.writer.Log, gc.HasLen, 1)
}
