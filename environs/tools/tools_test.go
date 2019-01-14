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
	"github.com/juju/version"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/bootstrap"
	sstesting "github.com/juju/juju/environs/simplestreams/testing"
	envtesting "github.com/juju/juju/environs/testing"
	envtools "github.com/juju/juju/environs/tools"
	toolstesting "github.com/juju/juju/environs/tools/testing"
	"github.com/juju/juju/juju/keys"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/provider/dummy"
	coretesting "github.com/juju/juju/testing"
	coretools "github.com/juju/juju/tools"
	jujuversion "github.com/juju/juju/version"
)

type SimpleStreamsToolsSuite struct {
	env environs.Environ
	coretesting.BaseSuite
	envtesting.ToolsFixture
	origCurrentVersion version.Number
	customToolsDir     string
	publicToolsDir     string
}

func setupToolsTests() {
	gc.Suite(&SimpleStreamsToolsSuite{})
	gc.Suite(&ToolsListSuite{})
}

func (s *SimpleStreamsToolsSuite) SetUpSuite(c *gc.C) {
	s.BaseSuite.SetUpSuite(c)
	s.customToolsDir = c.MkDir()
	s.publicToolsDir = c.MkDir()
	s.PatchValue(&keys.JujuPublicKey, sstesting.SignedMetadataPublicKey)
}

func (s *SimpleStreamsToolsSuite) SetUpTest(c *gc.C) {
	s.ToolsFixture.DefaultBaseURL = utils.MakeFileURL(s.publicToolsDir)
	s.BaseSuite.SetUpTest(c)
	s.ToolsFixture.SetUpTest(c)
	s.origCurrentVersion = jujuversion.Current
	s.reset(c, nil)
}

func (s *SimpleStreamsToolsSuite) TearDownTest(c *gc.C) {
	dummy.Reset(c)
	jujuversion.Current = s.origCurrentVersion
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
	return toolstesting.UploadToDirectory(c, s.customToolsDir, toolstesting.StreamVersions{"proposed": verses})["proposed"]
}

func (s *SimpleStreamsToolsSuite) uploadPublic(c *gc.C, verses ...version.Binary) map[version.Binary]string {
	return toolstesting.UploadToDirectory(c, s.publicToolsDir, toolstesting.StreamVersions{"proposed": verses})["proposed"]
}

func (s *SimpleStreamsToolsSuite) uploadStreams(c *gc.C, versions toolstesting.StreamVersions) map[string]map[version.Binary]string {
	return toolstesting.UploadToDirectory(c, s.publicToolsDir, versions)
}

func (s *SimpleStreamsToolsSuite) resetEnv(c *gc.C, attrs map[string]interface{}) {
	jujuversion.Current = s.origCurrentVersion
	dummy.Reset(c)
	attrs = dummy.SampleConfig().Merge(attrs)
	env, err := bootstrap.PrepareController(false, envtesting.BootstrapContext(c),
		jujuclient.NewMemStore(),
		bootstrap.PrepareParams{
			ControllerConfig: coretesting.FakeControllerConfig(),
			ControllerName:   attrs["name"].(string),
			ModelConfig:      attrs,
			Cloud:            dummy.SampleCloudSpec(),
			AdminSecret:      "admin-secret",
		},
	)
	c.Assert(err, jc.ErrorIsNil)
	s.env = env.(environs.Environ)
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
		streams := envtools.PreferredStreams(&jujuversion.Current, s.env.Config().Development(), s.env.Config().AgentStream())
		actual, err := envtools.FindTools(s.env, test.major, test.minor, streams, coretools.Filter{})
		if test.err != nil {
			if len(actual) > 0 {
				c.Logf(actual.String())
			}
			c.Check(err, jc.Satisfies, errors.IsNotFound)
			continue
		}
		expect := map[version.Binary][]string{}
		for _, expected := range test.expect {
			// If the tools exist in custom, that's preferred.
			url, ok := custom[expected]
			if !ok {
				url = public[expected]
			}
			expect[expected] = append(expect[expected], url)
		}
		c.Check(actual.URLs(), gc.DeepEquals, expect)
	}
}

func (s *SimpleStreamsToolsSuite) TestFindToolsFiltering(c *gc.C) {
	var tw loggo.TestWriter
	c.Assert(loggo.RegisterWriter("filter-tester", &tw), gc.IsNil)
	defer loggo.RemoveWriter("filter-tester")
	logger := loggo.GetLogger("juju.environs")
	defer logger.SetLogLevel(logger.LogLevel())
	logger.SetLogLevel(loggo.TRACE)

	_, err := envtools.FindTools(
		s.env, 1, -1, []string{"released"}, coretools.Filter{Number: version.Number{Major: 1, Minor: 2, Patch: 3}})
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
	// This is slightly overly prescriptive, but feel free to change or add
	// messages. This still helps to ensure that all log messages are
	// properly formed.
	messages := []jc.SimpleMessage{
		{loggo.DEBUG, "reading agent binaries with major version 1"},
		{loggo.DEBUG, "filtering agent binaries by version: \\d+\\.\\d+\\.\\d+"},
		{loggo.TRACE, "no architecture specified when finding agent binaries, looking for "},
		{loggo.TRACE, "no series specified when finding agent binaries, looking for \\[.*\\]"},
	}
	sources, err := envtools.GetMetadataSources(s.env)
	c.Assert(err, jc.ErrorIsNil)
	for i := 0; i < len(sources); i++ {
		messages = append(messages,
			jc.SimpleMessage{loggo.TRACE, `fetchData failed for .*`},
			jc.SimpleMessage{loggo.TRACE, `cannot load index .*`})
	}
	c.Check(tw.Log(), jc.LogMatches, messages)
}

var findExactToolsTests = []struct {
	info string
	// These are the contents of the proposed streams in each source.
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

func copyAndAppend(vs []version.Binary, more ...[]version.Binary) []version.Binary {
	// TODO(babbageclunk): I think the append(someversions,
	// moreversions...) technique used in environs/testing/tools.go
	// might be wrong because it can mutate someversions if there's
	// enough capacity. Use this there.
	// https://medium.com/@Jarema./golang-slice-append-gotcha-e9020ff37374
	result := make([]version.Binary, len(vs))
	copy(result, vs)
	for _, items := range more {
		result = append(result, items...)
	}
	return result
}

var findToolsFallbackTests = []struct {
	info     string
	major    int
	minor    int
	streams  []string
	devel    []version.Binary
	proposed []version.Binary
	released []version.Binary
	expect   []version.Binary
	err      error
}{{
	info:    "nothing available",
	major:   1,
	streams: []string{"released"},
	err:     envtools.ErrNoTools,
}, {
	info:    "only available in non-selected stream",
	major:   1,
	minor:   2,
	streams: []string{"released"},
	devel:   envtesting.VAll,
	err:     coretools.ErrNoMatches,
}, {
	info:     "finds things in devel and released, ignores proposed",
	major:    1,
	minor:    -1,
	streams:  []string{"devel", "released"},
	devel:    envtesting.V120all,
	proposed: envtesting.V110all,
	released: envtesting.V100all,
	expect:   copyAndAppend(envtesting.V120all, envtesting.V100all),
}, {
	info:     "finds matching things everywhere",
	major:    1,
	minor:    2,
	streams:  []string{"devel", "proposed", "released"},
	devel:    []version.Binary{envtesting.V110q64, envtesting.V120q64},
	proposed: []version.Binary{envtesting.V110p64, envtesting.V120p64},
	released: []version.Binary{envtesting.V100p64, envtesting.V120t64},
	expect:   []version.Binary{envtesting.V120p64, envtesting.V120q64, envtesting.V120t64},
}}

func (s *SimpleStreamsToolsSuite) TestFindToolsWithStreamFallback(c *gc.C) {
	for i, test := range findToolsFallbackTests {
		c.Logf("\ntest %d: %s", i, test.info)
		s.reset(c, nil)
		streams := s.uploadStreams(c, toolstesting.StreamVersions{
			"devel":    test.devel,
			"proposed": test.proposed,
			"released": test.released,
		})
		actual, err := envtools.FindTools(s.env, test.major, test.minor, test.streams, coretools.Filter{})
		if test.err != nil {
			if len(actual) > 0 {
				c.Logf(actual.String())
			}
			c.Check(err, jc.Satisfies, errors.IsNotFound)
			continue
		}
		expect := map[version.Binary][]string{}
		for _, expected := range test.expect {
			for _, stream := range []string{"devel", "proposed", "released"} {
				if url, ok := streams[stream][expected]; ok {
					expect[expected] = []string{url}
					break
				}
			}
		}
		c.Check(actual.URLs(), gc.DeepEquals, expect)
	}
}

var preferredStreamTests = []struct {
	explicitVers   string
	currentVers    string
	forceDevel     bool
	streamInConfig string
	expected       []string
}{{
	currentVers:    "1.22.0",
	streamInConfig: "released",
	expected:       []string{"released"},
}, {
	currentVers:    "1.22.0",
	streamInConfig: "proposed",
	expected:       []string{"proposed", "released"},
}, {
	currentVers:    "1.22.0",
	streamInConfig: "devel",
	expected:       []string{"devel", "proposed", "released"},
}, {
	currentVers:    "1.22.0",
	streamInConfig: "testing",
	expected:       []string{"testing", "devel", "proposed", "released"},
}, {
	currentVers: "1.22.0",
	expected:    []string{"released"},
}, {
	currentVers: "1.22-beta1",
	expected:    []string{"devel", "proposed", "released"},
}, {
	currentVers:    "1.22-beta1",
	streamInConfig: "released",
	expected:       []string{"devel", "proposed", "released"},
}, {
	currentVers:    "1.22-beta1",
	streamInConfig: "devel",
	expected:       []string{"devel", "proposed", "released"},
}, {
	currentVers: "1.22.0",
	forceDevel:  true,
	expected:    []string{"devel", "proposed", "released"},
}, {
	currentVers:  "1.22.0",
	explicitVers: "1.22-beta1",
	expected:     []string{"devel", "proposed", "released"},
}, {
	currentVers:  "1.22-bta1",
	explicitVers: "1.22.0",
	expected:     []string{"released"},
}}

func (s *SimpleStreamsToolsSuite) TestPreferredStreams(c *gc.C) {
	for i, test := range preferredStreamTests {
		c.Logf("\ntest %d", i)
		s.PatchValue(&jujuversion.Current, version.MustParse(test.currentVers))
		var vers *version.Number
		if test.explicitVers != "" {
			v := version.MustParse(test.explicitVers)
			vers = &v
		}
		obtained := envtools.PreferredStreams(vers, test.forceDevel, test.streamInConfig)
		c.Check(obtained, gc.DeepEquals, test.expected)
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
	c.Check(err, gc.ErrorMatches, "agent binary mismatch: expected series warty, got hoary")
}

func (s *ToolsListSuite) TestCheckToolsSeriesRejectsToolsForMixedSeries(c *gc.C) {
	list := fakeToolsList("precise", "raring")
	err := envtools.CheckToolsSeries(list, "precise")
	c.Assert(err, gc.NotNil)
	c.Check(err, gc.ErrorMatches, "expected single series, got .*")
}
