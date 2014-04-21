// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter_test

import (
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"

	jc "github.com/juju/testing/checkers"
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api/params"
	ft "launchpad.net/juju-core/testing/filetesting"
)

// These tests are copies of the old git-deployer-related tests, to test that
// the uniter with the manifest-deployer work patched out still works how it
// used to; thus demonstrating that the *other* tests that verify manifest
// deployer behaviour in the presence of an old git deployer are working against
// an accurate representation of the base state.
// The only actual behaviour change is that we no longer commit changes after
// each hook execution; this is reflected by checking that it's dirty in a couple
// of places where we once checked it was not.

var upgradeGitConflictsTests = []uniterTest{
	// Upgrade scenarios - handling conflicts.
	ut(
		"upgrade: conflicting files",
		startGitUpgradeError{},

		// NOTE: this is just dumbly committing the conflicts, but AFAICT this
		// is the only reasonable solution; if the user tells us it's resolved
		// we have to take their word for it.
		resolveError{state.ResolvedNoHooks},
		waitHooks{"upgrade-charm", "config-changed"},
		waitUnit{
			status: params.StatusStarted,
			charm:  1,
		},
		verifyGitCharm{revision: 1},
	), ut(
		`upgrade: conflicting directories`,
		createCharm{
			customize: func(c *gc.C, ctx *context, path string) {
				err := os.Mkdir(filepath.Join(path, "data"), 0755)
				c.Assert(err, gc.IsNil)
				appendHook(c, path, "start", "echo DATA > data/newfile")
			},
		},
		serveCharm{},
		createUniter{},
		waitUnit{
			status: params.StatusStarted,
		},
		waitHooks{"install", "config-changed", "start"},
		verifyGitCharm{dirty: true},

		createCharm{
			revision: 1,
			customize: func(c *gc.C, ctx *context, path string) {
				data := filepath.Join(path, "data")
				err := ioutil.WriteFile(data, []byte("<nelson>ha ha</nelson>"), 0644)
				c.Assert(err, gc.IsNil)
			},
		},
		serveCharm{},
		upgradeCharm{revision: 1},
		waitUnit{
			status: params.StatusError,
			info:   "upgrade failed",
			charm:  1,
		},
		verifyWaiting{},
		verifyGitCharm{dirty: true},

		resolveError{state.ResolvedNoHooks},
		waitHooks{"upgrade-charm", "config-changed"},
		waitUnit{
			status: params.StatusStarted,
			charm:  1,
		},
		verifyGitCharm{revision: 1},
	), ut(
		"upgrade conflict resolved with forced upgrade",
		startGitUpgradeError{},
		createCharm{
			revision: 2,
			customize: func(c *gc.C, ctx *context, path string) {
				otherdata := filepath.Join(path, "otherdata")
				err := ioutil.WriteFile(otherdata, []byte("blah"), 0644)
				c.Assert(err, gc.IsNil)
			},
		},
		serveCharm{},
		upgradeCharm{revision: 2, forced: true},
		waitUnit{
			status: params.StatusStarted,
			charm:  2,
		},
		waitHooks{"upgrade-charm", "config-changed"},
		verifyGitCharm{revision: 2},
		custom{func(c *gc.C, ctx *context) {
			// otherdata should exist (in v2)
			otherdata, err := ioutil.ReadFile(filepath.Join(ctx.path, "charm", "otherdata"))
			c.Assert(err, gc.IsNil)
			c.Assert(string(otherdata), gc.Equals, "blah")

			// ignore should not (only in v1)
			_, err = os.Stat(filepath.Join(ctx.path, "charm", "ignore"))
			c.Assert(err, jc.Satisfies, os.IsNotExist)

			// data should contain what was written in the start hook
			data, err := ioutil.ReadFile(filepath.Join(ctx.path, "charm", "data"))
			c.Assert(err, gc.IsNil)
			c.Assert(string(data), gc.Equals, "STARTDATA\n")
		}},
	), ut(
		"upgrade conflict service dying",
		startGitUpgradeError{},
		serviceDying,
		verifyWaiting{},
		resolveError{state.ResolvedNoHooks},
		waitHooks{"upgrade-charm", "config-changed", "stop"},
		waitUniterDead{},
	), ut(
		"upgrade conflict unit dying",
		startGitUpgradeError{},
		unitDying,
		verifyWaiting{},
		resolveError{state.ResolvedNoHooks},
		waitHooks{"upgrade-charm", "config-changed", "stop"},
		waitUniterDead{},
	), ut(
		"upgrade conflict unit dead",
		startGitUpgradeError{},
		unitDead,
		waitUniterDead{},
		waitHooks{},
	),
}

func (s *UniterSuite) TestUniterUpgradeGitConflicts(c *gc.C) {
	patchedTests := make([]uniterTest, len(upgradeGitConflictsTests))
	for i, test := range upgradeGitConflictsTests {
		patchedTests[i] = ut(test.summary, prepareGitUniter{test.steps})
	}
	s.runUniterTests(c, patchedTests)
}

type verifyGitCharm struct {
	revision int
	dirty    bool
}

func (s verifyGitCharm) step(c *gc.C, ctx *context) {
	charmPath := filepath.Join(ctx.path, "charm")
	if !s.dirty {
		revisionPath := filepath.Join(charmPath, "revision")
		content, err := ioutil.ReadFile(revisionPath)
		c.Assert(err, gc.IsNil)
		c.Assert(string(content), gc.Equals, strconv.Itoa(s.revision))
		err = ctx.unit.Refresh()
		c.Assert(err, gc.IsNil)
		url, ok := ctx.unit.CharmURL()
		c.Assert(ok, gc.Equals, true)
		c.Assert(url, gc.DeepEquals, curl(s.revision))
	}

	// Before we try to check the git status, make sure expected hooks are all
	// complete, to prevent the test and the uniter interfering with each other.
	step(c, ctx, waitHooks{})
	step(c, ctx, waitHooks{})
	cmd := exec.Command("git", "status")
	cmd.Dir = filepath.Join(ctx.path, "charm")
	out, err := cmd.CombinedOutput()
	c.Assert(err, gc.IsNil)
	cmp := gc.Matches
	if s.dirty {
		cmp = gc.Not(gc.Matches)
	}
	c.Assert(string(out), cmp, "(# )?On branch master\nnothing to commit.*\n")
}

type startGitUpgradeError struct{}

func (s startGitUpgradeError) step(c *gc.C, ctx *context) {
	steps := []stepper{
		createCharm{
			customize: func(c *gc.C, ctx *context, path string) {
				appendHook(c, path, "start", "echo STARTDATA > data")
			},
		},
		serveCharm{},
		createUniter{},
		waitUnit{
			status: params.StatusStarted,
		},
		waitHooks{"install", "config-changed", "start"},
		verifyGitCharm{dirty: true},

		createCharm{
			revision: 1,
			customize: func(c *gc.C, ctx *context, path string) {
				ft.File{"data", "<nelson>ha ha</nelson>", 0644}.Create(c, path)
				ft.File{"ignore", "anything", 0644}.Create(c, path)
			},
		},
		serveCharm{},
		upgradeCharm{revision: 1},
		waitUnit{
			status: params.StatusError,
			info:   "upgrade failed",
			charm:  1,
		},
		verifyWaiting{},
		verifyGitCharm{dirty: true},
	}
	for _, s_ := range steps {
		step(c, ctx, s_)
	}
}
