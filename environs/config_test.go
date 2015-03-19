// Copyright 2011, 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package environs_test

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/juju/loggo"
	gitjujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/provider/dummy"
	_ "github.com/juju/juju/provider/manual"
	"github.com/juju/juju/testing"
)

type suite struct {
	testing.FakeJujuHomeSuite
}

var _ = gc.Suite(&suite{})

func (s *suite) TearDownTest(c *gc.C) {
	dummy.Reset()
	s.FakeJujuHomeSuite.TearDownTest(c)
}

// dummySampleConfig returns the dummy sample config without
// the state server configured.
// This function also exists in cloudconfig/userdata_test
// Maybe place it in dummy and export it?
func dummySampleConfig() testing.Attrs {
	return dummy.SampleConfig().Merge(testing.Attrs{
		"state-server": false,
	})
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
	for i, t := range invalidEnvTests {
		c.Logf("running test %v", i)
		es, err := environs.ReadEnvironsBytes([]byte(t.env))
		c.Check(err, jc.ErrorIsNil)
		cfg, err := es.Config(t.name)
		c.Check(err, gc.ErrorMatches, t.err)
		c.Check(cfg, gc.IsNil)
	}
}

func (*suite) TestNoWarningForDeprecatedButUnusedEnv(c *gc.C) {
	// This tests that a config that has a deprecated field doesn't
	// generate a Warning if we don't actually ask for that environment.
	// However, we can only really trigger that when we have a deprecated
	// field. If support for the field is removed entirely, another
	// mechanism will need to be used
	content := `
environments:
    valid:
        type: dummy
        state-server: false
    deprecated:
        type: dummy
        state-server: false
        tools-metadata-url: aknowndeprecatedfield
        lxc-use-clone: true
`
	var tw loggo.TestWriter
	// we only capture Warning or above
	c.Assert(loggo.RegisterWriter("invalid-env-tester", &tw, loggo.WARNING), gc.IsNil)
	defer loggo.RemoveWriter("invalid-env-tester")

	envs, err := environs.ReadEnvironsBytes([]byte(content))
	c.Check(err, jc.ErrorIsNil)
	names := envs.Names()
	sort.Strings(names)
	c.Check(names, gc.DeepEquals, []string{"deprecated", "valid"})
	// There should be no warning in the log
	c.Check(tw.Log(), gc.HasLen, 0)
	// Now we actually grab the 'valid' entry
	_, err = envs.Config("valid")
	c.Check(err, jc.ErrorIsNil)
	// And still we have no warnings
	c.Check(tw.Log(), gc.HasLen, 0)
	// Only once we grab the deprecated one do we see any warnings
	_, err = envs.Config("deprecated")
	c.Check(err, jc.ErrorIsNil)
	c.Check(tw.Log(), gc.HasLen, 2)
}

func (*suite) TestNoHomeBeforeConfig(c *gc.C) {
	// Test that we don't actually need HOME set until we call envs.Config()
	os.Setenv("HOME", "")
	content := `
environments:
    valid:
        type: dummy
    amazon:
        type: ec2
`
	_, err := environs.ReadEnvironsBytes([]byte(content))
	c.Check(err, jc.ErrorIsNil)
}

func (*suite) TestNoEnv(c *gc.C) {
	envPath := gitjujutesting.HomePath(".juju", "environments.yaml")
	err := os.Remove(envPath)
	c.Assert(err, jc.ErrorIsNil)
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
		c.Assert(err, jc.ErrorIsNil)
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
		c.Assert(err, jc.ErrorIsNil)
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
	for i, t := range configTests {
		c.Logf("running test %v", i)
		envs, err := environs.ReadEnvironsBytes([]byte(t.env))
		c.Assert(err, jc.ErrorIsNil)
		t.check(c, envs)
	}
}

func (*suite) TestDefaultConfigFile(c *gc.C) {
	env := `
environments:
    only:
        type: dummy
        state-server: false
        authorized-keys: i-am-a-key
`
	outfile, err := environs.WriteEnvirons("", env)
	c.Assert(err, jc.ErrorIsNil)
	path := gitjujutesting.HomePath(".juju", "environments.yaml")
	c.Assert(path, gc.Equals, outfile)

	envs, err := environs.ReadEnvirons("")
	c.Assert(err, jc.ErrorIsNil)
	cfg, err := envs.Config("")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cfg.Name(), gc.Equals, "only")
}

func (s *suite) TestConfigPerm(c *gc.C) {
	testing.MakeSampleJujuHome(c)

	path := gitjujutesting.HomePath(".juju")
	info, err := os.Lstat(path)
	c.Assert(err, jc.ErrorIsNil)
	oldPerm := info.Mode().Perm()
	env := `
environments:
    only:
        type: dummy
        state-server: false
        authorized-keys: i-am-a-key
`
	outfile, err := environs.WriteEnvirons("", env)
	c.Assert(err, jc.ErrorIsNil)

	info, err = os.Lstat(outfile)
	c.Assert(err, jc.ErrorIsNil)
	// Windows is not fully POSIX compliant. Normal permission
	// checking will yield unexpected results
	if runtime.GOOS != "windows" {
		c.Assert(info.Mode().Perm(), gc.Equals, os.FileMode(0600))
	}

	info, err = os.Lstat(filepath.Dir(outfile))
	c.Assert(err, jc.ErrorIsNil)
	if runtime.GOOS != "windows" {
		c.Assert(info.Mode().Perm(), gc.Equals, oldPerm)
	}

}

func (*suite) TestNamedConfigFile(c *gc.C) {

	env := `
environments:
    only:
        type: dummy
        state-server: false
        authorized-keys: i-am-a-key
`
	path := filepath.Join(c.MkDir(), "a-file")
	outfile, err := environs.WriteEnvirons(path, env)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(path, gc.Equals, outfile)

	envs, err := environs.ReadEnvirons(path)
	c.Assert(err, jc.ErrorIsNil)
	cfg, err := envs.Config("")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cfg.Name(), gc.Equals, "only")
}

func inMap(attrs testing.Attrs, attr string) bool {
	_, ok := attrs[attr]
	return ok
}

func (*suite) TestBootstrapConfig(c *gc.C) {
	attrs := dummySampleConfig().Merge(testing.Attrs{
		"agent-version": "1.2.3",
	})
	c.Assert(inMap(attrs, "secret"), jc.IsTrue)
	c.Assert(inMap(attrs, "ca-private-key"), jc.IsTrue)
	c.Assert(inMap(attrs, "admin-secret"), jc.IsTrue)

	cfg, err := config.New(config.NoDefaults, attrs)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(err, jc.ErrorIsNil)

	cfg1, err := environs.BootstrapConfig(cfg)
	c.Assert(err, jc.ErrorIsNil)

	expect := cfg.AllAttrs()
	expect["admin-secret"] = ""
	expect["ca-private-key"] = ""
	c.Assert(cfg1.AllAttrs(), gc.DeepEquals, expect)
}

func (s *suite) TestDisallowedInBootstrap(c *gc.C) {
	content := `
environments:
    dummy:
        type: dummy
        state-server: false
`
	for key, value := range map[string]interface{}{
		"storage-default-block-source": "loop",
	} {
		envContent := fmt.Sprintf("%s\n        %s: %s", content, key, value)
		envs, err := environs.ReadEnvironsBytes([]byte(envContent))
		c.Check(err, jc.ErrorIsNil)
		_, err = envs.Config("dummy")
		c.Assert(err, gc.ErrorMatches, "attribute .* is not allowed in bootstrap configurations")
	}
}

type dummyProvider struct {
	environs.EnvironProvider
}

func (s *suite) TestRegisterProvider(c *gc.C) {
	s.PatchValue(environs.Providers, make(map[string]environs.EnvironProvider))
	s.PatchValue(environs.ProviderAliases, make(map[string]string))
	type step struct {
		name    string
		aliases []string
		err     string
	}
	type test []step

	tests := []test{
		[]step{{
			name: "providerName",
		}},
		[]step{{
			name:    "providerName",
			aliases: []string{"providerName"},
			err:     "juju: duplicate provider alias \"providerName\"",
		}},
		[]step{{
			name:    "providerName",
			aliases: []string{"providerAlias", "providerAlias"},
			err:     "juju: duplicate provider alias \"providerAlias\"",
		}},
		[]step{{
			name:    "providerName",
			aliases: []string{"providerAlias1", "providerAlias2"},
		}},
		[]step{{
			name: "providerName",
		}, {
			name: "providerName",
			err:  "juju: duplicate provider name \"providerName\"",
		}},
		[]step{{
			name: "providerName1",
		}, {
			name:    "providerName2",
			aliases: []string{"providerName"},
		}},
		[]step{{
			name: "providerName1",
		}, {
			name:    "providerName2",
			aliases: []string{"providerName1"},
			err:     "juju: duplicate provider alias \"providerName1\"",
		}},
	}

	registerProvider := func(name string, aliases []string) (err error) {
		defer func() { err, _ = recover().(error) }()
		registered := &dummyProvider{}
		environs.RegisterProvider(name, registered, aliases...)
		p, err := environs.Provider(name)
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(p, gc.Equals, registered)
		for _, alias := range aliases {
			p, err := environs.Provider(alias)
			c.Assert(err, jc.ErrorIsNil)
			c.Assert(p, gc.Equals, registered)
			c.Assert(p, gc.Equals, registered)
		}
		return nil
	}
	for i, test := range tests {
		c.Logf("test %d: %v", i, test)
		for k := range *environs.Providers {
			delete(*environs.Providers, k)
		}
		for k := range *environs.ProviderAliases {
			delete(*environs.ProviderAliases, k)
		}
		for _, step := range test {
			err := registerProvider(step.name, step.aliases)
			if step.err == "" {
				c.Assert(err, jc.ErrorIsNil)
			} else {
				c.Assert(err, gc.ErrorMatches, step.err)
			}
		}
	}
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
	c.Assert(err, jc.ErrorIsNil)
	return func() {
		_, _, err := loggo.RemoveWriter("test")
		c.Assert(err, jc.ErrorIsNil)
	}
}

func (s *ConfigDeprecationSuite) checkDeprecationWarning(c *gc.C, attrs testing.Attrs, expectedMsg string) {
	content := `
environments:
    deprecated:
        type: dummy
        state-server: false
`
	restore := s.setupLogger(c)
	defer restore()

	envs, err := environs.ReadEnvironsBytes([]byte(content))
	c.Assert(err, jc.ErrorIsNil)
	environs.UpdateEnvironAttrs(envs, "deprecated", attrs)
	_, err = envs.Config("deprecated")
	c.Assert(err, jc.ErrorIsNil)

	var stripped string
	if log := s.writer.Log(); len(log) == 1 {
		stripped = strings.Replace(log[0].Message, "\n", "", -1)
	}

	c.Check(stripped, gc.Matches, expectedMsg)
}

const (
	// This is a standard configuration warning when old attribute was specified.
	standardDeprecationWarning = `.*Your configuration should be updated to set .* %v.*`

	// This is a standard deprecation warning when both old and new attributes were specified.
	standardDeprecationWarningWithNew = `.*is deprecated and will be ignored since the new .*`
)

func (s *ConfigDeprecationSuite) TestDeprecatedToolsURLWarning(c *gc.C) {
	attrs := testing.Attrs{
		"tools-metadata-url": "aknowndeprecatedfield",
	}
	expected := fmt.Sprintf(standardDeprecationWarning, "aknowndeprecatedfield")
	s.checkDeprecationWarning(c, attrs, expected)
}

func (s *ConfigDeprecationSuite) TestDeprecatedSafeModeWarning(c *gc.C) {
	// Test that the warning is logged.
	attrs := testing.Attrs{"provisioner-safe-mode": true}
	expected := fmt.Sprintf(standardDeprecationWarning, "destroyed")
	s.checkDeprecationWarning(c, attrs, expected)
}

func (s *ConfigDeprecationSuite) TestDeprecatedSafeModeWarningWithHarvest(c *gc.C) {
	attrs := testing.Attrs{
		"provisioner-safe-mode":    true,
		"provisioner-harvest-mode": "none",
	}
	// Test that the warning is logged.
	expected := fmt.Sprintf(standardDeprecationWarningWithNew)
	s.checkDeprecationWarning(c, attrs, expected)
}

func (s *ConfigDeprecationSuite) TestDeprecatedToolsURLWithNewURLWarning(c *gc.C) {
	attrs := testing.Attrs{
		"tools-metadata-url": "aknowndeprecatedfield",
		"agent-metadata-url": "newvalue",
	}
	expected := fmt.Sprintf(standardDeprecationWarningWithNew)
	s.checkDeprecationWarning(c, attrs, expected)
}

func (s *ConfigDeprecationSuite) TestDeprecatedTypeNullWarning(c *gc.C) {
	attrs := testing.Attrs{"type": "null"}
	expected := `Provider type "null" has been renamed to "manual".Please update your environment configuration.`
	s.checkDeprecationWarning(c, attrs, expected)
}

func (s *ConfigDeprecationSuite) TestDeprecatedLxcUseCloneWarning(c *gc.C) {
	attrs := testing.Attrs{"lxc-use-clone": true}
	expected := fmt.Sprintf(standardDeprecationWarning, true)
	s.checkDeprecationWarning(c, attrs, expected)
}

func (s *ConfigDeprecationSuite) TestDeprecatedToolsStreamWarning(c *gc.C) {
	attrs := testing.Attrs{"tools-stream": "devel"}
	expected := fmt.Sprintf(standardDeprecationWarning, "devel")
	s.checkDeprecationWarning(c, attrs, expected)
}

func (s *ConfigDeprecationSuite) TestDeprecatedToolsStreamWIthAgentWarning(c *gc.C) {
	attrs := testing.Attrs{
		"tools-stream": "devel",
		"agent-stream": "proposed",
	}
	expected := fmt.Sprintf(standardDeprecationWarningWithNew)
	s.checkDeprecationWarning(c, attrs, expected)
}

func (s *ConfigDeprecationSuite) TestDeprecatedBlockWarning(c *gc.C) {
	assertBlockWarning := func(tst string) {
		attrs := testing.Attrs{tst: true}
		s.checkDeprecationWarning(c, attrs, ".*is deprecated and will be ignored since.*")
	}
	assertBlockWarning("block-destroy-environment")
	assertBlockWarning("block-remove-object")
	assertBlockWarning("block-all-changes")
}
