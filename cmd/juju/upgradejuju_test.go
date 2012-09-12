package main

import (
	"io/ioutil"
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/juju/testing"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/version"
	"strings"
)

type UpgradeJujuSuite struct {
	testing.JujuConnSuite
}

var _ = Suite(&UpgradeJujuSuite{})

var upgradeJujuTests = []struct {
	about          string
	private        []string
	public         []string
	currentVersion string
	agentVersion   string

	args              []string
	expectInitErr     string
	expectErr         string
	expectVersion     string
	expectDevelopment bool
	expectUploaded    string
}{{
	about:          "unwanted extra argument",
	currentVersion: "1.0.0-foo-bar",
	agentVersion:   "1.0.0",
	args:           []string{"foo"},
	expectInitErr:  "unrecognized args:.*",
}, {
	about:          "invalid --version value",
	currentVersion: "1.0.0-foo-bar",
	agentVersion:   "1.0.0",
	args:           []string{"--version", "invalid-version"},
	expectInitErr:  "invalid version .*",
}, {
	about:          "from private storage",
	private:        []string{"2.0.0-foo-bar", "2.0.2-foo-bletch", "2.0.3-foo-bar"},
	public:         []string{"2.0.0-foo-bar", "2.0.4-foo-bar", "2.0.5-foo-bar"},
	currentVersion: "2.0.0-foo-bar",
	agentVersion:   "2.0.0",
	expectVersion:  "2.0.2",
}, {
	about:          "current dev version, from private storage",
	private:        []string{"2.0.0-foo-bar", "2.0.2-foo-bar", "2.0.3-foo-bar", "3.0.1-foo-bar"},
	public:         []string{"2.0.0-foo-bar", "2.0.4-foo-bar", "2.0.5-foo-bar"},
	currentVersion: "2.0.1-foo-bar",
	agentVersion:   "2.0.1",
	expectVersion:  "2.0.3",
}, {
	about:             "dev version flag, from private storage",
	private:           []string{"2.0.0-foo-bar", "2.0.2-foo-bar", "2.0.3-foo-bar"},
	public:            []string{"2.0.0-foo-bar", "2.0.4-foo-bar", "2.0.5-foo-bar"},
	currentVersion:    "2.0.0-foo-bar",
	args:              []string{"--dev"},
	agentVersion:      "2.0.0",
	expectVersion:     "2.0.3",
	expectDevelopment: true,
}, {
	about:          "from public storage",
	public:         []string{"2.0.0-foo-bar", "2.0.2-arble-bletch", "2.0.3-foo-bar"},
	currentVersion: "2.0.0-foo-bar",
	agentVersion:   "2.0.0",
	expectVersion:  "2.0.2",
}, {
	about:          "current dev version, from public storage",
	public:         []string{"2.0.0-foo-bar", "2.0.2-arble-bletch", "2.0.3-foo-bar"},
	currentVersion: "2.0.1-foo-bar",
	agentVersion:   "2.0.1",
	expectVersion:  "2.0.3",
}, {
	about:             "dev version flag, from public storage",
	public:            []string{"2.0.0-foo-bar", "2.0.2-arble-bletch", "2.0.3-foo-bar"},
	currentVersion:    "2.0.0-foo-bar",
	args:              []string{"--dev"},
	agentVersion:      "2.0.0",
	expectVersion:     "2.0.3",
	expectDevelopment: true,
}, {
	about:          "specified version",
	currentVersion: "3.0.0-foo-bar",
	agentVersion:   "2.0.0",
	args:           []string{"--version", "2.0.3"},
	expectVersion:  "2.0.3",
}, {
	about:          "major version change",
	currentVersion: "2.0.0-foo-bar",
	agentVersion:   "4.1.2",
	args:           []string{"--version", "3.1.2"},
	expectErr:      "cannot upgrade major versions yet",
}, {
	about:          "upload",
	currentVersion: "2.0.2-foo-bar",
	agentVersion:   "2.0.0",
	args:           []string{"--upload-tools"},
	expectVersion:  "2.0.2",
	expectUploaded: "2.0.2-foo-bar",
}, {
	about:          "upload dev version, currently on release version",
	currentVersion: "2.0.1-foo-bar",
	agentVersion:   "2.0.0",
	args:           []string{"--upload-tools"},
	expectErr:      "cannot find newest version: no tools found",
	expectUploaded: "2.0.1-foo-bar",
},
}

func upload(s environs.Storage, v string) {
	vers := version.MustParseBinary(v)
	p := environs.ToolsStoragePath(vers)
	err := s.Put(p, strings.NewReader(v), int64(len(v)))
	if err != nil {
		panic(err)
	}
}

// testPutTools mocks environs.PutTools. This is an advantage in
// two ways:
// - we can make it return a tools with a version == version.Current.
// - we don't need to actually rebuild the juju source for each test
// that uses --upload-tools.
func testPutTools(storage environs.Storage, forceVersion *version.Binary) (*state.Tools, error) {
	vers := version.Current
	if forceVersion != nil {
		vers = *forceVersion
	}
	upload(storage, vers.String())
	return &state.Tools{
		Binary: vers,
	}, nil
}

func (s *UpgradeJujuSuite) TestUpgradeJuju(c *C) {
	oldVersion := version.Current
	putTools = testPutTools
	defer func() {
		version.Current = oldVersion
		putTools = environs.PutTools
	}()

	for i, test := range upgradeJujuTests {
		c.Logf("%d. %s", i, test.about)
		// Set up the test preconditions.
		s.Reset(c)
		for _, v := range test.private {
			upload(s.Conn.Environ.Storage(), v)
		}
		for _, v := range test.public {
			storage := s.Conn.Environ.PublicStorage().(environs.Storage)
			upload(storage, v)
		}
		version.Current = version.MustParseBinary(test.currentVersion)
		err := SetAgentVersion(s.State, version.MustParse(test.agentVersion), false)
		c.Assert(err, IsNil)

		// Run the command
		com := &UpgradeJujuCommand{}
		err = com.Init(newFlagSet(), test.args)
		if test.expectInitErr != "" {
			c.Check(err, ErrorMatches, test.expectInitErr)
			continue
		}
		err = com.Run(&cmd.Context{c.MkDir(), nil, ioutil.Discard, ioutil.Discard})
		if test.expectErr != "" {
			c.Check(err, ErrorMatches, test.expectErr)
			continue
		}
		c.Assert(err, IsNil)
		cfg, err := s.State.EnvironConfig()
		c.Check(err, IsNil)
		c.Check(cfg.AgentVersion(), Equals, version.MustParse(test.expectVersion))
		c.Check(cfg.Development(), Equals, test.expectDevelopment)

		if test.expectUploaded != "" {
			p := environs.ToolsStoragePath(version.MustParseBinary(test.expectUploaded))
			r, err := s.Conn.Environ.Storage().Get(p)
			c.Assert(err, IsNil)
			data, err := ioutil.ReadAll(r)
			c.Check(err, IsNil)
			c.Check(string(data), Equals, test.expectUploaded)
			r.Close()
		}
	}
}

// JujuConnSuite very helpfully uploads some default
// tools to the environment's storage. We don't want
// 'em there.
func (s *UpgradeJujuSuite) Reset(c *C) {
	s.JujuConnSuite.Reset(c)
	removeAll := func(storage environs.Storage) {
		names, err := storage.List("")
		c.Assert(err, IsNil)
		for _, name := range names {
			err := storage.Remove(name)
			c.Assert(err, IsNil)
		}
	}
	removeAll(s.Conn.Environ.Storage())
	removeAll(s.Conn.Environ.PublicStorage().(environs.Storage))
}

func (s *UpgradeJujuSuite) TestUpgradeJujuWithRealPutTools(c *C) {
	s.Reset(c)
	com := &UpgradeJujuCommand{}
	err := com.Init(newFlagSet(), []string{"--upload-tools", "--dev"})
	c.Assert(err, IsNil)
	err = com.Run(&cmd.Context{c.MkDir(), nil, ioutil.Discard, ioutil.Discard})
	c.Assert(err, IsNil)
	p := environs.ToolsStoragePath(version.Current)
	r, err := s.Conn.Environ.Storage().Get(p)
	c.Assert(err, IsNil)
	r.Close()
}
