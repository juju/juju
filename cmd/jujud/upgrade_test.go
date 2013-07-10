// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"fmt"
	"io/ioutil"
	"path/filepath"
	"time"

	. "launchpad.net/gocheck"

	"launchpad.net/juju-core/agent"
	"launchpad.net/juju-core/environs/dummy"
	envtesting "launchpad.net/juju-core/environs/testing"
	"launchpad.net/juju-core/environs/tools"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/version"
	"launchpad.net/juju-core/worker"
)

var _ = Suite(&UpgraderSuite{})

type UpgraderSuite struct {
	oldVersion version.Binary
	agentSuite
}

func (s *UpgraderSuite) SetUpTest(c *C) {
	s.JujuConnSuite.SetUpTest(c)
	s.oldVersion = version.Current
}

func (s *UpgraderSuite) TearDownTest(c *C) {
	version.Current = s.oldVersion
	s.JujuConnSuite.TearDownTest(c)
}

func (s *UpgraderSuite) TestUpgraderStop(c *C) {
	u := s.startUpgrader(c, &state.Tools{Binary: version.Current})
	err := u.Stop()
	c.Assert(err, IsNil)
}

var _ worker.Worker = (*Upgrader)(nil)

var (
	v200     = version.MustParse("2.0.0")
	t200p64  = version.MustParseBinary("2.0.0-precise-amd64")
	t200p32  = version.MustParseBinary("2.0.0-precise-i386")
	t200q64  = version.MustParseBinary("2.0.0-quantal-amd64")
	t2007p64 = version.MustParseBinary("2.0.0.7-precise-amd64")
	t200all  = []version.Binary{t200p64, t200p32, t200q64, t2007p64}

	v214     = version.MustParse("2.1.4")
	t214p64  = version.MustParseBinary("2.1.4-precise-amd64")
	v2144    = version.MustParse("2.1.4.4")
	t2144p64 = version.MustParseBinary("2.1.4.4-precise-amd64")
	t2144p32 = version.MustParseBinary("2.1.4.4-precise-i386")
	t2144q64 = version.MustParseBinary("2.1.4.4-quantal-amd64")
	t214all  = []version.Binary{t214p64, t2144p64, t2144p32, t2144q64}
	t2all    = append(t200all, t214all...)

	v300    = version.MustParse("3.0.0")
	t300p64 = version.MustParseBinary("3.0.0-precise-amd64")
	tAll    = append(t2all, t300p64)
)

var upgraderTests = []struct {
	about     string
	current   version.Binary
	available []version.Binary
	propose   version.Number
	upgrade   version.Binary
}{{
	about:   "same version, no tools available",
	current: t200p64,
	propose: v200,
}, {
	about:     "same version, bad tools available",
	current:   t200p64,
	available: []version.Binary{t200p32, t200q64, t214p64, t300p64},
	propose:   v200,
}, {
	about:     "same version, all tools available",
	current:   t200p64,
	available: tAll,
	propose:   v200,
}, {
	about:   "newer version, no tools available",
	current: t200p64,
	propose: v2144,
}, {
	about:     "newer version, bad tools available",
	current:   t200p64,
	available: []version.Binary{t200p64, t214p64, t2144p32, t2144q64, t300p64},
	propose:   v2144,
}, {
	about:     "newer version, all tools available",
	current:   t200p64,
	available: tAll,
	propose:   v2144,
	upgrade:   t2144p64,
}, {
	about:   "older version, no tools available",
	current: t2144p64,
	propose: v200,
}, {
	about:     "older version, bad tools available",
	current:   t2144p64,
	available: []version.Binary{t200p32, t200q64, t214p64, t2144p64, t300p64},
	propose:   v200,
}, {
	about:     "older version, all tools available",
	current:   t2144p64,
	available: tAll,
	propose:   v200,
	upgrade:   t200p64,
}, {
	about:     "newer major version, all tools available",
	current:   t200p64,
	available: tAll,
	propose:   v300,
}, {
	about:     "older major version, all tools available",
	current:   t300p64,
	available: tAll,
	propose:   v200,
}}

func (s *UpgraderSuite) TestUpgrader(c *C) {
	for i, test := range upgraderTests {
		c.Logf("\ntest %d: %s", i, test.about)
		// Note: primeTools sets version.Current...
		currentTools := s.primeTools(c, test.current)
		// ...but it also puts tools in storage we don't need, which is why we
		// don't clean up garbage from earlier runs first.
		envtesting.RemoveAllTools(c, s.Conn.Environ)
		uploaded := make(map[version.Binary]*state.Tools)
		for _, vers := range test.available {
			tools := s.uploadTools(c, vers)
			uploaded[vers] = tools
		}

		func() {
			u := s.startUpgrader(c, currentTools)
			defer u.Stop()
			s.proposeVersion(c, test.propose)
			s.State.StartSync()
			if test.upgrade.Number == version.Zero {
				assertNothingHappens(c, u)
				c.Assert(u.Stop(), IsNil)
				return
			}

			ug := waitDeath(c, u)
			tools := uploaded[test.upgrade]
			c.Check(ug.NewTools, DeepEquals, tools)
			c.Check(ug.OldTools.Binary, Equals, version.Current)
			c.Check(ug.DataDir, Equals, s.DataDir())
			c.Check(ug.AgentName, Equals, "testagent")

			// Check that the upgraded version was really downloaded.
			path := agent.SharedToolsDir(s.DataDir(), tools.Binary)
			data, err := ioutil.ReadFile(filepath.Join(path, "jujud"))
			c.Check(err, IsNil)
			c.Check(string(data), Equals, "jujud contents "+tools.Binary.String())
		}()
	}
}

func (s *UpgraderSuite) TestStillWorksAfterBadVersions(c *C) {
	currentTools := s.primeTools(c, t200p64)
	envtesting.RemoveAllTools(c, s.Conn.Environ)
	u := s.startUpgrader(c, currentTools)
	defer u.Stop()

	// Propose a missing version
	s.proposeVersion(c, v2144)
	s.State.StartSync()
	assertNothingHappens(c, u)

	// Propose an incompatible version
	s.proposeVersion(c, v300)
	s.State.StartSync()
	assertNothingHappens(c, u)

	// Propose a working version
	newTools := s.uploadTools(c, t2144p64)
	s.proposeVersion(c, v2144)
	s.State.StartSync()
	ug := waitDeath(c, u)
	c.Assert(ug.NewTools, DeepEquals, newTools)
}

var delayedStopTests = []struct {
	about             string
	upgraderKillDelay time.Duration
	storageDelay      time.Duration
	propose           string
	err               string
}{{
	about:             "same version",
	upgraderKillDelay: time.Second,
	propose:           "2.0.3",
}, {
	about:             "same version found for higher proposed version",
	upgraderKillDelay: time.Second,
	propose:           "2.0.4",
}, {
	about:             "no appropriate version found",
	upgraderKillDelay: time.Second,
	propose:           "2.0.3",
}, {
	about:             "time out",
	propose:           "2.0.6",
	storageDelay:      time.Second,
	upgraderKillDelay: 10 * time.Millisecond,
	err:               "upgrader aborted download.*",
}, {
	about:             "successful upgrade",
	upgraderKillDelay: time.Second,
	propose:           "2.0.6",
	// enough delay that the stop will probably arrive before the
	// tools have downloaded, thus checking that the
	// upgrader really did wait for the download.
	storageDelay: 5 * time.Millisecond,
	err:          `must restart: an agent upgrade is available`,
}, {
	about:             "fetch error",
	upgraderKillDelay: time.Second,
	propose:           "2.0.7",
},
}

func (s *UpgraderSuite) TestDelayedStop(c *C) {
	defer dummy.SetStorageDelay(0)
	tools := s.primeTools(c, version.MustParseBinary("2.0.3-foo-bar"))
	s.uploadTools(c, version.MustParseBinary("2.0.5-foo-bar"))
	s.uploadTools(c, version.MustParseBinary("2.0.6-foo-bar"))
	s.uploadTools(c, version.MustParseBinary("2.0.6-foo-bar"))
	s.uploadTools(c, version.MustParseBinary("2.0.7-foo-bar"))
	s.poisonVersion(version.MustParseBinary("2.0.7-foo-bar"))

	for i, test := range delayedStopTests {
		c.Logf("%d. %v", i, test.about)
		upgraderKillDelay = test.upgraderKillDelay
		dummy.SetStorageDelay(test.storageDelay)
		proposed := version.MustParse(test.propose)
		s.proposeVersion(c, proposed)
		u := s.startUpgrader(c, tools)
		t0 := time.Now()
		err := u.Stop()
		d := time.Now().Sub(t0)
		if d > 100*time.Millisecond {
			c.Errorf("upgrader took took too long: %v", d)
		}
		if test.err == "" {
			c.Check(err, IsNil)
		} else {
			c.Check(err, ErrorMatches, test.err)
		}
	}
}

func (s *UpgraderSuite) poisonVersion(vers version.Binary) {
	name := tools.StorageName(vers)
	dummy.Poison(s.Conn.Environ.Storage(), name, fmt.Errorf("poisoned file"))
}

func (s *UpgraderSuite) removeVersion(c *C, vers version.Binary) {
	name := tools.StorageName(vers)
	err := s.Conn.Environ.Storage().Remove(name)
	c.Assert(err, IsNil)
}

func (s *UpgraderSuite) TestUpgraderReadyErrorUpgrade(c *C) {
	currentTools := s.primeTools(c, version.MustParseBinary("2.0.2-foo-bar"))
	ug := &UpgradeReadyError{
		AgentName: "foo",
		OldTools:  &state.Tools{Binary: version.MustParseBinary("2.0.0-foo-bar")},
		NewTools:  currentTools,
		DataDir:   s.DataDir(),
	}
	err := ug.ChangeAgentTools()
	c.Assert(err, IsNil)
	d := agent.ToolsDir(s.DataDir(), "foo")
	data, err := ioutil.ReadFile(filepath.Join(d, "jujud"))
	c.Assert(err, IsNil)
	c.Assert(string(data), Equals, "jujud contents 2.0.2-foo-bar")
}
func assertNothingHappens(c *C, u *Upgrader) {
	done := make(chan error, 1)
	go func() {
		done <- u.Wait()
	}()
	select {
	case got := <-done:
		c.Fatalf("expected nothing to happen, got %#v", got)
	case <-time.After(100 * time.Millisecond):
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

// startUpgrader starts the upgrader using the given machine,
// expecting to see it set the given agent tools.
func (s *UpgraderSuite) startUpgrader(c *C, expectTools *state.Tools) *Upgrader {
	as := testAgentState(make(chan *state.Tools))
	u := NewUpgrader(s.State, as, s.DataDir())
	select {
	case tools := <-as:
		c.Assert(tools, DeepEquals, expectTools)
	case <-time.After(500 * time.Millisecond):
		c.Fatalf("upgrader did not set agent tools")
	}
	return u
}

func waitDeath(c *C, u *Upgrader) *UpgradeReadyError {
	done := make(chan error, 1)
	go func() {
		done <- u.Wait()
	}()
	select {
	case err := <-done:
		c.Assert(err, FitsTypeOf, &UpgradeReadyError{})
		return err.(*UpgradeReadyError)
	case <-time.After(500 * time.Millisecond):
		c.Fatalf("upgrader did not die as expected")
	}
	panic("unreachable")
}

type testAgentState chan *state.Tools

func (as testAgentState) SetAgentTools(tools *state.Tools) error {
	t := *tools
	as <- &t
	return nil
}

func (as testAgentState) Tag() string {
	return "testagent"
}

func (as testAgentState) Life() state.Life {
	panic("unimplemented")
}

func (as testAgentState) SetMongoPassword(string) error {
	panic("unimplemented")
}
