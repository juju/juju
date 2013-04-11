package environs_test

import (
	"io/ioutil"
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/constraints"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/dummy"
	envtesting "launchpad.net/juju-core/environs/testing"
	"launchpad.net/juju-core/environs/tools"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/testing"
	"launchpad.net/juju-core/version"
	"net/http"
	"strings"
)

type ToolsSuite struct {
	env environs.Environ
	testing.LoggingSuite
	dataDir string
}

var _ = Suite(&ToolsSuite{})

func (s *ToolsSuite) SetUpTest(c *C) {
	s.LoggingSuite.SetUpTest(c)
	env, err := environs.NewFromAttrs(map[string]interface{}{
		"name":            "test",
		"type":            "dummy",
		"state-server":    false,
		"authorized-keys": "i-am-a-key",
		"ca-cert":         testing.CACert,
		"ca-private-key":  "",
	})
	c.Assert(err, IsNil)
	s.env = env
	s.dataDir = c.MkDir()
	s.removeTools(c)
}

func (s *ToolsSuite) removeTools(c *C) {
	envtesting.RemoveTools(c, s.env.Storage())
	envtesting.RemoveTools(c, s.env.PublicStorage().(environs.Storage))
}

func (s *ToolsSuite) TearDownTest(c *C) {
	dummy.Reset()
	s.LoggingSuite.TearDownTest(c)
}

func toolsStorageName(vers string) string {
	return tools.StorageName(version.Binary{
		Number: version.MustParse(vers),
		Series: version.CurrentSeries(),
		Arch:   version.CurrentArch(),
	})
}

type toolsSpec struct {
	version string
	os      string
	arch    string
}

var findToolsTests = []struct {
	summary        string         // a summary of the test purpose.
	version        version.Number // version to assume is current for the test.
	flags          environs.ToolsSearchFlags
	contents       []string // names in private storage.
	publicContents []string // names in public storage.
	expect         string   // the name we expect to find (if no error).
	err            string   // the error we expect to find (if not blank).
}{{
	summary:  "current version should be satisfied by current tools path",
	version:  version.CurrentNumber(),
	flags:    environs.CompatVersion,
	contents: []string{tools.StorageName(version.Current)},
	expect:   tools.StorageName(version.Current),
}, {
	summary: "highest version of tools is chosen",
	version: version.MustParse("0.0.0"),
	flags:   environs.HighestVersion | environs.DevVersion | environs.CompatVersion,
	contents: []string{
		toolsStorageName("0.0.9"),
		toolsStorageName("0.1.9"),
	},
	expect: toolsStorageName("0.1.9"),
}, {
	summary: "fall back to public storage when nothing found in private",
	version: version.MustParse("1.0.2"),
	flags:   environs.DevVersion | environs.CompatVersion,
	contents: []string{
		toolsStorageName("0.0.9"),
	},
	publicContents: []string{
		toolsStorageName("1.0.0"),
		toolsStorageName("1.0.1"),
	},
	expect: "public-" + toolsStorageName("1.0.1"),
}, {
	summary: "always use private storage in preference to public storage",
	version: version.MustParse("1.9.0"),
	flags:   environs.DevVersion | environs.CompatVersion,
	contents: []string{
		toolsStorageName("1.0.2"),
	},
	publicContents: []string{
		toolsStorageName("1.0.9"),
	},
	expect: toolsStorageName("1.0.2"),
}, {
	summary: "mismatching series or architecture is ignored",
	version: version.MustParse("1.0.0"),
	flags:   environs.CompatVersion,
	contents: []string{
		tools.StorageName(version.Binary{
			Number: version.MustParse("1.9.9"),
			Series: "foo",
			Arch:   version.CurrentArch(),
		}),
		tools.StorageName(version.Binary{
			Number: version.MustParse("1.9.9"),
			Series: version.CurrentSeries(),
			Arch:   "foo",
		}),
		toolsStorageName("1.0.0"),
	},
	expect: toolsStorageName("1.0.0"),
},
}

// putNames puts a set of names into the environ's private
// and public storage. The data in the private storage is
// the name itself; in the public storage the name is preceded
// with "public-".
func putNames(c *C, env environs.Environ, private, public []string) {
	for _, name := range private {
		err := env.Storage().Put(name, strings.NewReader(name), int64(len(name)))
		c.Assert(err, IsNil)
	}
	// The contents of all files in the public storage is prefixed with "public-" so
	// that we can easily tell if we've got the right thing.
	for _, name := range public {
		data := "public-" + name
		storage := env.PublicStorage().(environs.Storage)
		err := storage.Put(name, strings.NewReader(data), int64(len(data)))
		c.Assert(err, IsNil)
	}
}

func (s *ToolsSuite) TestFindTools(c *C) {
	for i, test := range findToolsTests {
		c.Logf("Test %d: %s", i, test.summary)
		putNames(c, s.env, test.contents, test.publicContents)
		vers := version.Binary{
			Number: test.version,
			Series: version.Current.Series,
			Arch:   version.Current.Arch,
		}
		tools, err := environs.FindTools(s.env, vers, test.flags)
		if test.err != "" {
			c.Assert(err, ErrorMatches, test.err)
		} else {
			c.Assert(err, IsNil)
			assertURLContents(c, tools.URL, test.expect)
		}
		s.env.Destroy(nil)
	}
}

func binaryVersion(vers string) version.Binary {
	return version.MustParseBinary(vers)
}

func newTools(vers, url string) *state.Tools {
	return &state.Tools{
		Binary: binaryVersion(vers),
		URL:    url,
	}
}

func assertURLContents(c *C, url string, expect string) {
	resp, err := http.Get(url)
	c.Assert(err, IsNil)
	defer resp.Body.Close()
	data, err := ioutil.ReadAll(resp.Body)
	c.Assert(err, IsNil)
	c.Assert(string(data), Equals, expect)
}

var listToolsTests = []struct {
	major  int
	expect []string
}{{
	0,
	nil,
}, {
	1,
	[]string{"1.2.3-precise-i386"},
}, {
	2,
	[]string{"2.2.3-precise-amd64", "2.2.3-precise-i386", "2.2.4-precise-i386"},
}, {
	3,
	[]string{"3.2.1-quantal-amd64"},
}, {
	4,
	nil,
}}

func (s *ToolsSuite) TestListTools(c *C) {
	testList := []string{
		"foo",
		"tools/.tgz",
		"tools/juju-1.2.3-precise-i386.tgz",
		"tools/juju-2.2.3-precise-amd64.tgz",
		"tools/juju-2.2.3-precise-i386.tgz",
		"tools/juju-2.2.4-precise-i386.tgz",
		"tools/juju-2.2-precise-amd64.tgz",
		"tools/juju-3.2.1-quantal-amd64.tgz",
		"xtools/juju-2.2.3-precise-amd64.tgz",
	}

	putNames(c, s.env, testList, testList)

	for i, test := range listToolsTests {
		c.Logf("test %d", i)
		toolsList, err := environs.ListTools(s.env, test.major)
		c.Assert(err, IsNil)
		c.Assert(toolsList.Private, HasLen, len(test.expect))
		c.Assert(toolsList.Public, HasLen, len(test.expect))
		for i, t := range toolsList.Private {
			vers := binaryVersion(test.expect[i])
			c.Assert(t.Binary, Equals, vers)
			assertURLContents(c, t.URL, tools.StorageName(vers))
		}
		for i, t := range toolsList.Public {
			vers := binaryVersion(test.expect[i])
			c.Assert(t.Binary, Equals, vers)
			assertURLContents(c, t.URL, "public-"+tools.StorageName(vers))
		}
	}
}

var bestToolsTests = []struct {
	list             *environs.ToolsList
	vers             version.Binary
	flags            environs.ToolsSearchFlags
	expect           *state.Tools
	expectDev        *state.Tools
	expectHighest    *state.Tools
	expectDevHighest *state.Tools
}{{
	// 0. Check that we don't get anything from an empty list.
	list:   &environs.ToolsList{},
	vers:   binaryVersion("1.2.3-precise-amd64"),
	flags:  environs.DevVersion | environs.CompatVersion,
	expect: nil,
}, {
	// 1. Check that we can choose the same development version.
	list: &environs.ToolsList{
		Private: []*state.Tools{
			newTools("1.0.0-precise-amd64", ""),
		},
	},
	vers:             binaryVersion("1.0.0-precise-amd64"),
	expect:           newTools("1.0.0-precise-amd64", ""),
	expectDev:        newTools("1.0.0-precise-amd64", ""),
	expectHighest:    newTools("1.0.0-precise-amd64", ""),
	expectDevHighest: newTools("1.0.0-precise-amd64", ""),
}, {
	// 2. Check that major versions need to match.
	list: &environs.ToolsList{
		Private: []*state.Tools{
			newTools("2.0.0-precise-amd64", ""),
			newTools("6.0.0-precise-amd64", ""),
		},
	},
	vers: binaryVersion("4.0.0-precise-amd64"),
}, {
	// 3. Check that we can choose the same released version.
	list: &environs.ToolsList{
		Private: []*state.Tools{
			newTools("2.0.0-precise-amd64", ""),
		},
	},
	vers:             binaryVersion("2.0.0-precise-amd64"),
	expect:           newTools("2.0.0-precise-amd64", ""),
	expectDev:        newTools("2.0.0-precise-amd64", ""),
	expectHighest:    newTools("2.0.0-precise-amd64", ""),
	expectDevHighest: newTools("2.0.0-precise-amd64", ""),
}, {
	// 4. Check that different arch/series are ignored.
	list: &environs.ToolsList{
		Private: []*state.Tools{
			newTools("1.2.3-precise-amd64", ""),
			newTools("1.2.4-precise-amd64", ""),
			newTools("1.3.4-precise-amd64", ""),
			newTools("1.4.4-precise-i386", ""),
			newTools("1.4.5-quantal-i386", ""),
			newTools("2.2.3-precise-amd64", ""),
		},
	},
	vers:             binaryVersion("1.9.4-precise-amd64"),
	expect:           newTools("1.3.4-precise-amd64", ""),
	expectDev:        newTools("1.3.4-precise-amd64", ""),
	expectHighest:    newTools("1.3.4-precise-amd64", ""),
	expectDevHighest: newTools("1.3.4-precise-amd64", ""),
}, {
	// 5. Check that we can't upgrade to a dev version from
	// a release version if dev is false.
	list: &environs.ToolsList{
		Private: []*state.Tools{
			newTools("2.2.3-precise-amd64", ""),
			newTools("2.2.4-precise-amd64", ""),
			newTools("2.3.4-precise-amd64", ""),
			newTools("3.2.3-precise-amd64", ""),
		},
	},
	vers:             binaryVersion("2.8.8-precise-amd64"),
	expect:           newTools("2.2.4-precise-amd64", ""),
	expectDev:        newTools("2.3.4-precise-amd64", ""),
	expectHighest:    newTools("2.2.4-precise-amd64", ""),
	expectDevHighest: newTools("2.3.4-precise-amd64", ""),
}, {
	// 6. Check that we can upgrade to a release version from
	// a dev version if dev is false.
	list: &environs.ToolsList{
		Private: []*state.Tools{
			newTools("2.2.3-precise-amd64", ""),
			newTools("2.2.4-precise-amd64", ""),
			newTools("2.3.4-precise-amd64", ""),
			newTools("2.4.4-precise-amd64", ""),
			newTools("3.2.3-precise-amd64", ""),
		},
	},
	vers:             binaryVersion("2.8.8-precise-amd64"),
	expect:           newTools("2.4.4-precise-amd64", ""),
	expectDev:        newTools("2.4.4-precise-amd64", ""),
	expectHighest:    newTools("2.4.4-precise-amd64", ""),
	expectDevHighest: newTools("2.4.4-precise-amd64", ""),
}, {
	// 7. Check that a different minor version works ok.
	list: &environs.ToolsList{
		Private: []*state.Tools{
			newTools("1.2.3-precise-amd64", ""),
			newTools("1.2.4-precise-amd64", ""),
			newTools("1.3.4-precise-amd64", ""),
			newTools("1.4.4-precise-i386", ""),
			newTools("1.4.5-quantal-i386", ""),
			newTools("2.2.3-precise-amd64", ""),
			newTools("2.3.3-precise-amd64", ""),
		},
	},
	vers:             binaryVersion("2.8.8-precise-amd64"),
	expect:           newTools("2.2.3-precise-amd64", ""),
	expectDev:        newTools("2.3.3-precise-amd64", ""),
	expectHighest:    newTools("2.2.3-precise-amd64", ""),
	expectDevHighest: newTools("2.3.3-precise-amd64", ""),
}, {
	// 8. Check that the private tools are chosen even though
	// they have a lower version number.
	list: &environs.ToolsList{
		Private: []*state.Tools{
			newTools("1.2.2-precise-amd64", ""),
		},
		Public: []*state.Tools{
			newTools("1.2.4-precise-amd64", ""),
		},
	},
	vers:             binaryVersion("1.8.8-precise-amd64"),
	expect:           newTools("1.2.2-precise-amd64", ""),
	expectDev:        newTools("1.2.2-precise-amd64", ""),
	expectHighest:    newTools("1.2.2-precise-amd64", ""),
	expectDevHighest: newTools("1.2.2-precise-amd64", ""),
}, {
	// 9. Check that the public tools can be chosen when
	// there are no private tools.
	list: &environs.ToolsList{
		Public: []*state.Tools{
			newTools("1.2.4-precise-amd64", ""),
		},
	},
	vers:             binaryVersion("1.8.9-precise-amd64"),
	expect:           newTools("1.2.4-precise-amd64", ""),
	expectDev:        newTools("1.2.4-precise-amd64", ""),
	expectHighest:    newTools("1.2.4-precise-amd64", ""),
	expectDevHighest: newTools("1.2.4-precise-amd64", ""),
}, {
	// 10. One test giving different values for all flag combinations.
	list: &environs.ToolsList{
		Public: []*state.Tools{
			newTools("0.2.0-precise-amd64", ""),
			newTools("0.3.0-precise-amd64", ""),
			newTools("0.6.0-precise-amd64", ""),
			newTools("0.7.0-precise-amd64", ""),
		},
	},
	vers:             binaryVersion("0.4.2-precise-amd64"),
	expect:           newTools("0.2.0-precise-amd64", ""),
	expectDev:        newTools("0.3.0-precise-amd64", ""),
	expectHighest:    newTools("0.6.0-precise-amd64", ""),
	expectDevHighest: newTools("0.7.0-precise-amd64", ""),
}, {
	// 11. check that version comparing is numeric, not alphabetical.
	list: &environs.ToolsList{
		Public: []*state.Tools{
			newTools("0.9.0-precise-amd64", ""),
			newTools("0.10.0-precise-amd64", ""),
			newTools("0.11.0-precise-amd64", ""),
		},
	},
	vers:             binaryVersion("0.12.0-precise-amd64"),
	expect:           newTools("0.10.0-precise-amd64", ""),
	expectDev:        newTools("0.11.0-precise-amd64", ""),
	expectHighest:    newTools("0.10.0-precise-amd64", ""),
	expectDevHighest: newTools("0.11.0-precise-amd64", ""),
}, {
	// 12. check that minor version wins over patch version.
	list: &environs.ToolsList{
		Public: []*state.Tools{
			newTools("0.9.11-precise-amd64", ""),
			newTools("0.10.10-precise-amd64", ""),
			newTools("0.11.9-precise-amd64", ""),
		},
	},
	vers:             binaryVersion("0.10.10-precise-amd64"),
	expect:           newTools("0.10.10-precise-amd64", ""),
	expectDev:        newTools("0.10.10-precise-amd64", ""),
	expectHighest:    newTools("0.10.10-precise-amd64", ""),
	expectDevHighest: newTools("0.11.9-precise-amd64", ""),
},
}

func (s *ToolsSuite) TestBestTools(c *C) {
	for i, test := range bestToolsTests {
		c.Logf("test %d", i)
		tools := environs.BestTools(test.list, test.vers, environs.CompatVersion)
		c.Check(tools, DeepEquals, test.expect)
		tools = environs.BestTools(test.list, test.vers, environs.DevVersion|environs.CompatVersion)
		c.Check(tools, DeepEquals, test.expectDev)
		tools = environs.BestTools(test.list, test.vers, environs.HighestVersion|environs.CompatVersion)
		c.Check(tools, DeepEquals, test.expectHighest)
		tools = environs.BestTools(test.list, test.vers, environs.DevVersion|environs.HighestVersion|environs.CompatVersion)
		c.Check(tools, DeepEquals, test.expectDevHighest)
	}
}

var (
	v100    = version.MustParse("1.0.0")
	v100p64 = version.MustParseBinary("1.0.0-precise-amd64")
	v100p32 = version.MustParseBinary("1.0.0-precise-i386")
	v100q64 = version.MustParseBinary("1.0.0-quantal-amd64")
	v100q32 = version.MustParseBinary("1.0.0-quantal-i386")
	v100all = []version.Binary{v100p64, v100p32, v100q64, v100q32}

	v110    = version.MustParse("1.1.0")
	v110p64 = version.MustParseBinary("1.1.0-precise-amd64")
	v110p32 = version.MustParseBinary("1.1.0-precise-i386")
	v110p   = []version.Binary{v110p64, v110p32}

	v110q64 = version.MustParseBinary("1.1.0-quantal-amd64")
	v110q32 = version.MustParseBinary("1.1.0-quantal-i386")
	v110all = []version.Binary{v110p64, v110p32, v110q64, v110q32}

	v120    = version.MustParse("1.2.0")
	v120p64 = version.MustParseBinary("1.2.0-precise-amd64")
	v120p32 = version.MustParseBinary("1.2.0-precise-i386")
	v120q64 = version.MustParseBinary("1.2.0-quantal-amd64")
	v120q32 = version.MustParseBinary("1.2.0-quantal-i386")
	v120all = []version.Binary{v120p64, v120p32, v120q64, v120q32}

	v220    = version.MustParse("2.2.0")
	v220p32 = version.MustParseBinary("2.2.0-precise-i386")
	v220p64 = version.MustParseBinary("2.2.0-precise-amd64")
	v220q32 = version.MustParseBinary("2.2.0-quantal-i386")
	v220q64 = version.MustParseBinary("2.2.0-quantal-amd64")
	v220all = []version.Binary{v220p64, v220p32, v220q64, v220q32}
)

func (s *ToolsSuite) uploadPrivate(c *C, vers version.Binary) *state.Tools {
	return envtesting.UploadFakeToolsVersion(c, s.env.Storage(), vers)
}

func (s *ToolsSuite) uploadPublic(c *C, vers version.Binary) *state.Tools {
	storage := s.env.PublicStorage().(environs.Storage)
	return envtesting.UploadFakeToolsVersion(c, storage, vers)
}

var findAvailableToolsTests = []struct {
	info    string
	major   int
	private []version.Binary
	public  []version.Binary
	expect  []version.Binary
	err     error
}{{
	info:  "none available anywhere",
	major: 1,
	err:   tools.ErrNoTools,
}, {
	info:    "private tools only, none matching",
	major:   1,
	private: v220all,
	err:     tools.ErrNoMatches,
}, {
	info:    "tools found in private bucket",
	major:   1,
	private: v110all,
	expect:  v110all,
}, {
	info:   "private tools only, none matching",
	major:  1,
	public: v220all,
	err:    tools.ErrNoMatches,
}, {
	info:   "tools found in public bucket",
	major:  1,
	public: v110all,
	expect: v110all,
}, {
	info:    "tools found in both buckets, only taken from private",
	major:   1,
	private: v110p,
	public:  v110all,
	expect:  v110p,
}, {
	info:    "private tools completely block public ones",
	major:   1,
	private: v220all,
	public:  v110all,
	err:     tools.ErrNoMatches,
}}

func (s *ToolsSuite) TestFindAvailableTools(c *C) {
	for i, test := range findAvailableToolsTests {
		c.Logf("test %d: %s", i, test.info)
		s.removeTools(c)
		private := map[version.Binary]string{}
		for _, vers := range test.private {
			private[vers] = s.uploadPrivate(c, vers).URL
		}
		public := map[version.Binary]string{}
		for _, vers := range test.public {
			public[vers] = s.uploadPublic(c, vers).URL
		}
		actual, err := environs.FindAvailableTools(s.env, test.major)
		if test.err != nil {
			if len(actual) > 0 {
				c.Logf(actual.String())
			}
			c.Check(err, Equals, test.err)
			continue
		}
		expect := private
		if len(expect) == 0 {
			// We only use the public bucket if the private one has *no* tools.
			expect = public
		}
		c.Check(actual.URLs(), DeepEquals, expect)
	}
}

var findBootstrapToolsTests = []struct {
	info         string
	available    []version.Binary
	cliVersion   version.Number
	agentVersion version.Number
	constraints  constraints.Value
	expect       []version.Binary
	err          error
}{{
	info:       "released cli version, better released tools",
	available:  v120all,
	cliVersion: v100,
	expect:     v120all,
}, {
	info:       "released cli version, matching released tools",
	available:  v100all,
	cliVersion: v100,
	expect:     v100all,
}, {
	info:       "released cli version, only worse released tools",
	available:  v120all,
	cliVersion: v100,
	expect:     v120all,
}, {
	info: "released cli version, only dev tools",
}, {
	info: "released cli version, better dev tools",
}, {
	info: "released cli version, only worse dev tools",
}, {
	info: "released cli version with dev flag, only dev tools",
}, {
	info: "released cli version with dev flag, better dev tools",
}, {
	info: "released cli version with dev flag, only worse dev tools",
}, {
	info: "dev cli version, better released tools",
}, {
	info: "dev cli version, matching released tools",
}, {
	info: "dev cli version, only worse released tools",
}, {
	info: "dev cli version, better dev tools",
}, {
	info: "dev cli version, matching dev tools",
}, {
	info: "dev cli version, only worse dev tools",
}}

func (s *ToolsSuite) TestFindBootstrapTools(c *C) {
	for i, test := range findBootstrapToolsTests {
		c.Logf("test %d: %s", i, test.info)
		s.removeTools(c)
		c.Fatalf("BLAM")
	}
}
