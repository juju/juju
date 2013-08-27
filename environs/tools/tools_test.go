// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package tools_test

import (
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/constraints"
	"launchpad.net/juju-core/environs"
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
	origCurrentVersion version.Binary
}

var _ = gc.Suite(&ToolsSuite{})

func (s *ToolsSuite) SetUpTest(c *gc.C) {
	s.LoggingSuite.SetUpTest(c)
	s.origCurrentVersion = version.Current
	s.Reset(c, nil)
}

func (s *ToolsSuite) TearDownTest(c *gc.C) {
	dummy.Reset()
	version.Current = s.origCurrentVersion
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
	env, err := environs.NewFromAttrs(final)
	c.Assert(err, gc.IsNil)
	s.env = env
	envtesting.RemoveAllTools(c, s.env)
}

var (
	v100    = version.MustParse("1.0.0")
	v100p64 = version.MustParseBinary("1.0.0-precise-amd64")
	v100p32 = version.MustParseBinary("1.0.0-precise-i386")
	v100p   = []version.Binary{v100p64, v100p32}

	v100q64 = version.MustParseBinary("1.0.0-quantal-amd64")
	v100q32 = version.MustParseBinary("1.0.0-quantal-i386")
	v100q   = []version.Binary{v100q64, v100q32}
	v100all = append(v100p, v100q...)

	v1001    = version.MustParse("1.0.0.1")
	v1001p64 = version.MustParseBinary("1.0.0.1-precise-amd64")
	v100Xall = append(v100all, v1001p64)

	v110    = version.MustParse("1.1.0")
	v110p64 = version.MustParseBinary("1.1.0-precise-amd64")
	v110p32 = version.MustParseBinary("1.1.0-precise-i386")
	v110p   = []version.Binary{v110p64, v110p32}

	v110q64 = version.MustParseBinary("1.1.0-quantal-amd64")
	v110q32 = version.MustParseBinary("1.1.0-quantal-i386")
	v110q   = []version.Binary{v110q64, v110q32}
	v110all = append(v110p, v110q...)

	v120    = version.MustParse("1.2.0")
	v120p64 = version.MustParseBinary("1.2.0-precise-amd64")
	v120p32 = version.MustParseBinary("1.2.0-precise-i386")
	v120p   = []version.Binary{v120p64, v120p32}

	v120q64 = version.MustParseBinary("1.2.0-quantal-amd64")
	v120q32 = version.MustParseBinary("1.2.0-quantal-i386")
	v120q   = []version.Binary{v120q64, v120q32}
	v120all = append(v120p, v120q...)
	v1all   = append(v100Xall, append(v110all, v120all...)...)

	v220    = version.MustParse("2.2.0")
	v220p32 = version.MustParseBinary("2.2.0-precise-i386")
	v220p64 = version.MustParseBinary("2.2.0-precise-amd64")
	v220q32 = version.MustParseBinary("2.2.0-quantal-i386")
	v220q64 = version.MustParseBinary("2.2.0-quantal-amd64")
	v220all = []version.Binary{v220p64, v220p32, v220q64, v220q32}
	vAll    = append(v1all, v220all...)
)

func (s *ToolsSuite) uploadVersions(c *gc.C, storage environs.Storage, verses ...version.Binary) map[version.Binary]string {
	uploaded := map[version.Binary]string{}
	for _, vers := range verses {
		uploaded[vers] = envtesting.UploadFakeToolsVersion(c, storage, vers).URL
	}
	return uploaded
}

func (s *ToolsSuite) uploadPrivate(c *gc.C, verses ...version.Binary) map[version.Binary]string {
	return s.uploadVersions(c, s.env.Storage(), verses...)
}

func (s *ToolsSuite) uploadPublic(c *gc.C, verses ...version.Binary) map[version.Binary]string {
	storage := s.env.PublicStorage().(environs.Storage)
	return s.uploadVersions(c, storage, verses...)
}

var findToolsTests = []struct {
	info    string
	major   int
	private []version.Binary
	public  []version.Binary
	expect  []version.Binary
	err     error
}{{
	info:  "none available anywhere",
	major: 1,
	err:   envtools.ErrNoTools,
}, {
	info:    "private tools only, none matching",
	major:   1,
	private: v220all,
	err:     coretools.ErrNoMatches,
}, {
	info:    "tools found in private bucket",
	major:   1,
	private: vAll,
	expect:  v1all,
}, {
	info:   "tools found in public bucket",
	major:  1,
	public: vAll,
	expect: v1all,
}, {
	info:    "tools found in both buckets, only taken from private",
	major:   1,
	private: v110p,
	public:  vAll,
	expect:  v110p,
}, {
	info:    "private tools completely block public ones",
	major:   1,
	private: v220all,
	public:  vAll,
	err:     coretools.ErrNoMatches,
}}

func (s *ToolsSuite) TestFindTools(c *gc.C) {
	for i, test := range findToolsTests {
		c.Logf("\ntest %d: %s", i, test.info)
		s.Reset(c, nil)
		private := s.uploadPrivate(c, test.private...)
		public := s.uploadPublic(c, test.public...)
		actual, err := envtools.FindTools(s.env, test.major, coretools.Filter{})
		if test.err != nil {
			if len(actual) > 0 {
				c.Logf(actual.String())
			}
			c.Check(err, jc.Satisfies, errors.IsNotFoundError)
			continue
		}
		source := private
		if len(source) == 0 {
			// We only use the public bucket if the private one has *no* envtools.
			source = public
		}
		expect := map[version.Binary]string{}
		for _, expected := range test.expect {
			expect[expected] = source[expected]
		}
		c.Check(actual.URLs(), gc.DeepEquals, expect)
	}
}

var findBootstrapToolsTests = []struct {
	info          string
	available     []version.Binary
	cliVersion    version.Binary
	defaultSeries string
	agentVersion  version.Number
	development   bool
	constraints   string
	expect        []version.Binary
	err           error
}{{
	info:          "no tools at all",
	cliVersion:    v100p64,
	defaultSeries: "precise",
	err:           envtools.ErrNoTools,
}, {
	info:          "released cli: use newest compatible release version",
	available:     vAll,
	cliVersion:    v100p64,
	defaultSeries: "precise",
	expect:        v120p,
}, {
	info:          "released cli: cli arch ignored",
	available:     vAll,
	cliVersion:    v100p32,
	defaultSeries: "precise",
	expect:        v120p,
}, {
	info:          "released cli: cli series ignored",
	available:     vAll,
	cliVersion:    v100q64,
	defaultSeries: "precise",
	expect:        v120p,
}, {
	info:          "released cli: series taken from default-series",
	available:     v100Xall,
	cliVersion:    v100p64,
	defaultSeries: "quantal",
	expect:        v100q,
}, {
	info:          "released cli: ignore close dev match",
	available:     v100Xall,
	cliVersion:    v120p64,
	defaultSeries: "precise",
	expect:        v100p,
}, {
	info:          "released cli: use older release version if necessary",
	available:     v100Xall,
	cliVersion:    v120p64,
	defaultSeries: "quantal",
	expect:        v100q,
}, {
	info:          "released cli: ignore irrelevant constraints",
	available:     v100Xall,
	cliVersion:    v100p64,
	defaultSeries: "precise",
	constraints:   "mem=32G",
	expect:        v100p,
}, {
	info:          "released cli: filter by arch constraints",
	available:     v120all,
	cliVersion:    v100p64,
	defaultSeries: "precise",
	constraints:   "arch=i386",
	expect:        []version.Binary{v120p32},
}, {
	info:          "released cli: specific released version",
	available:     vAll,
	cliVersion:    v120p64,
	agentVersion:  v100,
	defaultSeries: "precise",
	expect:        v100p,
}, {
	info:          "released cli: specific dev version",
	available:     vAll,
	cliVersion:    v120p64,
	agentVersion:  v110,
	defaultSeries: "precise",
	expect:        v110p,
}, {
	info:          "released cli: major upgrades bad",
	available:     v220all,
	cliVersion:    v100p64,
	defaultSeries: "precise",
	err:           coretools.ErrNoMatches,
}, {
	info:          "released cli: major downgrades bad",
	available:     v100Xall,
	cliVersion:    v220p64,
	defaultSeries: "precise",
	err:           coretools.ErrNoMatches,
}, {
	info:          "released cli: no matching series",
	available:     vAll,
	cliVersion:    v100p64,
	defaultSeries: "raring",
	err:           coretools.ErrNoMatches,
}, {
	info:          "released cli: no matching arches",
	available:     vAll,
	cliVersion:    v100p64,
	defaultSeries: "precise",
	constraints:   "arch=arm",
	err:           coretools.ErrNoMatches,
}, {
	info:          "released cli: specific bad major 1",
	available:     vAll,
	cliVersion:    v220p64,
	agentVersion:  v120,
	defaultSeries: "precise",
	err:           coretools.ErrNoMatches,
}, {
	info:          "released cli: specific bad major 2",
	available:     vAll,
	cliVersion:    v120p64,
	agentVersion:  v220,
	defaultSeries: "precise",
	err:           coretools.ErrNoMatches,
}, {
	info:          "released cli: ignore dev tools 1",
	available:     v110all,
	cliVersion:    v100p64,
	defaultSeries: "precise",
	err:           coretools.ErrNoMatches,
}, {
	info:          "released cli: ignore dev tools 2",
	available:     v110all,
	cliVersion:    v120p64,
	defaultSeries: "precise",
	err:           coretools.ErrNoMatches,
}, {
	info:          "released cli: ignore dev tools 3",
	available:     []version.Binary{v1001p64},
	cliVersion:    v100p64,
	defaultSeries: "precise",
	err:           coretools.ErrNoMatches,
}, {
	info:          "released cli with dev setting picks newest matching 1",
	available:     v100Xall,
	cliVersion:    v120q32,
	defaultSeries: "precise",
	development:   true,
	expect:        []version.Binary{v1001p64},
}, {
	info:          "released cli with dev setting picks newest matching 2",
	available:     vAll,
	cliVersion:    v100q64,
	defaultSeries: "precise",
	development:   true,
	constraints:   "arch=i386",
	expect:        []version.Binary{v120p32},
}, {
	info:          "released cli with dev setting respects agent-version",
	available:     vAll,
	cliVersion:    v100q32,
	agentVersion:  v1001,
	defaultSeries: "precise",
	development:   true,
	expect:        []version.Binary{v1001p64},
}, {
	info:          "dev cli picks newest matching 1",
	available:     v100Xall,
	cliVersion:    v110q32,
	defaultSeries: "precise",
	expect:        []version.Binary{v1001p64},
}, {
	info:          "dev cli picks newest matching 2",
	available:     vAll,
	cliVersion:    v110q64,
	defaultSeries: "precise",
	constraints:   "arch=i386",
	expect:        []version.Binary{v120p32},
}, {
	info:          "dev cli respects agent-version",
	available:     vAll,
	cliVersion:    v110q32,
	agentVersion:  v1001,
	defaultSeries: "precise",
	expect:        []version.Binary{v1001p64},
}}

func (s *ToolsSuite) TestFindBootstrapTools(c *gc.C) {
	for i, test := range findBootstrapToolsTests {
		c.Logf("\ntest %d: %s", i, test.info)
		attrs := map[string]interface{}{
			"development":    test.development,
			"default-series": test.defaultSeries,
		}
		if test.agentVersion != version.Zero {
			attrs["agent-version"] = test.agentVersion.String()
		}
		s.Reset(c, attrs)
		version.Current = test.cliVersion
		available := s.uploadPrivate(c, test.available...)
		if len(available) > 0 {
			// These should never be chosen.
			s.uploadPublic(c, vAll...)
		}

		cons := constraints.MustParse(test.constraints)
		actual, err := envtools.FindBootstrapTools(s.env, cons)
		if test.err != nil {
			if len(actual) > 0 {
				c.Logf(actual.String())
			}
			c.Check(err, jc.Satisfies, errors.IsNotFoundError)
			continue
		}
		expect := map[version.Binary]string{}
		unique := map[version.Number]bool{}
		for _, expected := range test.expect {
			expect[expected] = available[expected]
			unique[expected.Number] = true
		}
		c.Check(actual.URLs(), gc.DeepEquals, expect)
		for expectAgentVersion := range unique {
			agentVersion, ok := s.env.Config().AgentVersion()
			c.Check(ok, gc.Equals, true)
			c.Check(agentVersion, gc.Equals, expectAgentVersion)
		}
	}
}

var findInstanceToolsTests = []struct {
	info         string
	available    []version.Binary
	agentVersion version.Number
	series       string
	constraints  string
	expect       []version.Binary
	err          error
}{{
	info:         "nothing at all",
	agentVersion: v120,
	series:       "precise",
	err:          envtools.ErrNoTools,
}, {
	info:         "nothing matching 1",
	available:    v100Xall,
	agentVersion: v120,
	series:       "precise",
	err:          coretools.ErrNoMatches,
}, {
	info:         "nothing matching 2",
	available:    v120all,
	agentVersion: v110,
	series:       "precise",
	err:          coretools.ErrNoMatches,
}, {
	info:         "nothing matching 3",
	available:    v120q,
	agentVersion: v120,
	series:       "precise",
	err:          coretools.ErrNoMatches,
}, {
	info:         "nothing matching 4",
	available:    v120q,
	agentVersion: v120,
	series:       "quantal",
	constraints:  "arch=arm",
	err:          coretools.ErrNoMatches,
}, {
	info:         "actual match 1",
	available:    vAll,
	agentVersion: v1001,
	series:       "precise",
	expect:       []version.Binary{v1001p64},
}, {
	info:         "actual match 2",
	available:    vAll,
	agentVersion: v120,
	series:       "quantal",
	expect:       []version.Binary{v120q64, v120q32},
}, {
	info:         "actual match 3",
	available:    vAll,
	agentVersion: v110,
	series:       "quantal",
	constraints:  "arch=i386",
	expect:       []version.Binary{v110q32},
}}

func (s *ToolsSuite) TestFindInstanceTools(c *gc.C) {
	for i, test := range findInstanceToolsTests {
		c.Logf("\ntest %d: %s", i, test.info)
		s.Reset(c, map[string]interface{}{
			"agent-version": test.agentVersion.String(),
		})
		available := s.uploadPrivate(c, test.available...)
		if len(available) > 0 {
			// These should never be chosen.
			s.uploadPublic(c, vAll...)
		}

		cons := constraints.MustParse(test.constraints)
		actual, err := envtools.FindInstanceTools(s.env, test.series, cons)
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
	info    string
	private []version.Binary
	public  []version.Binary
	seek    version.Binary
	err     error
}{{
	info: "nothing available",
	seek: v100p64,
	err:  envtools.ErrNoTools,
}, {
	info:    "only non-matches available in private",
	private: append(v110all, v100p32, v100q64, v1001p64),
	seek:    v100p64,
	err:     coretools.ErrNoMatches,
}, {
	info:    "exact match available in private",
	private: []version.Binary{v100p64},
	seek:    v100p64,
}, {
	info:    "only non-matches available in public",
	private: append(v110all, v100p32, v100q64, v1001p64),
	seek:    v100p64,
	err:     coretools.ErrNoMatches,
}, {
	info:   "exact match available in public",
	public: []version.Binary{v100p64},
	seek:   v100p64,
}, {
	info:    "exact match in public blocked by private",
	private: v110all,
	public:  []version.Binary{v100p64},
	seek:    v100p64,
	err:     coretools.ErrNoMatches,
}}

func (s *ToolsSuite) TestFindExactTools(c *gc.C) {
	for i, test := range findExactToolsTests {
		c.Logf("\ntest %d: %s", i, test.info)
		s.Reset(c, nil)
		private := s.uploadPrivate(c, test.private...)
		public := s.uploadPublic(c, test.public...)
		actual, err := envtools.FindExactTools(s.env, test.seek)
		if test.err == nil {
			c.Check(err, gc.IsNil)
			c.Check(actual.Version, gc.Equals, test.seek)
			source := private
			if len(source) == 0 {
				// We only use the public bucket if the private one has *no* envtools.
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

func (s *ToolsSuite) TestCheckToolsSeriesRequiresTools(c *gc.C) {
	err := envtools.CheckToolsSeries(fakeToolsList(), "precise")
	c.Assert(err, gc.NotNil)
	c.Check(err, gc.ErrorMatches, "expected single series, got \\[\\]")
}

func (s *ToolsSuite) TestCheckToolsSeriesAcceptsOneSetOfTools(c *gc.C) {
	names := []string{"precise", "raring"}
	for _, series := range names {
		list := fakeToolsList(series)
		err := envtools.CheckToolsSeries(list, series)
		c.Check(err, gc.IsNil)
	}
}

func (s *ToolsSuite) TestCheckToolsSeriesAcceptsMultipleForSameSeries(c *gc.C) {
	series := "quantal"
	list := fakeToolsList(series, series, series)
	err := envtools.CheckToolsSeries(list, series)
	c.Check(err, gc.IsNil)
}

func (s *ToolsSuite) TestCheckToolsSeriesRejectsToolsForOtherSeries(c *gc.C) {
	list := fakeToolsList("hoary")
	err := envtools.CheckToolsSeries(list, "warty")
	c.Assert(err, gc.NotNil)
	c.Check(err, gc.ErrorMatches, "tools mismatch: expected series warty, got hoary")
}

func (s *ToolsSuite) TestCheckToolsSeriesRejectsToolsForMixedSeries(c *gc.C) {
	list := fakeToolsList("precise", "raring")
	err := envtools.CheckToolsSeries(list, "precise")
	c.Assert(err, gc.NotNil)
	c.Check(err, gc.ErrorMatches, "expected single series, got .*")
}
