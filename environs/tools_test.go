package environs_test

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/agent"
	"launchpad.net/juju-core/environs/dummy"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/testing"
	"launchpad.net/juju-core/version"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type ToolsSuite struct {
	env environs.Environ
	testing.LoggingSuite
	dataDir string
}

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
	t.dataDir = c.MkDir()
}

func (t *ToolsSuite) TearDownTest(c *C) {
	dummy.Reset()
	t.LoggingSuite.TearDownTest(c)
}

var envs *environs.Environs

func toolsStoragePath(vers string) string {
	return environs.ToolsStoragePath(version.Binary{
		Number: version.MustParse(vers),
		Series: version.Current.Series,
		Arch:   version.Current.Arch,
	})
}

func mongoStoragePath(series, arch string) string {
	return environs.MongoStoragePath(version.Binary{
		Series: series,
		Arch:   arch,
	})
}

var _ = Suite(&ToolsSuite{})

const urlFile = "downloaded-url.txt"

var commandTests = []struct {
	cmd    []string
	output string
}{
	// TODO(niemeyer): Reintroduce this once we start deploying to the public bucket.
	//{
	//  []string{"juju", "arble"},
	//  "error: unrecognized command: juju arble\n",
	//},
	{
		[]string{"jujud", "arble"},
		"error: unrecognized command: jujud arble\n",
	},
}

func (t *ToolsSuite) TestPutGetTools(c *C) {
	tools, err := environs.PutTools(t.env.Storage(), nil)
	c.Assert(err, IsNil)
	c.Assert(tools.Binary, Equals, version.Current)
	c.Assert(tools.URL, Not(Equals), "")

	for i, get := range []func(dataDir string, t *state.Tools) error{
		getTools,
		getToolsWithTar,
	} {
		c.Logf("test %d", i)
		// Unarchive the tool executables into a temp directory.
		dataDir := c.MkDir()
		err = get(dataDir, tools)
		c.Assert(err, IsNil)

		dir := agent.SharedToolsDir(dataDir, version.Current)
		// Verify that each tool executes and produces some
		// characteristic output.
		for i, test := range commandTests {
			c.Logf("command test %d", i)
			out, err := exec.Command(filepath.Join(dir, test.cmd[0]), test.cmd[1:]...).CombinedOutput()
			if err != nil {
				c.Assert(err, FitsTypeOf, (*exec.ExitError)(nil))
			}
			c.Check(string(out), Matches, test.output)
		}
		data, err := ioutil.ReadFile(filepath.Join(dir, urlFile))
		c.Assert(err, IsNil)
		c.Assert(string(data), Equals, tools.URL)
	}
}

func (t *ToolsSuite) TestPutToolsFakeSeries(c *C) {
	tools, err := environs.PutTools(t.env.Storage(), nil, "sham", "fake")
	c.Assert(err, IsNil)
	c.Assert(tools.Binary, Equals, version.Current)
	expect := getToolsRaw(c, tools)

	for _, series := range []string{"sham", "fake", version.Current.Series} {
		vers := version.Current
		vers.Series = series
		tools, err := environs.FindTools(t.env, vers, environs.CompatVersion)
		c.Assert(err, IsNil)
		c.Assert(tools.Binary, Equals, vers)
		c.Assert(getToolsRaw(c, tools), DeepEquals, expect)
	}
}

func getToolsRaw(c *C, tools *state.Tools) []byte {
	resp, err := http.Get(tools.URL)
	c.Assert(err, IsNil)
	defer resp.Body.Close()
	c.Assert(resp.StatusCode, Equals, http.StatusOK)
	var buf bytes.Buffer
	_, err = io.Copy(&buf, resp.Body)
	c.Assert(err, IsNil)
	return buf.Bytes()
}

func (t *ToolsSuite) TestPutToolsAndForceVersion(c *C) {
	// This test actually tests three things:
	//   the writing of the FORCE-VERSION file;
	//   the reading of the FORCE-VERSION file by the version package;
	//   and the reading of the version from jujud.
	vers := version.Current
	vers.Patch++
	tools, err := environs.PutTools(t.env.Storage(), &vers.Number)
	c.Assert(err, IsNil)
	c.Assert(tools.Binary, Equals, vers)
}

// Test that the upload procedure fails correctly
// when the build process fails (because of a bad Go source
// file in this case).
func (t *ToolsSuite) TestUploadBadBuild(c *C) {
	gopath := c.MkDir()
	join := append([]string{gopath, "src"}, strings.Split("launchpad.net/juju-core/cmd/broken", "/")...)
	pkgdir := filepath.Join(join...)
	err := os.MkdirAll(pkgdir, 0777)
	c.Assert(err, IsNil)

	err = ioutil.WriteFile(filepath.Join(pkgdir, "broken.go"), []byte("nope"), 0666)
	c.Assert(err, IsNil)

	defer os.Setenv("GOPATH", os.Getenv("GOPATH"))
	os.Setenv("GOPATH", gopath)

	tools, err := environs.PutTools(t.env.Storage(), nil)
	c.Assert(tools, IsNil)
	c.Assert(err, ErrorMatches, `build command "go" failed: exit status 1; can't load package:(.|\n)*`)
}

func (t *ToolsSuite) toolsDir() string {
	return filepath.Join(t.dataDir, "tools")
}

func (t *ToolsSuite) TestToolsStoragePath(c *C) {
	c.Assert(environs.ToolsStoragePath(binaryVersion("1.2.3-precise-amd64")),
		Equals, "tools/juju-1.2.3-precise-amd64.tgz")
}

// getTools downloads and unpacks the given tools.
func getTools(dataDir string, tools *state.Tools) error {
	resp, err := http.Get(tools.URL)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("bad http status: %v", resp.Status)
	}
	return agent.UnpackTools(dataDir, tools, resp.Body)
}

// getToolsWithTar is the same as getTools but uses tar
// itself so we're not just testing the Go tar package against
// itself.
func getToolsWithTar(dataDir string, tools *state.Tools) error {
	resp, err := http.Get(tools.URL)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	dir := agent.SharedToolsDir(dataDir, tools.Binary)
	err = os.MkdirAll(dir, 0755)
	if err != nil {
		return err
	}

	cmd := exec.Command("tar", "xz")
	cmd.Dir = dir
	cmd.Stdin = resp.Body
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("tar extract failed: %s", out)
	}
	return ioutil.WriteFile(filepath.Join(cmd.Dir, urlFile), []byte(tools.URL), 0644)
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
	version:  version.Current.Number,
	flags:    environs.CompatVersion,
	contents: []string{environs.ToolsStoragePath(version.Current)},
	expect:   environs.ToolsStoragePath(version.Current),
}, {
	summary: "highest version of tools is chosen",
	version: version.MustParse("0.0.0"),
	flags:   environs.HighestVersion | environs.DevVersion | environs.CompatVersion,
	contents: []string{
		toolsStoragePath("0.0.9"),
		toolsStoragePath("0.1.9"),
	},
	expect: toolsStoragePath("0.1.9"),
}, {
	summary: "fall back to public storage when nothing found in private",
	version: version.MustParse("1.0.2"),
	flags:   environs.DevVersion | environs.CompatVersion,
	contents: []string{
		toolsStoragePath("0.0.9"),
	},
	publicContents: []string{
		toolsStoragePath("1.0.0"),
		toolsStoragePath("1.0.1"),
	},
	expect: "public-" + toolsStoragePath("1.0.1"),
}, {
	summary: "always use private storage in preference to public storage",
	version: version.MustParse("1.9.0"),
	flags:   environs.DevVersion | environs.CompatVersion,
	contents: []string{
		toolsStoragePath("1.0.2"),
	},
	publicContents: []string{
		toolsStoragePath("1.0.9"),
	},
	expect: toolsStoragePath("1.0.2"),
}, {
	summary: "mismatching series or architecture is ignored",
	version: version.MustParse("1.0.0"),
	flags:   environs.CompatVersion,
	contents: []string{
		environs.ToolsStoragePath(version.Binary{
			Number: version.MustParse("1.9.9"),
			Series: "foo",
			Arch:   version.Current.Arch,
		}),
		environs.ToolsStoragePath(version.Binary{
			Number: version.MustParse("1.9.9"),
			Series: version.Current.Series,
			Arch:   "foo",
		}),
		toolsStoragePath("1.0.0"),
	},
	expect: toolsStoragePath("1.0.0"),
},
}

// putNames puts a set of names into the environ's private
// and public storage. The data in the private storage is
// the name itself; in the public storage the name is preceded with "public-".
func putNames(c *C, env environs.Environ, private, public []string) {
	for _, name := range private {
		err := env.Storage().Put(name, strings.NewReader(name), int64(len(name)))
		c.Assert(err, IsNil)
	}
	// The contents of all files in the public storage is prefixed with "public-" so
	// that we can easily tell if we've got the right thing.
	for _, name := range public {
		data := "public-" + name
		err := env.PublicStorage().(environs.Storage).Put(name, strings.NewReader(data), int64(len(data)))
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

var mongoURLTests = []struct {
	summary        string   // a summary of the test purpose.
	contents       []string // names in private storage.
	publicContents []string // names in public storage.
	expect         string   // the name we expect to find (if no error).
	urlpart        string   // part of the url we expect to find (if not blank).
}{{
	summary:  "grab mongo from private storage if it exists there",
	contents: []string{environs.MongoStoragePath(version.Current)},
	expect:   environs.MongoStoragePath(version.Current),
}, {
	summary: "fall back to public storage when nothing found in private",
	contents: []string{
		environs.MongoStoragePath(version.Binary{
			Series: "foo",
			Arch:   version.Current.Arch}),
	},
	publicContents: []string{
		environs.MongoStoragePath(version.Current),
	},
	expect: "public-" + environs.MongoStoragePath(version.Current),
}, {
	summary: "if nothing in public or private storage, fall back to copy in ec2",
	contents: []string{
		environs.ToolsStoragePath(version.Binary{
			Series: "foo",
			Arch:   version.Current.Arch,
		}),
		environs.ToolsStoragePath(version.Binary{
			Series: version.Current.Series,
			Arch:   "foo",
		}),
	},
	publicContents: []string{
		environs.MongoStoragePath(version.Binary{
			Series: "foo",
			Arch:   version.Current.Arch,
		}),
	},
	urlpart: "http://juju-dist.s3.amazonaws.com",
},
}

func (t *ToolsSuite) TestMongoURL(c *C) {
	for i, tt := range mongoURLTests {
		c.Logf("Test %d: %s", i, tt.summary)
		putNames(c, t.env, tt.contents, tt.publicContents)
		vers := version.Binary{
			Series: version.Current.Series,
			Arch:   version.Current.Arch,
		}
		mongoURL := environs.MongoURL(t.env, vers)
		if tt.expect != "" {
			assertURLContents(c, mongoURL, tt.expect)
		}
		if tt.urlpart != "" {
			c.Assert(mongoURL, Matches, tt.urlpart+".*")
		}
		t.env.Destroy(nil)
		dummy.ResetPublicStorage(t.env)
	}
}

var setenvTests = []struct {
	set    string
	expect []string
}{
	{"foo=1", []string{"foo=1", "arble="}},
	{"foo=", []string{"foo=", "arble="}},
	{"arble=23", []string{"foo=bar", "arble=23"}},
	{"zaphod=42", []string{"foo=bar", "arble=", "zaphod=42"}},
}

func (*ToolsSuite) TestSetenv(c *C) {
	env0 := []string{"foo=bar", "arble="}
	for i, t := range setenvTests {
		c.Logf("test %d", i)
		env := make([]string, len(env0))
		copy(env, env0)
		env = environs.Setenv(env, t.set)
		c.Check(env, DeepEquals, t.expect)
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

	// dummy always populates the tools set with version.Current.
	// Remove any tools in the public storage to ensure they don't
	// conflict with the list of tools we expect.
	ps := t.env.PublicStorage().(environs.Storage)
	tools, err := ps.List("")
	c.Assert(err, IsNil)
	for _, tool := range tools {
		ps.Remove(tool)
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
			assertURLContents(c, t.URL, environs.ToolsStoragePath(vers))
		}
		for i, t := range toolsList.Public {
			vers := binaryVersion(test.expect[i])
			c.Assert(t.Binary, Equals, vers)
			assertURLContents(c, t.URL, "public-"+environs.ToolsStoragePath(vers))
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
