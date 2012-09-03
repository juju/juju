package main

import (
	"bytes"
	"fmt"
	"io/ioutil"
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/juju/testing"
	"launchpad.net/juju-core/state"
	coretesting "launchpad.net/juju-core/testing"
	"launchpad.net/juju-core/version"
	"launchpad.net/tomb"
	"net/http"
	"path/filepath"
	"time"
)

var _ = Suite(&upgraderSuite{})

type upgraderSuite struct {
	testing.JujuConnSuite
	oldVarDir string
}

func (s *upgraderSuite) SetUpTest(c *C) {
	s.JujuConnSuite.SetUpTest(c)
	s.oldVarDir = environs.VarDir
	environs.VarDir = c.MkDir()
}

func (s *upgraderSuite) TearDownTest(c *C) {
	environs.VarDir = s.oldVarDir
	invalidVersion = func() {}
	sameVersion = func() {}
	s.JujuConnSuite.TearDownTest(c)
}

func (s *upgraderSuite) TestUpgraderError(c *C) {
	st, err := state.Open(s.StateInfo(c))
	c.Assert(err, IsNil)
	event := make(chan string, 10)
	_, upgraderDone := startUpgrader(st, event)
	// We have no installed tools, so the logic should set the agent
	// tools anyway, but with no URL.
	assertEvent(c, event, fmt.Sprintf("SetAgentTools %s ", version.Current))

	// Close the state under the watcher and check that the upgrader dies.
	st.Close()
	select {
	case err := <-upgraderDone:
		c.Assert(err, Not(FitsTypeOf), &UpgradedError{})
		c.Assert(err, NotNil)
	case <-time.After(500 * time.Millisecond):
		c.Fatalf("upgrader did not stop as expected")
	}
}

func (s *upgraderSuite) TestUpgraderStop(c *C) {
	event := make(chan string, 10)
	u, upgraderDone := startUpgrader(s.State, event)
	assertEvent(c, event, fmt.Sprintf("SetAgentTools %s ", version.Current))

	err := u.Stop()
	c.Assert(err, IsNil)

	select {
	case err := <-upgraderDone:
		c.Assert(err, IsNil)
	case <-time.After(500 * time.Millisecond):
		c.Fatalf("upgrader did not stop as expected")
	}
}

// startUpgrader starts the upgrader using the given machine
// for observing and changing agent tools.
func startUpgrader(st *state.State, event chan<-string) (u *Upgrader, upgraderDone <-chan error) {
	as := &testAgentState{
		event: event,
	}
	u = NewUpgrader(st, "testagent", as)
	done := make(chan error, 1)
	go func() {
		done <- u.Wait()
	}()
	upgraderDone = done
	return
}

func (s *upgraderSuite) proposeVersion(c *C, vers version.Number, devVersion bool) {
	cfg, err := s.State.EnvironConfig()
	c.Assert(err, IsNil)
	attrs := cfg.AllAttrs()
	attrs["agent-version"] = vers.String()
	attrs["dev-version"] = devVersion
	newCfg, err := config.New(attrs)
	c.Assert(err, IsNil)
	err = s.State.SetEnvironConfig(newCfg)
	c.Assert(err, IsNil)
}

func (s *upgraderSuite) uploadTools(c *C, vers version.Binary) (path string, tools *state.Tools) {
	tgz := coretesting.TarGz(
		coretesting.NewTarFile("juju", 0777, "juju contents "+vers.String()),
		coretesting.NewTarFile("jujuc", 0777, "jujuc contents "+vers.String()),
		coretesting.NewTarFile("jujud", 0777, "jujud contents "+vers.String()),
	)
	storage := s.Conn.Environ.Storage()
	err := storage.Put(environs.ToolsStoragePath(vers), bytes.NewReader(tgz), int64(len(tgz)))
	c.Assert(err, IsNil)
	path = environs.ToolsStoragePath(vers)
	url, err := s.Conn.Environ.Storage().URL(path)
	c.Assert(err, IsNil)
	return path, &state.Tools{URL: url, Binary: vers}
}

type proposal struct {
	version string
	devVersion bool
}	

var upgraderTests = []struct{
	about string
	upload []string		// Upload these tools versions.
	propose string		// Propose this version...
	devVersion bool	// ... with devVersion set to this.

	// Things that may happen.
	invalidVersion bool
	sameVersion bool
	upgradeTo string
} {{
	about: "propose with no possible candidates",
	propose: "2.0.2",
	invalidVersion: true,
}, {
	about: "propose with same candidate as current",
	upload: []string{"2.0.0"},
	propose: "2.0.4",
	sameVersion: true,
}, {
	about: "propose development version when !devVersion",
	upload: []string{"2.0.1"},
	propose: "2.0.4",
	sameVersion: true,
}, {
	about: "propose development version when devVersion",
	propose: "2.0.4",
	devVersion: true,
	upgradeTo: "2.0.1",
}, {
	about: "propose release version when !devVersion",
	propose: "2.0.4",
	upgradeTo: "2.0.0",
}, {
	about: "propose with higher available candidates",
	upload: []string{"2.0.5", "2.0.6"},
	propose: "2.0.4",
	sameVersion: true,
}, {
	about: "propose exact available version",
	propose: "2.0.6",
	upgradeTo: "2.0.6",
}, {
	about: "propose downgrade",
	propose: "2.0.5",
	upgradeTo: "2.0.5",
},
}
		
func (s *upgraderSuite) TestUpgrader(c *C) {
	// Set up the test hooks.
	event := make(chan string, 10)
	sameVersion = func() {
		event <- "sameVersion"
	}
	invalidVersion = func() {
		event <- "invalidVersion"
	}

	// Set up the current version and tools.
	version.Current = version.MustParseBinary("2.0.0-foo-bar")
	v0path, v0tools := s.uploadTools(c, version.Current)

	// Unpack the "current" version of the tools, and delete them from
	// the storage so that we're sure that the uploader isn't trying
	// to fetch them.
	resp, err := http.Get(v0tools.URL)
	c.Assert(err, IsNil)
	err = environs.UnpackTools(v0tools, resp.Body)
	c.Assert(err, IsNil)
	err = s.Conn.Environ.Storage().Remove(v0path)
	c.Assert(err, IsNil)

	var (
		u *Upgrader
		upgraderDone <-chan error
		currentTools = v0tools
	)

	defer func() {
		// If a test failed, make sure we see any error from the upgrader.
		if u != nil {
			select {
			case err := <-upgraderDone:
				c.Check(err, IsNil)
			case <-time.After(500 * time.Millisecond):
			}
		}
	}()

	uploaded := make(map[version.Number] *state.Tools)
	for i, test := range upgraderTests {
		c.Logf("%d. %s; current version: %v", i, test.about, version.Current)
		for _, v := range test.upload {
			vers := version.Current
			vers.Number = version.MustParse(v)
			_, tools := s.uploadTools(c, vers)
			uploaded[vers.Number] = tools
		}
		if u == nil {
			u, upgraderDone = startUpgrader(s.State, event)
			expect := fmt.Sprintf("SetAgentTools %v %s", currentTools.Binary, currentTools.URL)
			assertEvent(c, event, expect)
		}
		s.proposeVersion(c, version.MustParse(test.propose), test.devVersion)
		switch {
		case test.sameVersion:
			assertEvent(c, event, "sameVersion")
		case test.invalidVersion:
			assertEvent(c, event, "invalidVersion")
		case test.upgradeTo != "":
			tools := uploaded[version.MustParse(test.upgradeTo)]
			select {
			case err := <-upgraderDone:
				c.Assert(err, FitsTypeOf, &UpgradedError{})
				c.Assert(err.(*UpgradedError).Tools, DeepEquals, tools)
			case <-time.After(500 * time.Millisecond):
				c.Fatalf("upgrader did not stop as expected")
			}
			// Check that the upgraded version was really downloaded.
			data, err := ioutil.ReadFile(filepath.Join(environs.ToolsDir(tools.Binary), "jujud"))
			c.Assert(err, IsNil)
			c.Assert(string(data), Equals, "jujud contents " + tools.Binary.String())

			u, upgraderDone = nil, nil
			currentTools = tools
			version.Current = tools.Binary
		default:
			panic("no action in test")
		}
	}
}

func assertEvent(c *C, event <-chan string, want string) {
	select {
	case got := <-event:
		c.Assert(got, Equals, want)
	case <-time.After(500 * time.Millisecond):
		c.Fatalf("no event received; expected %q", want)
	}
}

type testAgentState struct {
	tomb.Tomb
	event chan<- string
}

func (t *testAgentState) SetAgentTools(tools *state.Tools) error {
	t.event <- fmt.Sprintf("SetAgentTools %v %s", tools.Binary, tools.URL)
	return nil
}
