// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package tools_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/juju/errors"
	"github.com/juju/loggo/v2"
	"github.com/juju/tc"
	"github.com/juju/utils/v4"

	"github.com/juju/juju/core/semversion"
	jujuversion "github.com/juju/juju/core/version"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/bootstrap"
	"github.com/juju/juju/environs/simplestreams"
	sstesting "github.com/juju/juju/environs/simplestreams/testing"
	envtesting "github.com/juju/juju/environs/testing"
	envtools "github.com/juju/juju/environs/tools"
	toolstesting "github.com/juju/juju/environs/tools/testing"
	_ "github.com/juju/juju/internal/provider/dummy"
	coretesting "github.com/juju/juju/internal/testing"
	coretools "github.com/juju/juju/internal/tools"
	"github.com/juju/juju/juju/keys"
	"github.com/juju/juju/jujuclient"
)

type SimpleStreamsToolsSuite struct {
	env environs.Environ
	coretesting.BaseSuite
	envtesting.ToolsFixture
	origCurrentVersion semversion.Number
	customToolsDir     string
	publicToolsDir     string
}

func TestSimpleStreamsToolsSuite(t *testing.T) {
	tc.Run(t, &SimpleStreamsToolsSuite{})
}

func (s *SimpleStreamsToolsSuite) SetUpSuite(c *tc.C) {
	s.BaseSuite.SetUpSuite(c)
	s.customToolsDir = c.MkDir()
	s.publicToolsDir = c.MkDir()
	s.PatchValue(&keys.JujuPublicKey, sstesting.SignedMetadataPublicKey)
}

func (s *SimpleStreamsToolsSuite) SetUpTest(c *tc.C) {
	s.ToolsFixture.DefaultBaseURL = utils.MakeFileURL(s.publicToolsDir)
	s.BaseSuite.SetUpTest(c)
	s.ToolsFixture.SetUpTest(c)
	s.origCurrentVersion = jujuversion.Current
	s.reset(c, nil)
}

func (s *SimpleStreamsToolsSuite) TearDownTest(c *tc.C) {
	jujuversion.Current = s.origCurrentVersion
	s.ToolsFixture.TearDownTest(c)
	s.BaseSuite.TearDownTest(c)
}

func (s *SimpleStreamsToolsSuite) reset(c *tc.C, attrs map[string]interface{}) {
	final := map[string]interface{}{
		"agent-metadata-url": utils.MakeFileURL(s.customToolsDir),
		"agent-stream":       "proposed",
	}
	for k, v := range attrs {
		final[k] = v
	}
	s.resetEnv(c, final)
}

func (s *SimpleStreamsToolsSuite) removeTools(c *tc.C) {
	for _, dir := range []string{s.customToolsDir, s.publicToolsDir} {
		files, err := os.ReadDir(dir)
		c.Assert(err, tc.ErrorIsNil)
		for _, f := range files {
			err := os.RemoveAll(filepath.Join(dir, f.Name()))
			c.Assert(err, tc.ErrorIsNil)
		}
	}
}

func (s *SimpleStreamsToolsSuite) uploadCustom(c *tc.C, verses ...semversion.Binary) map[semversion.Binary]string {
	return toolstesting.UploadToDirectory(c, s.customToolsDir, toolstesting.StreamVersions{"proposed": verses})["proposed"]
}

func (s *SimpleStreamsToolsSuite) uploadPublic(c *tc.C, verses ...semversion.Binary) map[semversion.Binary]string {
	return toolstesting.UploadToDirectory(c, s.publicToolsDir, toolstesting.StreamVersions{"proposed": verses})["proposed"]
}

func (s *SimpleStreamsToolsSuite) uploadStreams(c *tc.C, versions toolstesting.StreamVersions) map[string]map[semversion.Binary]string {
	return toolstesting.UploadToDirectory(c, s.publicToolsDir, versions)
}

func (s *SimpleStreamsToolsSuite) resetEnv(c *tc.C, attrs map[string]interface{}) {
	jujuversion.Current = s.origCurrentVersion
	attrs = coretesting.FakeConfig().Merge(attrs)
	env, err := bootstrap.PrepareController(false, envtesting.BootstrapContext(c.Context(), c),
		jujuclient.NewMemStore(),
		bootstrap.PrepareParams{
			ControllerConfig: coretesting.FakeControllerConfig(),
			ControllerName:   attrs["name"].(string),
			ModelConfig:      attrs,
			Cloud:            coretesting.FakeCloudSpec(),
			AdminSecret:      "admin-secret",
		},
	)
	c.Assert(err, tc.ErrorIsNil)
	s.env = env.(environs.Environ)
	s.removeTools(c)
}

var findToolsTests = []struct {
	info   string
	major  int
	minor  int
	custom []semversion.Binary
	public []semversion.Binary
	expect []semversion.Binary
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
	public: envtesting.V110p,
	expect: envtesting.V110p,
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

func (s *SimpleStreamsToolsSuite) TestFindTools(c *tc.C) {
	ss := simplestreams.NewSimpleStreams(sstesting.TestDataSourceFactory())
	for i, test := range findToolsTests {
		c.Logf("\ntest %d: %s", i, test.info)
		s.reset(c, nil)
		custom := s.uploadCustom(c, test.custom...)
		public := s.uploadPublic(c, test.public...)
		streams := envtools.PreferredStreams(&jujuversion.Current, s.env.Config().Development(), s.env.Config().AgentStream())
		actual, err := envtools.FindTools(c.Context(), ss, s.env, test.major, test.minor, streams, coretools.Filter{})
		if test.err != nil {
			if len(actual) > 0 {
				c.Logf("%s", actual.String())
			}
			c.Check(err, tc.ErrorIs, errors.NotFound)
			continue
		}
		expect := map[semversion.Binary][]string{}
		for _, expected := range test.expect {
			// If the tools exist in custom, that's preferred.
			url, ok := custom[expected]
			if !ok {
				url = public[expected]
			}
			expect[expected] = append(expect[expected], url)
		}
		c.Check(actual.URLs(), tc.DeepEquals, expect)
	}
}

func (s *SimpleStreamsToolsSuite) TestFindToolsFiltering(c *tc.C) {
	var tw loggo.TestWriter
	c.Assert(loggo.RegisterWriter("filter-tester", &tw), tc.IsNil)
	defer loggo.RemoveWriter("filter-tester")
	logger := loggo.GetLogger("juju.environs")
	defer logger.SetLogLevel(logger.LogLevel())
	logger.SetLogLevel(loggo.TRACE)

	ss := simplestreams.NewSimpleStreams(sstesting.TestDataSourceFactory())
	_, err := envtools.FindTools(c.Context(), ss,
		s.env, 1, -1, []string{"released"}, coretools.Filter{Number: semversion.Number{Major: 1, Minor: 2, Patch: 3}})
	c.Assert(err, tc.ErrorIs, errors.NotFound)
	// This is slightly overly prescriptive, but feel free to change or add
	// messages. This still helps to ensure that all log messages are
	// properly formed.
	messages := []loggo.Entry{
		{Level: loggo.DEBUG, Message: "reading agent binaries with major version 1"},
		{Level: loggo.DEBUG, Message: `filtering agent binaries by version: \d+\.\d+\.\d+`},
		{Level: loggo.TRACE, Message: "no architecture specified when finding agent binaries, looking for .*"},
		{Level: loggo.TRACE, Message: "no os type specified when finding agent binaries, looking for .*"},
	}
	sources, err := envtools.GetMetadataSources(s.env, ss)
	c.Assert(err, tc.ErrorIsNil)
	for i := 0; i < len(sources); i++ {
		messages = append(messages,
			loggo.Entry{Level: loggo.TRACE, Message: `fetchData failed for .*`},
			loggo.Entry{Level: loggo.DEBUG, Message: `cannot load index .*`})
	}

	mc := tc.NewMultiChecker()
	mc.AddExpr(`_.Level`, tc.Equals, tc.ExpectedValue)
	mc.AddExpr(`_.Message`, tc.Matches, tc.ExpectedValue)
	mc.AddExpr(`_._`, tc.Ignore)
	c.Assert(tw.Log(), tc.OrderedRight[[]loggo.Entry](mc), messages, tc.Commentf("log messages missing"))
}

var findExactToolsTests = []struct {
	info string
	// These are the contents of the proposed streams in each source.
	custom []semversion.Binary
	public []semversion.Binary
	seek   semversion.Binary
	err    error
}{{
	info: "nothing available",
	seek: envtesting.V100u64,
	err:  envtools.ErrNoTools,
}, {
	info:   "only non-matches available in custom",
	custom: append(envtesting.V110p, envtesting.V100u32, envtesting.V1001u64),
	seek:   envtesting.V100u64,
	err:    coretools.ErrNoMatches,
}, {
	info:   "exact match available in custom",
	custom: []semversion.Binary{envtesting.V100u64},
	seek:   envtesting.V100u64,
}, {
	info:   "only non-matches available in public",
	custom: append(envtesting.V110p, envtesting.V100u32, envtesting.V1001u64),
	seek:   envtesting.V100u64,
	err:    coretools.ErrNoMatches,
}, {
	info:   "exact match available in public",
	public: []semversion.Binary{envtesting.V100u64},
	seek:   envtesting.V100u64,
}, {
	info:   "exact match in public not blocked by custom",
	custom: envtesting.V110p,
	public: []semversion.Binary{envtesting.V100u64},
	seek:   envtesting.V100u64,
}}

func (s *SimpleStreamsToolsSuite) TestFindExactTools(c *tc.C) {
	ss := simplestreams.NewSimpleStreams(sstesting.TestDataSourceFactory())
	for i, test := range findExactToolsTests {
		c.Logf("\ntest %d: %s", i, test.info)
		s.reset(c, nil)
		custom := s.uploadCustom(c, test.custom...)
		public := s.uploadPublic(c, test.public...)
		actual, err := envtools.FindExactTools(c.Context(), ss, s.env, test.seek.Number, test.seek.Release, test.seek.Arch)
		if test.err == nil {
			if !c.Check(err, tc.ErrorIsNil) {
				continue
			}
			c.Check(actual.Version, tc.Equals, test.seek)
			if _, ok := custom[actual.Version]; ok {
				c.Check(actual.URL, tc.DeepEquals, custom[actual.Version])
			} else {
				c.Check(actual.URL, tc.DeepEquals, public[actual.Version])
			}
		} else {
			c.Check(err, tc.ErrorIs, errors.NotFound)
		}
	}
}

func copyAndAppend(vs []semversion.Binary, more ...[]semversion.Binary) []semversion.Binary {
	// TODO(babbageclunk): I think the append(someversions,
	// moreversions...) technique used in environs/testing/tools.go
	// might be wrong because it can mutate someversions if there's
	// enough capacity. Use this there.
	// https://medium.com/@Jarema./golang-slice-append-gotcha-e9020ff37374
	result := make([]semversion.Binary, len(vs))
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
	devel    []semversion.Binary
	proposed []semversion.Binary
	released []semversion.Binary
	expect   []semversion.Binary
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
	proposed: envtesting.V110p,
	released: envtesting.V100p,
	expect:   copyAndAppend(envtesting.V120all, envtesting.V100p),
}, {
	info:     "finds matching things everywhere",
	major:    1,
	minor:    2,
	streams:  []string{"devel", "proposed", "released"},
	devel:    []semversion.Binary{},
	proposed: []semversion.Binary{envtesting.V110u64, envtesting.V120u64},
	released: []semversion.Binary{envtesting.V100u64},
	expect:   []semversion.Binary{envtesting.V120u64},
}}

func (s *SimpleStreamsToolsSuite) TestFindToolsWithStreamFallback(c *tc.C) {
	ss := simplestreams.NewSimpleStreams(sstesting.TestDataSourceFactory())
	for i, test := range findToolsFallbackTests {
		c.Logf("\ntest %d: %s", i, test.info)
		s.reset(c, nil)
		streams := s.uploadStreams(c, toolstesting.StreamVersions{
			"devel":    test.devel,
			"proposed": test.proposed,
			"released": test.released,
		})
		actual, err := envtools.FindTools(c.Context(), ss,
			s.env, test.major, test.minor, test.streams, coretools.Filter{})
		if test.err != nil {
			if len(actual) > 0 {
				c.Logf("%s", actual.String())
			}
			c.Check(err, tc.ErrorIs, errors.NotFound)
			continue
		}
		expect := map[semversion.Binary][]string{}
		for _, expected := range test.expect {
			for _, stream := range []string{"devel", "proposed", "released"} {
				if url, ok := streams[stream][expected]; ok {
					expect[expected] = []string{url}
					break
				}
			}
		}
		c.Check(actual.URLs(), tc.DeepEquals, expect)
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

func (s *SimpleStreamsToolsSuite) TestPreferredStreams(c *tc.C) {
	for i, test := range preferredStreamTests {
		c.Logf("\ntest %d", i)
		s.PatchValue(&jujuversion.Current, semversion.MustParse(test.currentVers))
		var vers *semversion.Number
		if test.explicitVers != "" {
			v := semversion.MustParse(test.explicitVers)
			vers = &v
		}
		obtained := envtools.PreferredStreams(vers, test.forceDevel, test.streamInConfig)
		c.Check(obtained, tc.DeepEquals, test.expected)
	}
}

// fakeToolsForRelease fakes a Tools object with just enough information for
// testing the handling its OS type.
func fakeToolsForRelease(osType string) *coretools.Tools {
	return &coretools.Tools{Version: semversion.Binary{Release: osType}}
}

// fakeToolsList fakes a envtools.List containing Tools objects for the given
// respective os types, in the same number and order.
func fakeToolsList(releases ...string) coretools.List {
	list := coretools.List{}
	for _, name := range releases {
		list = append(list, fakeToolsForRelease(name))
	}
	return list
}

type ToolsListSuite struct{}

func TestToolsListSuite(t *testing.T) {
	tc.Run(t, &ToolsListSuite{})
}

func (s *ToolsListSuite) TestCheckToolsReleaseRequiresTools(c *tc.C) {
	err := envtools.CheckToolsReleases(fakeToolsList(), "ubuntu")
	c.Assert(err, tc.NotNil)
	c.Check(err, tc.ErrorMatches, "expected single os type, got \\[\\]")
}

func (s *ToolsListSuite) TestCheckToolsReleaseAcceptsOneSetOfTools(c *tc.C) {
	names := []string{"ubuntu", "windows"}
	for _, release := range names {
		list := fakeToolsList(release)
		err := envtools.CheckToolsReleases(list, release)
		c.Check(err, tc.ErrorIsNil)
	}
}

func (s *ToolsListSuite) TestCheckToolsReleaseAcceptsMultipleForSameOSType(c *tc.C) {
	osType := "ubuntu"
	list := fakeToolsList(osType, osType, osType)
	err := envtools.CheckToolsReleases(list, osType)
	c.Check(err, tc.ErrorIsNil)
}

func (s *ToolsListSuite) TestCheckToolsReleaseRejectsToolsForOthers(c *tc.C) {
	list := fakeToolsList("windows")
	err := envtools.CheckToolsReleases(list, "ubuntu")
	c.Assert(err, tc.NotNil)
	c.Check(err, tc.ErrorMatches, "agent binary mismatch: expected os type ubuntu, got windows")
}

func (s *ToolsListSuite) TestCheckToolsReleaseRejectsToolsForMixed(c *tc.C) {
	list := fakeToolsList("ubuntu", "windows")
	err := envtools.CheckToolsReleases(list, "ubuntu")
	c.Assert(err, tc.NotNil)
	c.Check(err, tc.ErrorMatches, "expected single os type, got .*")
}
