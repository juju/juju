// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package tools_test

import (
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/configstore"
	envtesting "github.com/juju/juju/environs/testing"
	envtools "github.com/juju/juju/environs/tools"
	toolstesting "github.com/juju/juju/environs/tools/testing"
	"github.com/juju/juju/provider/dummy"
	coretesting "github.com/juju/juju/testing"
	coretools "github.com/juju/juju/tools"
	"github.com/juju/juju/version"
)

type SimpleStreamsToolsSuite struct {
	env environs.Environ
	coretesting.BaseSuite
	envtesting.ToolsFixture
	origCurrentVersion version.Binary
	customToolsDir     string
	publicToolsDir     string
}

func setupToolsTests() {
	gc.Suite(&SimpleStreamsToolsSuite{})
}

func (s *SimpleStreamsToolsSuite) SetUpSuite(c *gc.C) {
	s.BaseSuite.SetUpSuite(c)
	s.customToolsDir = c.MkDir()
	s.publicToolsDir = c.MkDir()
}

func (s *SimpleStreamsToolsSuite) SetUpTest(c *gc.C) {
	s.ToolsFixture.DefaultBaseURL = utils.MakeFileURL(s.publicToolsDir)
	s.BaseSuite.SetUpTest(c)
	s.ToolsFixture.SetUpTest(c)
	s.origCurrentVersion = version.Current
	s.reset(c, nil)
}

func (s *SimpleStreamsToolsSuite) TearDownTest(c *gc.C) {
	dummy.Reset()
	version.Current = s.origCurrentVersion
	s.ToolsFixture.TearDownTest(c)
	s.BaseSuite.TearDownTest(c)
}

func (s *SimpleStreamsToolsSuite) reset(c *gc.C, attrs map[string]interface{}) {
	final := map[string]interface{}{
		"agent-metadata-url": utils.MakeFileURL(s.customToolsDir),
		"agent-stream":       "proposed",
	}
	for k, v := range attrs {
		final[k] = v
	}
	s.resetEnv(c, final)
}

func (s *SimpleStreamsToolsSuite) removeTools(c *gc.C) {
	for _, dir := range []string{s.customToolsDir, s.publicToolsDir} {
		files, err := ioutil.ReadDir(dir)
		c.Assert(err, jc.ErrorIsNil)
		for _, f := range files {
			err := os.RemoveAll(filepath.Join(dir, f.Name()))
			c.Assert(err, jc.ErrorIsNil)
		}
	}
}

func (s *SimpleStreamsToolsSuite) uploadCustom(c *gc.C, verses ...version.Binary) map[version.Binary]string {
	return toolstesting.UploadToDirectory(c, "proposed", s.customToolsDir, verses...)
}

func (s *SimpleStreamsToolsSuite) uploadPublic(c *gc.C, verses ...version.Binary) map[version.Binary]string {
	return toolstesting.UploadToDirectory(c, "proposed", s.publicToolsDir, verses...)
}

func (s *SimpleStreamsToolsSuite) resetEnv(c *gc.C, attrs map[string]interface{}) {
	version.Current = s.origCurrentVersion
	dummy.Reset()
	cfg, err := config.New(config.NoDefaults, dummy.SampleConfig().Merge(attrs))
	c.Assert(err, jc.ErrorIsNil)
	env, err := environs.Prepare(cfg, envtesting.BootstrapContext(c), configstore.NewMem())
	c.Assert(err, jc.ErrorIsNil)
	s.env = env
	s.removeTools(c)
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
	minor:  -1,
	custom: envtesting.V220all,
	public: envtesting.VAll,
	expect: envtesting.V1all,
}, {
	info:   "tools matching major version only",
	major:  1,
	minor:  -1,
	public: envtesting.VAll,
	expect: envtesting.V1all,
}}

func (s *SimpleStreamsToolsSuite) TestFindTools(c *gc.C) {
	for i, test := range findToolsTests {
		c.Logf("\ntest %d: %s", i, test.info)
		s.reset(c, nil)
		custom := s.uploadCustom(c, test.custom...)
		public := s.uploadPublic(c, test.public...)
		stream := envtools.PreferredStream(&version.Current.Number, s.env.Config().Development(), s.env.Config().AgentStream())
		actual, err := envtools.FindTools(s.env, test.major, test.minor, stream, coretools.Filter{})
		if test.err != nil {
			if len(actual) > 0 {
				c.Logf(actual.String())
			}
			c.Check(err, jc.Satisfies, errors.IsNotFound)
			continue
		}
		expect := map[version.Binary]string{}
		for _, expected := range test.expect {
			// If the tools exist in custom, that's preferred.
			var ok bool
			if expect[expected], ok = custom[expected]; !ok {
				expect[expected] = public[expected]
			}
		}
		c.Check(actual.URLs(), gc.DeepEquals, expect)
	}
}

func (s *SimpleStreamsToolsSuite) TestFindToolsFiltering(c *gc.C) {
	var tw loggo.TestWriter
	c.Assert(loggo.RegisterWriter("filter-tester", &tw, loggo.TRACE), gc.IsNil)
	defer loggo.RemoveWriter("filter-tester")
	logger := loggo.GetLogger("juju.environs")
	defer logger.SetLogLevel(logger.LogLevel())
	logger.SetLogLevel(loggo.TRACE)

	_, err := envtools.FindTools(
		s.env, 1, -1, "released", coretools.Filter{Number: version.Number{Major: 1, Minor: 2, Patch: 3}})
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
	// This is slightly overly prescriptive, but feel free to change or add
	// messages. This still helps to ensure that all log messages are
	// properly formed.
	messages := []jc.SimpleMessage{
		{loggo.INFO, "reading tools with major version 1"},
		{loggo.INFO, "filtering tools by version: \\d+\\.\\d+\\.\\d+"},
		{loggo.TRACE, "no architecture specified when finding tools, looking for "},
		{loggo.TRACE, "no series specified when finding tools, looking for \\[.*\\]"},
	}
	sources, err := envtools.GetMetadataSources(s.env)
	c.Assert(err, jc.ErrorIsNil)
	for i := 0; i < 2*len(sources); i++ {
		messages = append(messages,
			jc.SimpleMessage{loggo.TRACE, `fetchData failed for .*`},
			jc.SimpleMessage{loggo.TRACE, `cannot load index .*`})
	}
	c.Check(tw.Log(), jc.LogMatches, messages)
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
	info:   "exact match in public not blocked by custom",
	custom: envtesting.V110all,
	public: []version.Binary{envtesting.V100p64},
	seek:   envtesting.V100p64,
}}

func (s *SimpleStreamsToolsSuite) TestFindExactTools(c *gc.C) {
	for i, test := range findExactToolsTests {
		c.Logf("\ntest %d: %s", i, test.info)
		s.reset(c, nil)
		custom := s.uploadCustom(c, test.custom...)
		public := s.uploadPublic(c, test.public...)
		actual, err := envtools.FindExactTools(s.env, test.seek.Number, test.seek.Series, test.seek.Arch)
		if test.err == nil {
			if !c.Check(err, jc.ErrorIsNil) {
				continue
			}
			c.Check(actual.Version, gc.Equals, test.seek)
			if _, ok := custom[actual.Version]; ok {
				c.Check(actual.URL, gc.DeepEquals, custom[actual.Version])
			} else {
				c.Check(actual.URL, gc.DeepEquals, public[actual.Version])
			}
		} else {
			c.Check(err, jc.Satisfies, errors.IsNotFound)
		}
	}
}

var preferredStreamTests = []struct {
	explicitVers   string
	currentVers    string
	forceDevel     bool
	streamInConfig string
	expected       string
}{{
	currentVers:    "1.22.0",
	streamInConfig: "released",
	expected:       "released",
}, {
	currentVers:    "1.22.0",
	streamInConfig: "devel",
	expected:       "devel",
}, {
	currentVers: "1.22.0",
	expected:    "released",
}, {
	currentVers: "1.22-beta1",
	expected:    "devel",
}, {
	currentVers:    "1.22-beta1",
	streamInConfig: "released",
	expected:       "devel",
}, {
	currentVers:    "1.22-beta1",
	streamInConfig: "devel",
	expected:       "devel",
}, {
	currentVers: "1.22.0",
	forceDevel:  true,
	expected:    "devel",
}, {
	currentVers:  "1.22.0",
	explicitVers: "1.22-beta1",
	expected:     "devel",
}, {
	currentVers:  "1.22-bta1",
	explicitVers: "1.22.0",
	expected:     "released",
}}

func (s *SimpleStreamsToolsSuite) TestPreferredStream(c *gc.C) {
	for i, test := range preferredStreamTests {
		c.Logf("\ntest %d", i)
		origVers := version.Current
		version.Current.Number = version.MustParse(test.currentVers)
		var vers *version.Number
		if test.explicitVers != "" {
			v := version.MustParse(test.explicitVers)
			vers = &v
		}
		obtained := envtools.PreferredStream(vers, test.forceDevel, test.streamInConfig)
		c.Check(obtained, gc.Equals, test.expected)
		version.Current = origVers
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
		c.Check(err, jc.ErrorIsNil)
	}
}

func (s *ToolsListSuite) TestCheckToolsSeriesAcceptsMultipleForSameSeries(c *gc.C) {
	series := "quantal"
	list := fakeToolsList(series, series, series)
	err := envtools.CheckToolsSeries(list, series)
	c.Check(err, jc.ErrorIsNil)
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
