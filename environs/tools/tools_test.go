// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package tools_test

import (
	gc "launchpad.net/gocheck"
	"launchpad.net/loggo"

	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/config"
	envtesting "launchpad.net/juju-core/environs/testing"
	envtools "launchpad.net/juju-core/environs/tools"
	"launchpad.net/juju-core/errors"
	"launchpad.net/juju-core/provider/dummy"
	"launchpad.net/juju-core/testing"
	jc "launchpad.net/juju-core/testing/checkers"
	coretools "launchpad.net/juju-core/tools"
	"launchpad.net/juju-core/version"
)

type ToolsSuite struct {
	env environs.Environ
	testing.LoggingSuite
	envtesting.ToolsSuite
	origCurrentVersion version.Binary
	helper             toolsTestHelper
}

type LegacyToolsSuite struct {
	ToolsSuite
}

type SimpleStreamsToolsSuite struct {
	ToolsSuite
}

func setupToolsTests() {
	gc.Suite(&ToolsListSuite{})
	gc.Suite(&LegacyToolsSuite{ToolsSuite: ToolsSuite{helper: &LegacyToolsHelper{}}})
	gc.Suite(&SimpleStreamsToolsSuite{ToolsSuite: ToolsSuite{helper: &SimpleStreamsToolsHelper{}}})
}

// toolsTestHelper defines tools manipulation functions which are implemented differently for
// legacy and simplestreams tools tests.
type toolsTestHelper interface {
	uploadCustom(c *gc.C, env environs.Environ, verses ...version.Binary) map[version.Binary]string
	uploadPublic(c *gc.C, env environs.Environ, verses ...version.Binary) map[version.Binary]string
	removeTools(c *gc.C, env environs.Environ)
}

// LegacyToolsHelper implements helper functions for legacy tools tests.
type LegacyToolsHelper struct{}

func (h *LegacyToolsHelper) removeTools(c *gc.C, env environs.Environ) {
	envtesting.RemoveAllTools(c, env)
}

func (h *LegacyToolsHelper) uploadVersions(c *gc.C, storage environs.Storage, verses ...version.Binary) map[version.Binary]string {
	uploaded := map[version.Binary]string{}
	for _, vers := range verses {
		uploaded[vers] = envtesting.UploadFakeToolsVersion(c, storage, vers).URL
	}
	return uploaded
}

func (h *LegacyToolsHelper) uploadCustom(c *gc.C, env environs.Environ, verses ...version.Binary) map[version.Binary]string {
	return h.uploadVersions(c, env.Storage(), verses...)
}

func (h *LegacyToolsHelper) uploadPublic(c *gc.C, env environs.Environ, verses ...version.Binary) map[version.Binary]string {
	storage := env.PublicStorage().(environs.Storage)
	return h.uploadVersions(c, storage, verses...)
}

// SimpleStreamsToolsHelper implements helper functions for simplestreams tools tests.
type SimpleStreamsToolsHelper struct{}

func (h *SimpleStreamsToolsHelper) removeTools(c *gc.C, env environs.Environ) {
	//	todo
}

func (h *SimpleStreamsToolsHelper) uploadVersions(c *gc.C, verses ...version.Binary) map[version.Binary]string {
	uploaded := map[version.Binary]string{}
	//todo
	return uploaded
}

func (h *SimpleStreamsToolsHelper) uploadCustom(c *gc.C, env environs.Environ, verses ...version.Binary) map[version.Binary]string {
	return h.uploadVersions(c, verses...)
}

func (h *SimpleStreamsToolsHelper) uploadPublic(c *gc.C, env environs.Environ, verses ...version.Binary) map[version.Binary]string {
	return h.uploadVersions(c, verses...)
}

func (s *ToolsSuite) SetUpTest(c *gc.C) {
	s.LoggingSuite.SetUpTest(c)
	s.ToolsSuite.SetUpTest(c)
	s.origCurrentVersion = version.Current
	s.Reset(c, nil)
}

func (s *ToolsSuite) TearDownTest(c *gc.C) {
	dummy.Reset()
	version.Current = s.origCurrentVersion
	s.ToolsSuite.TearDownTest(c)
	s.LoggingSuite.TearDownTest(c)
}

func (s *ToolsSuite) Reset(c *gc.C, attrs map[string]interface{}) {
	version.Current = s.origCurrentVersion
	dummy.Reset()
	final := map[string]interface{}{
		"name":            "test",
		"type":            "dummy",
		"state-server":    false,
		"authorized-keys": "i-am-a-key",
		"ca-cert":         testing.CACert,
		"ca-private-key":  "",
	}
	for k, v := range attrs {
		final[k] = v
	}
	cfg, err := config.New(final)
	c.Assert(err, gc.IsNil)
	env, err := environs.Prepare(cfg)
	c.Assert(err, gc.IsNil)
	s.env = env
	s.helper.removeTools(c, s.env)
}

var findToolsTests = []struct {
	info   string
	major  int
	minor  int
	custom []version.Binary
	public []version.Binary
	expect []version.Binary
	err    error
}{{
	info:  "none available anywhere",
	major: 1,
	err:   envtools.ErrNoTools,
}, {
	info:   "custom/private tools only, none matching",
	major:  1,
	minor:  2,
	custom: envtesting.V220all,
	err:    coretools.ErrNoMatches,
}, {
	info:   "custom tools found",
	major:  1,
	minor:  2,
	custom: envtesting.VAll,
	expect: envtesting.V120all,
}, {
	info:   "public tools found",
	major:  1,
	minor:  1,
	public: envtesting.VAll,
	expect: envtesting.V110all,
}, {
	info:   "public and custom tools found, only taken from custom",
	major:  1,
	minor:  1,
	custom: envtesting.V110p,
	public: envtesting.VAll,
	expect: envtesting.V110p,
}, {
	info:   "custom tools completely block public ones",
	major:  1,
	custom: envtesting.V220all,
	public: envtesting.VAll,
	err:    coretools.ErrNoMatches,
}, {
	info:   "tools matching major version only",
	major:  1,
	minor:  -1,
	public: envtesting.VAll,
	expect: envtesting.V1all,
}}

func (s *ToolsSuite) TestFindTools(c *gc.C) {
	for i, test := range findToolsTests {
		c.Logf("\ntest %d: %s", i, test.info)
		s.Reset(c, nil)
		custom := s.helper.uploadCustom(c, s.env, test.custom...)
		public := s.helper.uploadPublic(c, s.env, test.public...)
		actual, err := envtools.FindTools(s.env, test.major, test.minor, coretools.Filter{})
		if test.err != nil {
			if len(actual) > 0 {
				c.Logf(actual.String())
			}
			c.Check(err, jc.Satisfies, errors.IsNotFoundError)
			continue
		}
		source := custom
		if len(source) == 0 {
			// We only use the public tools if there are *no* custom tools.
			source = public
		}
		expect := map[version.Binary]string{}
		for _, expected := range test.expect {
			expect[expected] = source[expected]
		}
		c.Check(actual.URLs(), gc.DeepEquals, expect)
	}
}

func (s *LegacyToolsSuite) TestFindToolsFiltering(c *gc.C) {
	tw := &loggo.TestWriter{}
	c.Assert(loggo.RegisterWriter("filter-tester", tw, loggo.DEBUG), gc.IsNil)
	defer loggo.RemoveWriter("filter-tester")
	_, err := envtools.FindTools(s.env, 1, -1, coretools.Filter{Number: version.Number{Major: 1, Minor: 2, Patch: 3}})
	c.Assert(err, jc.Satisfies, errors.IsNotFoundError)
	// This is slightly overly prescriptive, but feel free to change or add
	// messages. This still helps to ensure that all log messages are
	// properly formed.
	c.Check(tw.Log, jc.LogMatches, []jc.SimpleMessage{
		{loggo.INFO, "reading tools with major version 1"},
		{loggo.INFO, "filtering tools by version: 1.2.3"},
		{loggo.DEBUG, `cannot load index "dummy-tools-url/streams/v1/index.sjson": invalid URL "dummy-tools-url/streams/v1/index.sjson" not found`},
		{loggo.DEBUG, `cannot load index "dummy-tools-url/streams/v1/index.json": invalid URL "dummy-tools-url/streams/v1/index.json" not found`},
		{loggo.DEBUG, "reading v1.* tools"},
		{loggo.DEBUG, "reading v1.* tools"},
	})
}

func (s *SimpleStreamsToolsSuite) TestFindToolsFiltering(c *gc.C) {
	tw := &loggo.TestWriter{}
	c.Assert(loggo.RegisterWriter("filter-tester", tw, loggo.DEBUG), gc.IsNil)
	defer loggo.RemoveWriter("filter-tester")
	_, err := envtools.FindTools(s.env, 1, -1, coretools.Filter{Number: version.Number{Major: 1, Minor: 2, Patch: 3}})
	c.Assert(err, jc.Satisfies, errors.IsNotFoundError)
	// This is slightly overly prescriptive, but feel free to change or add
	// messages. This still helps to ensure that all log messages are
	// properly formed.
	c.Check(tw.Log, jc.LogMatches, []jc.SimpleMessage{
			{loggo.INFO, "reading tools with major version 1"},
			{loggo.INFO, "filtering tools by version: 1.2.3"},
			{loggo.DEBUG, `cannot load index "dummy-tools-url/streams/v1/index.sjson": invalid URL "dummy-tools-url/streams/v1/index.sjson" not found`},
			{loggo.DEBUG, `cannot load index "dummy-tools-url/streams/v1/index.json": invalid URL "dummy-tools-url/streams/v1/index.json" not found`},
			{loggo.DEBUG, "reading v1.* tools"},
			{loggo.DEBUG, "reading v1.* tools"},
			{loggo.DEBUG, "found 1.13.3-precise-amd64"},
			{loggo.DEBUG, "found 1.13.3-raring-amd64"},
			{loggo.ERROR, `cannot match tools.Filter{Released:false, Number:version.Number{Major:1, Minor:2, Patch:3, Build:0}, Series:"", Arch:""}`},
		})
}

func (s *ToolsSuite) TestFindBootstrapTools(c *gc.C) {
	for i, test := range envtesting.BootstrapToolsTests {
		c.Logf("\ntest %d: %s", i, test.Info)
		attrs := map[string]interface{}{
			"development":    test.Development,
			"default-series": test.DefaultSeries,
		}
		var agentVersion *version.Number
		if test.AgentVersion != version.Zero {
			attrs["agent-version"] = test.AgentVersion.String()
			agentVersion = &test.AgentVersion
		}
		s.Reset(c, attrs)
		version.Current = test.CliVersion
		available := s.helper.uploadCustom(c, s.env, test.Available...)
		if len(available) > 0 {
			// These should never be chosen.
			s.helper.uploadPublic(c, s.env, envtesting.VAll...)
		}

		cfg := s.env.Config()
		actual, err := envtools.FindBootstrapTools(
			s.env, agentVersion, cfg.DefaultSeries(), &test.Arch, cfg.Development())
		if test.Err != nil {
			if len(actual) > 0 {
				c.Logf(actual.String())
			}
			c.Check(err, jc.Satisfies, errors.IsNotFoundError)
			continue
		}
		expect := map[version.Binary]string{}
		for _, expected := range test.Expect {
			expect[expected] = available[expected]
		}
		c.Check(actual.URLs(), gc.DeepEquals, expect)
	}
}

var findInstanceToolsTests = []struct {
	info         string
	available    []version.Binary
	agentVersion version.Number
	series       string
	arch         string
	expect       []version.Binary
	err          error
}{{
	info:         "nothing at all",
	agentVersion: envtesting.V120,
	series:       "precise",
	err:          envtools.ErrNoTools,
}, {
	info:         "nothing matching 1",
	available:    envtesting.V100Xall,
	agentVersion: envtesting.V120,
	series:       "precise",
	err:          coretools.ErrNoMatches,
}, {
	info:         "nothing matching 2",
	available:    envtesting.V120all,
	agentVersion: envtesting.V110,
	series:       "precise",
	err:          coretools.ErrNoMatches,
}, {
	info:         "nothing matching 3",
	available:    envtesting.V120q,
	agentVersion: envtesting.V120,
	series:       "precise",
	err:          coretools.ErrNoMatches,
}, {
	info:         "nothing matching 4",
	available:    envtesting.V120q,
	agentVersion: envtesting.V120,
	series:       "quantal",
	arch:         "arm",
	err:          coretools.ErrNoMatches,
}, {
	info:         "actual match 1",
	available:    envtesting.VAll,
	agentVersion: envtesting.V1001,
	series:       "precise",
	expect:       []version.Binary{envtesting.V1001p64},
}, {
	info:         "actual match 2",
	available:    envtesting.VAll,
	agentVersion: envtesting.V120,
	series:       "quantal",
	expect:       []version.Binary{envtesting.V120q64, envtesting.V120q32},
}, {
	info:         "actual match 3",
	available:    envtesting.VAll,
	agentVersion: envtesting.V110,
	series:       "quantal",
	arch:         "i386",
	expect:       []version.Binary{envtesting.V110q32},
}}

func (s *ToolsSuite) TestFindInstanceTools(c *gc.C) {
	for i, test := range findInstanceToolsTests {
		c.Logf("\ntest %d: %s", i, test.info)
		s.Reset(c, map[string]interface{}{
			"agent-version": test.agentVersion.String(),
		})
		available := s.helper.uploadCustom(c, s.env, test.available...)
		if len(available) > 0 {
			// These should never be chosen.
			s.helper.uploadPublic(c, s.env, envtesting.VAll...)
		}

		agentVersion, _ := s.env.Config().AgentVersion()
		actual, err := envtools.FindInstanceTools(s.env, agentVersion, test.series, &test.arch)
		if test.err != nil {
			if len(actual) > 0 {
				c.Logf(actual.String())
			}
			c.Check(err, jc.Satisfies, errors.IsNotFoundError)
			continue
		}
		expect := map[version.Binary]string{}
		for _, expected := range test.expect {
			expect[expected] = available[expected]
		}
		c.Check(actual.URLs(), gc.DeepEquals, expect)
	}
}

var findExactToolsTests = []struct {
	info   string
	custom []version.Binary
	public []version.Binary
	seek   version.Binary
	err    error
}{{
	info: "nothing available",
	seek: envtesting.V100p64,
	err:  envtools.ErrNoTools,
}, {
	info:   "only non-matches available in custom",
	custom: append(envtesting.V110all, envtesting.V100p32, envtesting.V100q64, envtesting.V1001p64),
	seek:   envtesting.V100p64,
	err:    coretools.ErrNoMatches,
}, {
	info:   "exact match available in custom",
	custom: []version.Binary{envtesting.V100p64},
	seek:   envtesting.V100p64,
}, {
	info:   "only non-matches available in public",
	custom: append(envtesting.V110all, envtesting.V100p32, envtesting.V100q64, envtesting.V1001p64),
	seek:   envtesting.V100p64,
	err:    coretools.ErrNoMatches,
}, {
	info:   "exact match available in public",
	public: []version.Binary{envtesting.V100p64},
	seek:   envtesting.V100p64,
}, {
	info:   "exact match in public blocked by custom",
	custom: envtesting.V110all,
	public: []version.Binary{envtesting.V100p64},
	seek:   envtesting.V100p64,
	err:    coretools.ErrNoMatches,
}}

func (s *ToolsSuite) TestFindExactTools(c *gc.C) {
	for i, test := range findExactToolsTests {
		c.Logf("\ntest %d: %s", i, test.info)
		s.Reset(c, nil)
		custom := s.helper.uploadCustom(c, s.env, test.custom...)
		public := s.helper.uploadPublic(c, s.env, test.public...)
		actual, err := envtools.FindExactTools(s.env, test.seek.Number, test.seek.Series, test.seek.Arch)
		if test.err == nil {
			c.Check(err, gc.IsNil)
			c.Check(actual.Version, gc.Equals, test.seek)
			source := custom
			if len(source) == 0 {
				// We only use public tools if there are *no* private tools.
				source = public
			}
			c.Check(actual.URL, gc.DeepEquals, source[actual.Version])
		} else {
			c.Check(err, jc.Satisfies, errors.IsNotFoundError)
		}
	}
}

// fakeToolsForSeries fakes a Tools object with just enough information for
// testing the handling its OS series.
func fakeToolsForSeries(series string) *coretools.Tools {
	return &coretools.Tools{Version: version.Binary{Series: series}}
}

// fakeToolsList fakes a envtools.List containing Tools objects for the given
// respective series, in the same number and order.
func fakeToolsList(series ...string) coretools.List {
	list := coretools.List{}
	for _, name := range series {
		list = append(list, fakeToolsForSeries(name))
	}
	return list
}

type ToolsListSuite struct{}

func (s *ToolsListSuite) TestCheckToolsSeriesRequiresTools(c *gc.C) {
	err := envtools.CheckToolsSeries(fakeToolsList(), "precise")
	c.Assert(err, gc.NotNil)
	c.Check(err, gc.ErrorMatches, "expected single series, got \\[\\]")
}

func (s *ToolsListSuite) TestCheckToolsSeriesAcceptsOneSetOfTools(c *gc.C) {
	names := []string{"precise", "raring"}
	for _, series := range names {
		list := fakeToolsList(series)
		err := envtools.CheckToolsSeries(list, series)
		c.Check(err, gc.IsNil)
	}
}

func (s *ToolsListSuite) TestCheckToolsSeriesAcceptsMultipleForSameSeries(c *gc.C) {
	series := "quantal"
	list := fakeToolsList(series, series, series)
	err := envtools.CheckToolsSeries(list, series)
	c.Check(err, gc.IsNil)
}

func (s *ToolsListSuite) TestCheckToolsSeriesRejectsToolsForOtherSeries(c *gc.C) {
	list := fakeToolsList("hoary")
	err := envtools.CheckToolsSeries(list, "warty")
	c.Assert(err, gc.NotNil)
	c.Check(err, gc.ErrorMatches, "tools mismatch: expected series warty, got hoary")
}

func (s *ToolsListSuite) TestCheckToolsSeriesRejectsToolsForMixedSeries(c *gc.C) {
	list := fakeToolsList("precise", "raring")
	err := envtools.CheckToolsSeries(list, "precise")
	c.Assert(err, gc.NotNil)
	c.Check(err, gc.ErrorMatches, "expected single series, got .*")
}
