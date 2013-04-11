package main

import (
	"io/ioutil"
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/environs"
	envtesting "launchpad.net/juju-core/environs/testing"
	"launchpad.net/juju-core/environs/tools"
	"launchpad.net/juju-core/juju/testing"
	"launchpad.net/juju-core/state"
	coretesting "launchpad.net/juju-core/testing"
	"launchpad.net/juju-core/version"
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
	args:           []string{"foo"},
	expectInitErr:  "unrecognized args:.*",
}, {
	about:          "invalid --version value",
	currentVersion: "1.0.0-foo-bar",
	args:           []string{"--version", "invalid-version"},
	expectInitErr:  "invalid version .*",
}, {
	about:          "major version upgrade to incompatible version",
	currentVersion: "2.0.0-foo-bar",
	args:           []string{"--version", "5.2.0"},
	expectInitErr:  "cannot upgrade to incompatible version",
}, {
	about:          "major version downgrade to incompatible version",
	currentVersion: "4.2.0-foo-bar",
	args:           []string{"--version", "3.2.0"},
	expectInitErr:  "cannot upgrade to incompatible version",
}, {
	about:          "from private storage",
	private:        []string{"2.0.0-foo-bar", "2.0.2-foo-bletch", "2.0.3-foo-bar"},
	public:         []string{"2.0.0-foo-bar", "2.0.4-foo-bar", "2.0.5-foo-bar"},
	currentVersion: "2.0.0-foo-bar",
	agentVersion:   "2.0.0",
	expectVersion:  "2.0.3",
}, {
	about:          "current dev version, from private storage",
	private:        []string{"2.0.0-foo-bar", "2.2.0-foo-bar", "2.3.0-foo-bar", "3.0.1-foo-bar"},
	public:         []string{"2.0.0-foo-bar", "2.4.0-foo-bar", "2.5.0-foo-bar"},
	currentVersion: "2.1.0-foo-bar",
	agentVersion:   "2.1.0",
	expectVersion:  "2.3.0",
}, {
	about:             "dev version flag, from private storage",
	private:           []string{"2.0.0-foo-bar", "2.2.0-foo-bar", "2.3.0-foo-bar"},
	public:            []string{"2.0.0-foo-bar", "2.4.0-foo-bar", "2.5.0-foo-bar"},
	currentVersion:    "2.0.0-foo-bar",
	args:              []string{"--dev"},
	agentVersion:      "2.0.0",
	expectVersion:     "2.3.0",
	expectDevelopment: true,
}, {
	about:          "from public storage",
	public:         []string{"2.0.0-foo-bar", "2.2.0-arble-bletch", "2.3.0-foo-bar"},
	currentVersion: "2.0.0-foo-bar",
	agentVersion:   "2.0.0",
	expectVersion:  "2.2.0",
}, {
	about:          "current dev version, from public storage",
	public:         []string{"2.0.0-foo-bar", "2.2.0-arble-bletch", "2.3.0-foo-bar"},
	currentVersion: "2.1.0-foo-bar",
	agentVersion:   "2.1.0",
	expectVersion:  "2.3.0",
}, {
	about:             "dev version flag, from public storage",
	public:            []string{"2.0.0-foo-bar", "2.2.0-arble-bletch", "2.3.0-foo-bar"},
	currentVersion:    "2.0.0-foo-bar",
	args:              []string{"--dev"},
	agentVersion:      "2.0.0",
	expectVersion:     "2.3.0",
	expectDevelopment: true,
}, {
	about:          "specified version",
	public:         []string{"2.3.0-foo-bar"},
	currentVersion: "3.0.0-foo-bar",
	agentVersion:   "2.0.0",
	args:           []string{"--version", "2.3.0"},
	expectVersion:  "2.3.0",
}, {
	about:          "specified version missing, but already set",
	currentVersion: "3.0.0-foo-bar",
	agentVersion:   "3.0.0",
	args:           []string{"--version", "3.0.0"},
}, {
	about:          "specified version missing",
	currentVersion: "3.0.0-foo-bar",
	agentVersion:   "3.0.0",
	args:           []string{"--version", "3.2.0"},
	expectErr:      "no matching tools available",
}, {
	about:          "major version downgrade to compatible version",
	private:        []string{"3.2.0-foo-bar"},
	currentVersion: "3.2.0-foo-bar",
	agentVersion:   "4.2.0",
	args:           []string{"--version", "3.2.0"},
	expectErr:      "cannot downgrade major version from 4 to 3",
}, {
	about:          "major version upgrade to compatible version",
	private:        []string{"3.2.0-foo-bar"},
	currentVersion: "3.2.0-foo-bar",
	agentVersion:   "2.8.2",
	args:           []string{"--version", "3.2.0"},
	expectErr:      "major version upgrades are not supported yet",
}, {
	about:          "upload",
	currentVersion: "2.2.0-foo-bar",
	agentVersion:   "2.0.0",
	args:           []string{"--upload-tools"},
	expectVersion:  "2.2.0",
	expectUploaded: "2.2.0-foo-bar",
}, {
	about:          "upload dev version, currently on release version",
	currentVersion: "2.1.0-foo-bar",
	agentVersion:   "2.0.0",
	args:           []string{"--upload-tools"},
	expectVersion:  "2.1.0",
	expectUploaded: "2.1.0-foo-bar",
}, {
	about:          "upload and bump version",
	private:        []string{"2.4.6-foo-bar", "2.4.8-foo-bar"},
	public:         []string{"2.4.10-foo-bar"},
	currentVersion: "2.4.6-foo-bar",
	agentVersion:   "2.4.0",
	args:           []string{"--upload-tools", "--bump-version"},
	expectVersion:  "2.4.6.1",
	expectUploaded: "2.4.6.1-foo-bar",
}, {
	about:          "upload with previously bumped version",
	private:        []string{"2.4.6-foo-bar", "2.4.6.1-foo-bar", "2.4.8-foo-bar"},
	public:         []string{"2.4.10-foo-bar"},
	currentVersion: "2.4.6-foo-bar",
	agentVersion:   "2.4.6.1",
	args:           []string{"--upload-tools", "--bump-version"},
	expectVersion:  "2.4.6.2",
	expectUploaded: "2.4.6.2-foo-bar",
},
}

// mockUploadTools simulates the effect of tools.Upload, but skips the time-
// consuming build from source. TODO(fwereade) better factor environs/tools
// such that build logic is exposed and can itself be neatly mocked?
func mockUploadTools(putter tools.URLPutter, forceVersion *version.Number, fakeSeries ...string) (*state.Tools, error) {
	storage := putter.(environs.Storage)
	vers := version.Current
	if forceVersion != nil {
		vers.Number = *forceVersion
	}
	t := envtesting.MustUploadFakeToolsVersion(storage, vers)
	for _, series := range fakeSeries {
		vers.Series = series
		envtesting.MustUploadFakeToolsVersion(storage, vers)
	}
	return t, nil
}

func (s *UpgradeJujuSuite) TestUpgradeJuju(c *C) {
	oldVersion := version.Current
	uploadTools = mockUploadTools
	defer func() {
		version.Current = oldVersion
		uploadTools = tools.Upload
	}()

	for i, test := range upgradeJujuTests {
		c.Logf("\ntest %d: %s", i, test.about)
		s.Reset(c)

		// Set up apparent CLI version and initialize the command.
		version.Current = version.MustParseBinary(test.currentVersion)
		com := &UpgradeJujuCommand{}
		if err := coretesting.InitCommand(com, test.args); err != nil {
			if test.expectInitErr != "" {
				c.Check(err, ErrorMatches, test.expectInitErr)
			} else {
				c.Check(err, IsNil)
			}
			continue
		}

		// Set up environ/state and run the command.
		err := SetAgentVersion(s.State, version.MustParse(test.agentVersion), false)
		c.Assert(err, IsNil)
		for _, v := range test.private {
			vers := version.MustParseBinary(v)
			envtesting.MustUploadFakeToolsVersion(s.Conn.Environ.Storage(), vers)
		}
		for _, v := range test.public {
			vers := version.MustParseBinary(v)
			storage := s.Conn.Environ.PublicStorage().(environs.Storage)
			envtesting.MustUploadFakeToolsVersion(storage, vers)
		}
		if err := com.Run(coretesting.Context(c)); err != nil {
			if test.expectErr != "" {
				c.Check(err, ErrorMatches, test.expectErr)
			} else {
				c.Check(err, IsNil)
			}
			continue
		}

		// Check expected changes to environ/state.
		cfg, err := s.State.EnvironConfig()
		c.Check(err, IsNil)
		agentVersion, ok := cfg.AgentVersion()
		c.Check(ok, Equals, true)
		c.Check(agentVersion, Equals, version.MustParse(test.expectVersion))
		c.Check(cfg.Development(), Equals, test.expectDevelopment)

		if test.expectUploaded != "" {
			vers := version.MustParseBinary(test.expectUploaded)
			r, err := s.Conn.Environ.Storage().Get(tools.StorageName(vers))
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
	envtesting.RemoveTools(c, s.Conn.Environ.Storage())
	envtesting.RemoveTools(c, s.Conn.Environ.PublicStorage().(environs.Storage))
}

func (s *UpgradeJujuSuite) TestUpgradeJujuWithRealUpload(c *C) {
	s.Reset(c)
	_, err := coretesting.RunCommand(c, &UpgradeJujuCommand{}, []string{"--upload-tools"})
	c.Assert(err, IsNil)
	name := tools.StorageName(version.Current)
	r, err := s.Conn.Environ.Storage().Get(name)
	c.Assert(err, IsNil)
	r.Close()
}
