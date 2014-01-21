// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package tools_test

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"time"

	gc "launchpad.net/gocheck"
	"launchpad.net/loggo"

	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/environs/configstore"
	"launchpad.net/juju-core/environs/simplestreams"
	"launchpad.net/juju-core/environs/storage"
	envtesting "launchpad.net/juju-core/environs/testing"
	envtools "launchpad.net/juju-core/environs/tools"
	"launchpad.net/juju-core/errors"
	"launchpad.net/juju-core/provider/dummy"
	jc "launchpad.net/juju-core/testing/checkers"
	"launchpad.net/juju-core/testing/testbase"
	coretools "launchpad.net/juju-core/tools"
	"launchpad.net/juju-core/version"
)

type SimpleStreamsToolsSuite struct {
	env environs.Environ
	testbase.LoggingSuite
	envtesting.ToolsFixture
	origCurrentVersion version.Binary
	customToolsDir     string
	publicToolsDir     string
}

func setupToolsTests() {
	gc.Suite(&SimpleStreamsToolsSuite{})
}

func (s *SimpleStreamsToolsSuite) SetUpSuite(c *gc.C) {
	s.LoggingSuite.SetUpSuite(c)
	s.customToolsDir = c.MkDir()
	s.publicToolsDir = c.MkDir()
}

func (s *SimpleStreamsToolsSuite) SetUpTest(c *gc.C) {
	s.ToolsFixture.DefaultBaseURL = "file://" + s.publicToolsDir
	s.LoggingSuite.SetUpTest(c)
	s.ToolsFixture.SetUpTest(c)
	s.origCurrentVersion = version.Current
	s.reset(c, nil)
}

func (s *SimpleStreamsToolsSuite) TearDownTest(c *gc.C) {
	dummy.Reset()
	version.Current = s.origCurrentVersion
	s.ToolsFixture.TearDownTest(c)
	s.LoggingSuite.TearDownTest(c)
}

func (s *SimpleStreamsToolsSuite) reset(c *gc.C, attrs map[string]interface{}) {
	final := map[string]interface{}{
		"tools-metadata-url": "file://" + s.customToolsDir,
	}
	for k, v := range attrs {
		final[k] = v
	}
	s.resetEnv(c, final)
}

func (s *SimpleStreamsToolsSuite) removeTools(c *gc.C) {
	for _, dir := range []string{s.customToolsDir, s.publicToolsDir} {
		files, err := ioutil.ReadDir(dir)
		c.Assert(err, gc.IsNil)
		for _, f := range files {
			err := os.RemoveAll(filepath.Join(dir, f.Name()))
			c.Assert(err, gc.IsNil)
		}
	}
}

type metadataFile struct {
	path string
	data []byte
}

func (s *SimpleStreamsToolsSuite) generateMetadata(c *gc.C, verses ...version.Binary) []metadataFile {
	var metadata = make([]*envtools.ToolsMetadata, len(verses))
	for i, vers := range verses {
		basePath := fmt.Sprintf("releases/tools-%s.tar.gz", vers.String())
		metadata[i] = &envtools.ToolsMetadata{
			Release: vers.Series,
			Version: vers.Number.String(),
			Arch:    vers.Arch,
			Path:    basePath,
		}
	}
	index, products, err := envtools.MarshalToolsMetadataJSON(metadata, time.Now())
	c.Assert(err, gc.IsNil)
	objects := []metadataFile{
		{simplestreams.UnsignedIndex, index},
		{envtools.ProductMetadataPath, products},
	}
	return objects
}

func (s *SimpleStreamsToolsSuite) uploadToStorage(c *gc.C, stor storage.Storage, verses ...version.Binary) map[version.Binary]string {
	uploaded := map[version.Binary]string{}
	if len(verses) == 0 {
		return uploaded
	}
	var err error
	for _, vers := range verses {
		uploaded[vers], err = stor.URL(fmt.Sprintf("tools/releases/tools-%s.tar.gz", vers.String()))
		c.Assert(err, gc.IsNil)
	}
	objects := s.generateMetadata(c, verses...)
	for _, object := range objects {
		toolspath := path.Join("tools", object.path)
		err = stor.Put(toolspath, bytes.NewReader(object.data), int64(len(object.data)))
		c.Assert(err, gc.IsNil)
	}
	return uploaded
}

func (s *SimpleStreamsToolsSuite) uploadVersions(c *gc.C, dir string, verses ...version.Binary) map[version.Binary]string {
	uploaded := map[version.Binary]string{}
	if len(verses) == 0 {
		return uploaded
	}
	for _, vers := range verses {
		basePath := fmt.Sprintf("releases/tools-%s.tar.gz", vers.String())
		uploaded[vers] = fmt.Sprintf("file://%s/%s", dir, basePath)
	}
	objects := s.generateMetadata(c, verses...)
	for _, object := range objects {
		path := filepath.Join(dir, object.path)
		dir := filepath.Dir(path)
		if err := os.MkdirAll(dir, 0755); err != nil && !os.IsExist(err) {
			c.Assert(err, gc.IsNil)
		}
		err := ioutil.WriteFile(path, object.data, 0644)
		c.Assert(err, gc.IsNil)
	}
	return uploaded
}

func (s *SimpleStreamsToolsSuite) uploadCustom(c *gc.C, verses ...version.Binary) map[version.Binary]string {
	return s.uploadVersions(c, s.customToolsDir, verses...)
}

func (s *SimpleStreamsToolsSuite) uploadPublic(c *gc.C, verses ...version.Binary) map[version.Binary]string {
	return s.uploadVersions(c, s.publicToolsDir, verses...)
}

func (s *SimpleStreamsToolsSuite) resetEnv(c *gc.C, attrs map[string]interface{}) {
	version.Current = s.origCurrentVersion
	dummy.Reset()
	cfg, err := config.New(config.NoDefaults, dummy.SampleConfig().Merge(attrs))
	c.Assert(err, gc.IsNil)
	env, err := environs.Prepare(cfg, configstore.NewMem())
	c.Assert(err, gc.IsNil)
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
		actual, err := envtools.FindTools(s.env, test.major, test.minor, coretools.Filter{}, envtools.DoNotAllowRetry)
		if test.err != nil {
			if len(actual) > 0 {
				c.Logf(actual.String())
			}
			c.Check(err, jc.Satisfies, errors.IsNotFoundError)
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

func (s *SimpleStreamsToolsSuite) TestFindToolsInControlBucket(c *gc.C) {
	s.reset(c, nil)
	custom := s.uploadToStorage(c, s.env.Storage(), envtesting.V110p...)
	s.uploadPublic(c, envtesting.VAll...)
	actual, err := envtools.FindTools(s.env, 1, 1, coretools.Filter{}, envtools.DoNotAllowRetry)
	c.Assert(err, gc.IsNil)
	expect := map[version.Binary]string{}
	for _, expected := range envtesting.V110p {
		expect[expected] = custom[expected]
	}
	c.Assert(actual.URLs(), gc.DeepEquals, expect)
}

func (s *SimpleStreamsToolsSuite) TestFindToolsFiltering(c *gc.C) {
	tw := &loggo.TestWriter{}
	c.Assert(loggo.RegisterWriter("filter-tester", tw, loggo.DEBUG), gc.IsNil)
	defer loggo.RemoveWriter("filter-tester")
	_, err := envtools.FindTools(
		s.env, 1, -1, coretools.Filter{Number: version.Number{Major: 1, Minor: 2, Patch: 3}}, envtools.DoNotAllowRetry)
	c.Assert(err, jc.Satisfies, errors.IsNotFoundError)
	// This is slightly overly prescriptive, but feel free to change or add
	// messages. This still helps to ensure that all log messages are
	// properly formed.
	messages := []jc.SimpleMessage{
		{loggo.INFO, "reading tools with major version 1"},
		{loggo.INFO, "filtering tools by version: \\d+\\.\\d+\\.\\d+"},
		{loggo.DEBUG, "no architecture specified when finding tools, looking for any"},
		{loggo.DEBUG, "no series specified when finding tools, looking for any"},
	}
	sources, err := envtools.GetMetadataSources(s.env)
	c.Assert(err, gc.IsNil)
	for i := 0; i < 2*len(sources); i++ {
		messages = append(messages,
			jc.SimpleMessage{loggo.DEBUG, `fetchData failed for .*`},
			jc.SimpleMessage{loggo.DEBUG, `cannot load index .*`})
	}
	c.Check(tw.Log, jc.LogMatches, messages)
}

func (s *SimpleStreamsToolsSuite) TestFindBootstrapTools(c *gc.C) {
	// Remove the default tools URL from the search path, just look in cloud storage.
	s.PatchValue(&envtools.DefaultBaseURL, "")
	for i, test := range envtesting.BootstrapToolsTests {
		c.Logf("\ntest %d: %s", i, test.Info)
		attrs := map[string]interface{}{
			"development": test.Development,
		}
		var agentVersion *version.Number
		if test.AgentVersion != version.Zero {
			attrs["agent-version"] = test.AgentVersion.String()
			agentVersion = &test.AgentVersion
		}
		s.reset(c, attrs)
		version.Current = test.CliVersion
		available := s.uploadCustom(c, test.Available...)

		params := envtools.BootstrapToolsParams{
			Version: agentVersion,
			Series:  test.DefaultSeries,
			Arch:    &test.Arch,
		}
		actual, err := envtools.FindBootstrapTools(s.env, params)
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

func (s *SimpleStreamsToolsSuite) TestFindInstanceTools(c *gc.C) {
	for i, test := range findInstanceToolsTests {
		c.Logf("\ntest %d: %s", i, test.info)
		s.reset(c, map[string]interface{}{
			"agent-version": test.agentVersion.String(),
		})
		available := s.uploadCustom(c, test.available...)

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
			if !c.Check(err, gc.IsNil) {
				continue
			}
			c.Check(actual.Version, gc.Equals, test.seek)
			if _, ok := custom[actual.Version]; ok {
				c.Check(actual.URL, gc.DeepEquals, custom[actual.Version])
			} else {
				c.Check(actual.URL, gc.DeepEquals, public[actual.Version])
			}
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
