package environs_test

import (
	"io/ioutil"
	. "launchpad.net/gocheck"
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

func (t *ToolsSuite) SetUpTest(c *C) {
	t.LoggingSuite.SetUpTest(c)
	env, err := environs.NewFromAttrs(map[string]interface{}{
		"name":            "test",
		"type":            "dummy",
		"state-server":    false,
		"authorized-keys": "i-am-a-key",
		"ca-cert":         testing.CACert,
		"ca-private-key":  "",
	})
	c.Assert(err, IsNil)
	t.env = env
	envtesting.RemoveTools(c, t.env.Storage())
	envtesting.RemoveTools(c, t.env.PublicStorage().(environs.Storage))
	t.dataDir = c.MkDir()
}

func (t *ToolsSuite) TearDownTest(c *C) {
	dummy.Reset()
	t.LoggingSuite.TearDownTest(c)
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

func (t *ToolsSuite) TestFindTools(c *C) {
	for i, tt := range findToolsTests {
		c.Logf("Test %d: %s", i, tt.summary)
		putNames(c, t.env, tt.contents, tt.publicContents)
		vers := version.Binary{
			Number: tt.version,
			Series: version.Current.Series,
			Arch:   version.Current.Arch,
		}
		tools, err := environs.FindTools(t.env, vers, tt.flags)
		if tt.err != "" {
			c.Assert(err, ErrorMatches, tt.err)
		} else {
			c.Assert(err, IsNil)
			assertURLContents(c, tools.URL, tt.expect)
		}
		t.env.Destroy(nil)
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

func (t *ToolsSuite) TestListTools(c *C) {
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

	putNames(c, t.env, testList, testList)

	for i, test := range listToolsTests {
		c.Logf("test %d", i)
		toolsList, err := environs.ListTools(t.env, test.major)
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
		},
	},
	vers:             binaryVersion("2.8.8-precise-amd64"),
	expect:           nil,
	expectDev:        newTools("2.2.3-precise-amd64", ""),
	expectHighest:    nil,
	expectDevHighest: newTools("2.2.3-precise-amd64", ""),
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
			newTools("0.2.1-precise-amd64", ""),
			newTools("0.4.2-precise-amd64", ""),
			newTools("0.4.3-precise-amd64", ""),
		},
	},
	vers:             binaryVersion("0.2.2-precise-amd64"),
	expect:           newTools("0.2.0-precise-amd64", ""),
	expectDev:        newTools("0.2.1-precise-amd64", ""),
	expectHighest:    newTools("0.4.2-precise-amd64", ""),
	expectDevHighest: newTools("0.4.3-precise-amd64", ""),
}, {
	// 11. check that version comparing is numeric, not alphabetical.
	list: &environs.ToolsList{
		Public: []*state.Tools{
			newTools("0.0.9-precise-amd64", ""),
			newTools("0.0.10-precise-amd64", ""),
			newTools("0.0.11-precise-amd64", ""),
		},
	},
	vers:             binaryVersion("0.0.98-precise-amd64"),
	expect:           newTools("0.0.10-precise-amd64", ""),
	expectDev:        newTools("0.0.11-precise-amd64", ""),
	expectHighest:    newTools("0.0.10-precise-amd64", ""),
	expectDevHighest: newTools("0.0.11-precise-amd64", ""),
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

func (t *ToolsSuite) TestBestTools(c *C) {
	for i, t := range bestToolsTests {
		c.Logf("test %d", i)
		tools := environs.BestTools(t.list, t.vers, environs.CompatVersion)
		c.Assert(tools, DeepEquals, t.expect)
		tools = environs.BestTools(t.list, t.vers, environs.DevVersion|environs.CompatVersion)
		c.Assert(tools, DeepEquals, t.expectDev)
		tools = environs.BestTools(t.list, t.vers, environs.HighestVersion|environs.CompatVersion)
		c.Assert(tools, DeepEquals, t.expectHighest)
		tools = environs.BestTools(t.list, t.vers, environs.DevVersion|environs.HighestVersion|environs.CompatVersion)
		c.Assert(tools, DeepEquals, t.expectDevHighest)
	}
}
