// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/ioutil"
	"net/rpc"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	stdtesting "testing"
	"time"

	gc "launchpad.net/gocheck"
	"launchpad.net/goyaml"

	"launchpad.net/juju-core/agent/tools"
	"launchpad.net/juju-core/charm"
	"launchpad.net/juju-core/errors"
	"launchpad.net/juju-core/juju/osenv"
	"launchpad.net/juju-core/juju/testing"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api"
	"launchpad.net/juju-core/state/api/params"
	apiuniter "launchpad.net/juju-core/state/api/uniter"
	coretesting "launchpad.net/juju-core/testing"
	jc "launchpad.net/juju-core/testing/checkers"
	"launchpad.net/juju-core/utils"
	utilexec "launchpad.net/juju-core/utils/exec"
	"launchpad.net/juju-core/utils/fslock"
	"launchpad.net/juju-core/worker"
	"launchpad.net/juju-core/worker/uniter"
)

// worstCase is used for timeouts when timing out
// will fail the test. Raising this value should
// not affect the overall running time of the tests
// unless they fail.
const worstCase = coretesting.LongWait

func TestPackage(t *stdtesting.T) {
	coretesting.MgoTestPackage(t)
}

type UniterSuite struct {
	coretesting.GitSuite
	testing.JujuConnSuite
	coretesting.HTTPSuite
	dataDir  string
	oldLcAll string
	unitDir  string

	st     *api.State
	uniter *apiuniter.State
}

var _ = gc.Suite(&UniterSuite{})

func (s *UniterSuite) SetUpSuite(c *gc.C) {
	s.JujuConnSuite.SetUpSuite(c)
	s.HTTPSuite.SetUpSuite(c)
	s.dataDir = c.MkDir()
	toolsDir := tools.ToolsDir(s.dataDir, "unit-u-0")
	err := os.MkdirAll(toolsDir, 0755)
	c.Assert(err, gc.IsNil)
	cmd := exec.Command("go", "build", "launchpad.net/juju-core/cmd/jujud")
	cmd.Dir = toolsDir
	out, err := cmd.CombinedOutput()
	c.Logf(string(out))
	c.Assert(err, gc.IsNil)
	s.oldLcAll = os.Getenv("LC_ALL")
	os.Setenv("LC_ALL", "en_US")
	s.unitDir = filepath.Join(s.dataDir, "agents", "unit-u-0")
}

func (s *UniterSuite) TearDownSuite(c *gc.C) {
	os.Setenv("LC_ALL", s.oldLcAll)
	s.HTTPSuite.TearDownSuite(c)
	s.JujuConnSuite.TearDownSuite(c)
}

func (s *UniterSuite) SetUpTest(c *gc.C) {
	s.GitSuite.SetUpTest(c)
	s.JujuConnSuite.SetUpTest(c)
	s.HTTPSuite.SetUpTest(c)
}

func (s *UniterSuite) TearDownTest(c *gc.C) {
	s.ResetContext(c)
	s.HTTPSuite.TearDownTest(c)
	s.JujuConnSuite.TearDownTest(c)
	s.GitSuite.TearDownTest(c)
}

func (s *UniterSuite) Reset(c *gc.C) {
	s.JujuConnSuite.Reset(c)
	s.ResetContext(c)
}

func (s *UniterSuite) ResetContext(c *gc.C) {
	coretesting.Server.Flush()
	err := os.RemoveAll(s.unitDir)
	c.Assert(err, gc.IsNil)
}

func (s *UniterSuite) APILogin(c *gc.C, unit *state.Unit) {
	password, err := utils.RandomPassword()
	c.Assert(err, gc.IsNil)
	err = unit.SetPassword(password)
	c.Assert(err, gc.IsNil)
	s.st = s.OpenAPIAs(c, unit.Tag(), password)
	c.Assert(s.st, gc.NotNil)
	c.Logf("API: login as %q successful", unit.Tag())
	s.uniter = s.st.Uniter()
	c.Assert(s.uniter, gc.NotNil)
}

var _ worker.Worker = (*uniter.Uniter)(nil)

type uniterTest struct {
	summary string
	steps   []stepper
}

func ut(summary string, steps ...stepper) uniterTest {
	return uniterTest{summary, steps}
}

type stepper interface {
	step(c *gc.C, ctx *context)
}

type context struct {
	uuid          string
	path          string
	dataDir       string
	s             *UniterSuite
	st            *state.State
	charms        coretesting.ResponseMap
	hooks         []string
	sch           *state.Charm
	svc           *state.Service
	unit          *state.Unit
	uniter        *uniter.Uniter
	relatedSvc    *state.Service
	relation      *state.Relation
	relationUnits map[string]*state.RelationUnit
	subordinate   *state.Unit

	hooksCompleted []string
}

var _ uniter.UniterExecutionObserver = (*context)(nil)

func (ctx *context) HookCompleted(hookName string) {
	ctx.hooksCompleted = append(ctx.hooksCompleted, hookName)
}

func (ctx *context) HookFailed(hookName string) {
	ctx.hooksCompleted = append(ctx.hooksCompleted, "fail-"+hookName)
}

func (ctx *context) run(c *gc.C, steps []stepper) {
	defer func() {
		if ctx.uniter != nil {
			err := ctx.uniter.Stop()
			c.Assert(err, gc.IsNil)
		}
	}()
	for i, s := range steps {
		c.Logf("step %d:\n", i)
		step(c, ctx, s)
	}
}

var goodHook = `
#!/bin/bash --norc
juju-log $JUJU_ENV_UUID %s $JUJU_REMOTE_UNIT
`[1:]

var badHook = `
#!/bin/bash --norc
juju-log $JUJU_ENV_UUID fail-%s $JUJU_REMOTE_UNIT
exit 1
`[1:]

func (ctx *context) writeHook(c *gc.C, path string, good bool) {
	hook := badHook
	if good {
		hook = goodHook
	}
	content := fmt.Sprintf(hook, filepath.Base(path))
	err := ioutil.WriteFile(path, []byte(content), 0755)
	c.Assert(err, gc.IsNil)
}

func (ctx *context) matchHooks(c *gc.C) (match bool, overshoot bool) {
	c.Logf("ctx.hooksCompleted: %#v", ctx.hooksCompleted)
	if len(ctx.hooksCompleted) < len(ctx.hooks) {
		return false, false
	}
	for i, e := range ctx.hooks {
		if ctx.hooksCompleted[i] != e {
			return false, false
		}
	}
	return true, len(ctx.hooksCompleted) > len(ctx.hooks)
}

var startupTests = []uniterTest{
	// Check conditions that can cause the uniter to fail to start.
	ut(
		"unable to create state dir",
		writeFile{"state", 0644},
		createCharm{},
		createServiceAndUnit{},
		startUniter{},
		waitUniterDead{`failed to initialize uniter for "unit-u-0": .*not a directory`},
	), ut(
		"unknown unit",
		// We still need to create a unit, because that's when we also
		// connect to the API, but here we use a different service
		// (and hence unit) name.
		createCharm{},
		createServiceAndUnit{serviceName: "w"},
		startUniter{"unit-u-0"},
		waitUniterDead{`failed to initialize uniter for "unit-u-0": permission denied`},
	),
}

func (s *UniterSuite) TestUniterStartup(c *gc.C) {
	s.runUniterTests(c, startupTests)
}

var bootstrapTests = []uniterTest{
	// Check error conditions during unit bootstrap phase.
	ut(
		"insane deployment",
		createCharm{},
		serveCharm{},
		writeFile{"charm", 0644},
		createUniter{},
		waitUniterDead{`ModeInstalling cs:quantal/wordpress-0: charm deployment failed: ".*charm" is not a directory`},
	), ut(
		"charm cannot be downloaded",
		createCharm{},
		custom{func(c *gc.C, ctx *context) {
			coretesting.Server.Response(404, nil, nil)
		}},
		createUniter{},
		waitUniterDead{`ModeInstalling cs:quantal/wordpress-0: failed to download charm .* 404 Not Found`},
	),
}

func (s *UniterSuite) TestUniterBootstrap(c *gc.C) {
	s.runUniterTests(c, bootstrapTests)
}

var installHookTests = []uniterTest{
	ut(
		"install hook fail and resolve",
		startupError{"install"},
		verifyWaiting{},

		resolveError{state.ResolvedNoHooks},
		waitUnit{
			status: params.StatusStarted,
		},
		waitHooks{"config-changed", "start"},
	), ut(
		"install hook fail and retry",
		startupError{"install"},
		verifyWaiting{},

		resolveError{state.ResolvedRetryHooks},
		waitUnit{
			status: params.StatusError,
			info:   `hook failed: "install"`,
			data: params.StatusData{
				"hook": "install",
			},
		},
		waitHooks{"fail-install"},
		fixHook{"install"},
		verifyWaiting{},

		resolveError{state.ResolvedRetryHooks},
		waitUnit{
			status: params.StatusStarted,
		},
		waitHooks{"install", "config-changed", "start"},
	),
}

func (s *UniterSuite) TestUniterInstallHook(c *gc.C) {
	s.runUniterTests(c, installHookTests)
}

var startHookTests = []uniterTest{
	ut(
		"start hook fail and resolve",
		startupError{"start"},
		verifyWaiting{},

		resolveError{state.ResolvedNoHooks},
		waitUnit{
			status: params.StatusStarted,
		},
		waitHooks{"config-changed"},
		verifyRunning{},
	), ut(
		"start hook fail and retry",
		startupError{"start"},
		verifyWaiting{},

		resolveError{state.ResolvedRetryHooks},
		waitUnit{
			status: params.StatusError,
			info:   `hook failed: "start"`,
			data: params.StatusData{
				"hook": "start",
			},
		},
		waitHooks{"fail-start"},
		verifyWaiting{},

		fixHook{"start"},
		resolveError{state.ResolvedRetryHooks},
		waitUnit{
			status: params.StatusStarted,
		},
		waitHooks{"start", "config-changed"},
		verifyRunning{},
	),
}

func (s *UniterSuite) TestUniterStartHook(c *gc.C) {
	s.runUniterTests(c, startHookTests)
}

var multipleErrorsTests = []uniterTest{
	ut(
		"resolved is cleared before moving on to next hook",
		createCharm{badHooks: []string{"install", "config-changed", "start"}},
		serveCharm{},
		createUniter{},
		waitUnit{
			status: params.StatusError,
			info:   `hook failed: "install"`,
			data: params.StatusData{
				"hook": "install",
			},
		},
		resolveError{state.ResolvedNoHooks},
		waitUnit{
			status: params.StatusError,
			info:   `hook failed: "config-changed"`,
			data: params.StatusData{
				"hook": "config-changed",
			},
		},
		resolveError{state.ResolvedNoHooks},
		waitUnit{
			status: params.StatusError,
			info:   `hook failed: "start"`,
			data: params.StatusData{
				"hook": "start",
			},
		},
	),
}

func (s *UniterSuite) TestUniterMultipleErrors(c *gc.C) {
	s.runUniterTests(c, multipleErrorsTests)
}

var configChangedHookTests = []uniterTest{
	ut(
		"config-changed hook fail and resolve",
		startupError{"config-changed"},
		verifyWaiting{},

		// Note: we'll run another config-changed as soon as we hit the
		// started state, so the broken hook would actually prevent us
		// from advancing at all if we didn't fix it.
		fixHook{"config-changed"},
		resolveError{state.ResolvedNoHooks},
		waitUnit{
			status: params.StatusStarted,
		},
		waitHooks{"start", "config-changed"},
		// If we'd accidentally retried that hook, somehow, we would get
		// an extra config-changed as we entered started; see that we don't.
		waitHooks{},
		verifyRunning{},
	), ut(
		"config-changed hook fail and retry",
		startupError{"config-changed"},
		verifyWaiting{},

		resolveError{state.ResolvedRetryHooks},
		waitUnit{
			status: params.StatusError,
			info:   `hook failed: "config-changed"`,
			data: params.StatusData{
				"hook": "config-changed",
			},
		},
		waitHooks{"fail-config-changed"},
		verifyWaiting{},

		fixHook{"config-changed"},
		resolveError{state.ResolvedRetryHooks},
		waitUnit{
			status: params.StatusStarted,
		},
		waitHooks{"config-changed", "start"},
		verifyRunning{},
	),
	ut(
		"steady state config change with config-get verification",
		createCharm{
			customize: func(c *gc.C, ctx *context, path string) {
				appendHook(c, path, "config-changed", "config-get --format yaml --output config.out")
			},
		},
		serveCharm{},
		createUniter{},
		waitUnit{
			status: params.StatusStarted,
		},
		waitHooks{"install", "config-changed", "start"},
		assertYaml{"charm/config.out", map[string]interface{}{
			"blog-title": "My Title",
		}},
		changeConfig{"blog-title": "Goodness Gracious Me"},
		waitHooks{"config-changed"},
		verifyRunning{},
		assertYaml{"charm/config.out", map[string]interface{}{
			"blog-title": "Goodness Gracious Me",
		}},
	)}

func (s *UniterSuite) TestUniterConfigChangedHook(c *gc.C) {
	s.runUniterTests(c, configChangedHookTests)
}

var hookSynchronizationTests = []uniterTest{
	ut(
		"verify config change hook not run while lock held",
		quickStart{},
		acquireHookSyncLock{},
		changeConfig{"blog-title": "Goodness Gracious Me"},
		waitHooks{},
		releaseHookSyncLock,
		waitHooks{"config-changed"},
	),
	ut(
		"verify held lock by this unit is broken",
		acquireHookSyncLock{"u/0:fake"},
		quickStart{},
		verifyHookSyncLockUnlocked,
	),
	ut(
		"verify held lock by another unit is not broken",
		acquireHookSyncLock{"u/1:fake"},
		// Can't use quickstart as it has a built in waitHooks.
		createCharm{},
		serveCharm{},
		ensureStateWorker{},
		createServiceAndUnit{},
		startUniter{},
		waitAddresses{},
		waitHooks{},
		verifyHookSyncLockLocked,
		releaseHookSyncLock,
		waitUnit{status: params.StatusStarted},
		waitHooks{"install", "config-changed", "start"},
	),
}

func (s *UniterSuite) TestUniterHookSynchronisation(c *gc.C) {
	s.runUniterTests(c, hookSynchronizationTests)
}

var dyingReactionTests = []uniterTest{
	// Reaction to entity deaths.
	ut(
		"steady state service dying",
		quickStart{},
		serviceDying,
		waitHooks{"stop"},
		waitUniterDead{},
	), ut(
		"steady state unit dying",
		quickStart{},
		unitDying,
		waitHooks{"stop"},
		waitUniterDead{},
	), ut(
		"steady state unit dead",
		quickStart{},
		unitDead,
		waitUniterDead{},
		waitHooks{},
	), ut(
		"hook error service dying",
		startupError{"start"},
		serviceDying,
		verifyWaiting{},
		fixHook{"start"},
		resolveError{state.ResolvedRetryHooks},
		waitHooks{"start", "config-changed", "stop"},
		waitUniterDead{},
	), ut(
		"hook error unit dying",
		startupError{"start"},
		unitDying,
		verifyWaiting{},
		fixHook{"start"},
		resolveError{state.ResolvedRetryHooks},
		waitHooks{"start", "config-changed", "stop"},
		waitUniterDead{},
	), ut(
		"hook error unit dead",
		startupError{"start"},
		unitDead,
		waitUniterDead{},
		waitHooks{},
	),
}

func (s *UniterSuite) TestUniterDyingReaction(c *gc.C) {
	s.runUniterTests(c, dyingReactionTests)
}

var steadyUpgradeTests = []uniterTest{
	// Upgrade scenarios from steady state.
	ut(
		"steady state upgrade",
		quickStart{},
		createCharm{revision: 1},
		upgradeCharm{revision: 1},
		waitUnit{
			status: params.StatusStarted,
			charm:  1,
		},
		waitHooks{"upgrade-charm", "config-changed"},
		verifyCharm{revision: 1},
		verifyRunning{},
	), ut(
		"steady state forced upgrade (identical behaviour)",
		quickStart{},
		createCharm{revision: 1},
		upgradeCharm{revision: 1, forced: true},
		waitUnit{
			status: params.StatusStarted,
			charm:  1,
		},
		waitHooks{"upgrade-charm", "config-changed"},
		verifyCharm{revision: 1},
		verifyRunning{},
	), ut(
		"steady state upgrade hook fail and resolve",
		quickStart{},
		createCharm{revision: 1, badHooks: []string{"upgrade-charm"}},
		upgradeCharm{revision: 1},
		waitUnit{
			status: params.StatusError,
			info:   `hook failed: "upgrade-charm"`,
			data: params.StatusData{
				"hook": "upgrade-charm",
			},
			charm: 1,
		},
		waitHooks{"fail-upgrade-charm"},
		verifyCharm{revision: 1},
		verifyWaiting{},

		resolveError{state.ResolvedNoHooks},
		waitUnit{
			status: params.StatusStarted,
			charm:  1,
		},
		waitHooks{"config-changed"},
		verifyRunning{},
	), ut(
		"steady state upgrade hook fail and retry",
		quickStart{},
		createCharm{revision: 1, badHooks: []string{"upgrade-charm"}},
		upgradeCharm{revision: 1},
		waitUnit{
			status: params.StatusError,
			info:   `hook failed: "upgrade-charm"`,
			data: params.StatusData{
				"hook": "upgrade-charm",
			},
			charm: 1,
		},
		waitHooks{"fail-upgrade-charm"},
		verifyCharm{revision: 1},
		verifyWaiting{},

		resolveError{state.ResolvedRetryHooks},
		waitUnit{
			status: params.StatusError,
			info:   `hook failed: "upgrade-charm"`,
			data: params.StatusData{
				"hook": "upgrade-charm",
			},
			charm: 1,
		},
		waitHooks{"fail-upgrade-charm"},
		verifyWaiting{},

		fixHook{"upgrade-charm"},
		resolveError{state.ResolvedRetryHooks},
		waitUnit{
			status: params.StatusStarted,
			charm:  1,
		},
		waitHooks{"upgrade-charm", "config-changed"},
		verifyRunning{},
	),
	ut(
		// This test does an add-relation as quickly as possible
		// after an upgrade-charm, in the hope that the scheduler will
		// deliver the events in the wrong order. The observed
		// behaviour should be the same in either case.
		"ignore unknown relations until upgrade is done",
		quickStart{},
		createCharm{
			revision: 2,
			customize: func(c *gc.C, ctx *context, path string) {
				renameRelation(c, path, "db", "db2")
				hpath := filepath.Join(path, "hooks", "db2-relation-joined")
				ctx.writeHook(c, hpath, true)
			},
		},
		serveCharm{},
		upgradeCharm{revision: 2},
		addRelation{},
		addRelationUnit{},
		waitHooks{"upgrade-charm", "config-changed", "db2-relation-joined mysql/0 db2:0"},
		verifyCharm{revision: 2},
	),
}

func (s *UniterSuite) TestUniterSteadyStateUpgrade(c *gc.C) {
	s.runUniterTests(c, steadyUpgradeTests)
}

var errorUpgradeTests = []uniterTest{
	// Upgrade scenarios from error state.
	ut(
		"error state unforced upgrade (ignored until started state)",
		startupError{"start"},
		createCharm{revision: 1},
		upgradeCharm{revision: 1},
		waitUnit{
			status: params.StatusError,
			info:   `hook failed: "start"`,
			data: params.StatusData{
				"hook": "start",
			},
		},
		waitHooks{},
		verifyCharm{},
		verifyWaiting{},

		resolveError{state.ResolvedNoHooks},
		waitUnit{
			status: params.StatusStarted,
			charm:  1,
		},
		waitHooks{"config-changed", "upgrade-charm", "config-changed"},
		verifyCharm{revision: 1},
		verifyRunning{},
	), ut(
		"error state forced upgrade",
		startupError{"start"},
		createCharm{revision: 1},
		upgradeCharm{revision: 1, forced: true},
		// It's not possible to tell directly from state when the upgrade is
		// complete, because the new unit charm URL is set at the upgrade
		// process's point of no return (before actually deploying, but after
		// the charm has been downloaded and verified). However, it's still
		// useful to wait until that point...
		waitUnit{
			status: params.StatusError,
			info:   `hook failed: "start"`,
			data: params.StatusData{
				"hook": "start",
			},
			charm: 1,
		},
		// ...because the uniter *will* complete a started deployment even if
		// we stop it from outside. So, by stopping and starting, we can be
		// sure that the operation has completed and can safely verify that
		// the charm state on disk is as we expect.
		verifyWaiting{},
		verifyCharm{revision: 1},

		resolveError{state.ResolvedNoHooks},
		waitUnit{
			status: params.StatusStarted,
			charm:  1,
		},
		waitHooks{"config-changed"},
		verifyRunning{},
	),
}

func (s *UniterSuite) TestUniterErrorStateUpgrade(c *gc.C) {
	s.runUniterTests(c, errorUpgradeTests)
}

var upgradeConflictsTests = []uniterTest{
	// Upgrade scenarios - handling conflicts.
	ut(
		"upgrade: conflicting files",
		startUpgradeError{},

		// NOTE: this is just dumbly committing the conflicts, but AFAICT this
		// is the only reasonable solution; if the user tells us it's resolved
		// we have to take their word for it.
		resolveError{state.ResolvedNoHooks},
		waitHooks{"upgrade-charm", "config-changed"},
		waitUnit{
			status: params.StatusStarted,
			charm:  1,
		},
		verifyCharm{revision: 1},
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
		verifyCharm{},

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
		verifyCharm{dirty: true},

		resolveError{state.ResolvedNoHooks},
		waitHooks{"upgrade-charm", "config-changed"},
		waitUnit{
			status: params.StatusStarted,
			charm:  1,
		},
		verifyCharm{revision: 1},
	), ut(
		"upgrade conflict resolved with forced upgrade",
		startUpgradeError{},
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
		verifyCharm{revision: 2},
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
		startUpgradeError{},
		serviceDying,
		verifyWaiting{},
		resolveError{state.ResolvedNoHooks},
		waitHooks{"upgrade-charm", "config-changed", "stop"},
		waitUniterDead{},
	), ut(
		"upgrade conflict unit dying",
		startUpgradeError{},
		unitDying,
		verifyWaiting{},
		resolveError{state.ResolvedNoHooks},
		waitHooks{"upgrade-charm", "config-changed", "stop"},
		waitUniterDead{},
	), ut(
		"upgrade conflict unit dead",
		startUpgradeError{},
		unitDead,
		waitUniterDead{},
		waitHooks{},
	),
}

func (s *UniterSuite) TestUniterUpgradeConflicts(c *gc.C) {
	s.runUniterTests(c, upgradeConflictsTests)
}

func (s *UniterSuite) TestRunCommand(c *gc.C) {
	testDir := c.MkDir()
	testFile := func(name string) string {
		return filepath.Join(testDir, name)
	}
	echoUnitNameToFile := func(name string) string {
		return fmt.Sprintf("echo juju run ${JUJU_UNIT_NAME} > %s", filepath.Join(testDir, name))
	}
	tests := []uniterTest{
		ut(
			"run commands: environment",
			quickStart{},
			runCommands{echoUnitNameToFile("run.output")},
			verifyFile{filepath.Join(testDir, "run.output"), "juju run u/0\n"},
		), ut(
			"run commands: jujuc commands",
			quickStartRelation{},
			runCommands{
				fmt.Sprintf("owner-get tag > %s", testFile("jujuc.output")),
				fmt.Sprintf("unit-get private-address >> %s", testFile("jujuc.output")),
				fmt.Sprintf("unit-get public-address >> %s", testFile("jujuc.output")),
			},
			verifyFile{
				testFile("jujuc.output"),
				"user-admin\nprivate.dummy.address.example.com\npublic.dummy.address.example.com\n",
			},
		), ut(
			"run commands: proxy settings set",
			quickStartRelation{},
			setProxySettings{Http: "http", Https: "https", Ftp: "ftp"},
			runCommands{
				fmt.Sprintf("echo $http_proxy > %s", testFile("proxy.output")),
				fmt.Sprintf("echo $HTTP_PROXY >> %s", testFile("proxy.output")),
				fmt.Sprintf("echo $https_proxy >> %s", testFile("proxy.output")),
				fmt.Sprintf("echo $HTTPS_PROXY >> %s", testFile("proxy.output")),
				fmt.Sprintf("echo $ftp_proxy >> %s", testFile("proxy.output")),
				fmt.Sprintf("echo $FTP_PROXY >> %s", testFile("proxy.output")),
			},
			verifyFile{
				testFile("proxy.output"),
				"http\nhttp\nhttps\nhttps\nftp\nftp\n",
			},
		), ut(
			"run commands: async using rpc client",
			quickStart{},
			asyncRunCommands{echoUnitNameToFile("run.output")},
			verifyFile{testFile("run.output"), "juju run u/0\n"},
		), ut(
			"run commands: waits for lock",
			quickStart{},
			acquireHookSyncLock{},
			asyncRunCommands{echoUnitNameToFile("wait.output")},
			verifyNoFile{testFile("wait.output")},
			releaseHookSyncLock,
			verifyFile{testFile("wait.output"), "juju run u/0\n"},
		),
	}
	s.runUniterTests(c, tests)
}

var relationsTests = []uniterTest{
	// Relations.
	ut(
		"simple joined/changed/departed",
		quickStartRelation{},
		addRelationUnit{},
		waitHooks{"db-relation-joined mysql/1 db:0", "db-relation-changed mysql/1 db:0"},
		changeRelationUnit{"mysql/0"},
		waitHooks{"db-relation-changed mysql/0 db:0"},
		removeRelationUnit{"mysql/1"},
		waitHooks{"db-relation-departed mysql/1 db:0"},
		verifyRunning{},
	), ut(
		"relation becomes dying; unit is not last remaining member",
		quickStartRelation{},
		relationDying,
		waitHooks{"db-relation-departed mysql/0 db:0", "db-relation-broken db:0"},
		verifyRunning{},
		relationState{life: state.Dying},
		removeRelationUnit{"mysql/0"},
		verifyRunning{},
		relationState{removed: true},
		verifyRunning{},
	), ut(
		"relation becomes dying; unit is last remaining member",
		quickStartRelation{},
		removeRelationUnit{"mysql/0"},
		waitHooks{"db-relation-departed mysql/0 db:0"},
		relationDying,
		waitHooks{"db-relation-broken db:0"},
		verifyRunning{},
		relationState{removed: true},
		verifyRunning{},
	), ut(
		"service becomes dying while in a relation",
		quickStartRelation{},
		serviceDying,
		waitHooks{"db-relation-departed mysql/0 db:0", "db-relation-broken db:0", "stop"},
		waitUniterDead{},
		relationState{life: state.Dying},
		removeRelationUnit{"mysql/0"},
		relationState{removed: true},
	), ut(
		"unit becomes dying while in a relation",
		quickStartRelation{},
		unitDying,
		waitHooks{"db-relation-departed mysql/0 db:0", "db-relation-broken db:0", "stop"},
		waitUniterDead{},
		relationState{life: state.Alive},
		removeRelationUnit{"mysql/0"},
		relationState{life: state.Alive},
	), ut(
		"unit becomes dead while in a relation",
		quickStartRelation{},
		unitDead,
		waitUniterDead{},
		waitHooks{},
		// TODO BUG(?): the unit doesn't leave the scope, leaving the relation
		// unkillable without direct intervention. I'm pretty sure it's not a
		// uniter bug -- it should be the responisbility of `juju remove-unit
		// --force` to cause the unit to leave any relation scopes it may be
		// in -- but it's worth noting here all the same.
	),
}

func (s *UniterSuite) TestUniterRelations(c *gc.C) {
	s.runUniterTests(c, relationsTests)
}

var relationsErrorTests = []uniterTest{
	ut(
		"hook error during join of a relation",
		startupRelationError{"db-relation-joined"},
		waitUnit{
			status: params.StatusError,
			info:   `hook failed: "db-relation-joined"`,
			data: params.StatusData{
				"hook":        "db-relation-joined",
				"relation-id": 0,
				"remote-unit": "mysql/0",
			},
		},
	), ut(
		"hook error during change of a relation",
		startupRelationError{"db-relation-changed"},
		waitUnit{
			status: params.StatusError,
			info:   `hook failed: "db-relation-changed"`,
			data: params.StatusData{
				"hook":        "db-relation-changed",
				"relation-id": 0,
				"remote-unit": "mysql/0",
			},
		},
	), ut(
		"hook error after a unit departed",
		startupRelationError{"db-relation-departed"},
		waitHooks{"db-relation-joined mysql/0 db:0", "db-relation-changed mysql/0 db:0"},
		removeRelationUnit{"mysql/0"},
		waitUnit{
			status: params.StatusError,
			info:   `hook failed: "db-relation-departed"`,
			data: params.StatusData{
				"hook":        "db-relation-departed",
				"relation-id": 0,
				"remote-unit": "mysql/0",
			},
		},
	),
	ut(
		"hook error after a relation died",
		startupRelationError{"db-relation-broken"},
		waitHooks{"db-relation-joined mysql/0 db:0", "db-relation-changed mysql/0 db:0"},
		relationDying,
		waitUnit{
			status: params.StatusError,
			info:   `hook failed: "db-relation-broken"`,
			data: params.StatusData{
				"hook":        "db-relation-broken",
				"relation-id": 0,
			},
		},
	),
}

func (s *UniterSuite) TestUniterRelationErrors(c *gc.C) {
	s.runUniterTests(c, relationsErrorTests)
}

var subordinatesTests = []uniterTest{
	// Subordinates.
	ut(
		"unit becomes dying while subordinates exist",
		quickStart{},
		addSubordinateRelation{"juju-info"},
		waitSubordinateExists{"logging/0"},
		unitDying,
		waitSubordinateDying{},
		waitHooks{"stop"},
		verifyRunning{true},
		removeSubordinate{},
		waitUniterDead{},
	), ut(
		"new subordinate becomes necessary while old one is dying",
		quickStart{},
		addSubordinateRelation{"juju-info"},
		waitSubordinateExists{"logging/0"},
		removeSubordinateRelation{"juju-info"},
		// The subordinate Uniter would usually set Dying in this situation.
		subordinateDying,
		addSubordinateRelation{"logging-dir"},
		verifyRunning{},
		removeSubordinate{},
		waitSubordinateExists{"logging/1"},
	),
}

func (s *UniterSuite) TestUniterSubordinates(c *gc.C) {
	s.runUniterTests(c, subordinatesTests)
}

func (s *UniterSuite) runUniterTests(c *gc.C, uniterTests []uniterTest) {
	for i, t := range uniterTests {
		c.Logf("\ntest %d: %s\n", i, t.summary)
		func() {
			defer s.Reset(c)
			env, err := s.State.Environment()
			c.Assert(err, gc.IsNil)
			ctx := &context{
				s:       s,
				st:      s.State,
				uuid:    env.UUID(),
				path:    s.unitDir,
				dataDir: s.dataDir,
				charms:  coretesting.ResponseMap{},
			}
			ctx.run(c, t.steps)
		}()
	}
}

func (s *UniterSuite) TestSubordinateDying(c *gc.C) {
	// Create a test context for later use.
	ctx := &context{
		s:       s,
		st:      s.State,
		path:    filepath.Join(s.dataDir, "agents", "unit-u-0"),
		dataDir: s.dataDir,
		charms:  coretesting.ResponseMap{},
	}

	testing.AddStateServerMachine(c, ctx.st)

	// Create the subordinate service.
	dir := coretesting.Charms.ClonedDir(c.MkDir(), "logging")
	curl, err := charm.ParseURL("cs:quantal/logging")
	c.Assert(err, gc.IsNil)
	curl = curl.WithRevision(dir.Revision())
	step(c, ctx, addCharm{dir, curl})
	ctx.svc = s.AddTestingService(c, "u", ctx.sch)

	// Create the principal service and add a relation.
	wps := s.AddTestingService(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	wpu, err := wps.AddUnit()
	c.Assert(err, gc.IsNil)
	eps, err := s.State.InferEndpoints([]string{"wordpress", "u"})
	c.Assert(err, gc.IsNil)
	rel, err := s.State.AddRelation(eps...)
	c.Assert(err, gc.IsNil)

	// Create the subordinate unit by entering scope as the principal.
	wpru, err := rel.Unit(wpu)
	c.Assert(err, gc.IsNil)
	err = wpru.EnterScope(nil)
	c.Assert(err, gc.IsNil)
	ctx.unit, err = s.State.Unit("u/0")
	c.Assert(err, gc.IsNil)

	s.APILogin(c, ctx.unit)

	// Run the actual test.
	ctx.run(c, []stepper{
		serveCharm{},
		startUniter{},
		waitAddresses{},
		custom{func(c *gc.C, ctx *context) {
			c.Assert(rel.Destroy(), gc.IsNil)
		}},
		waitUniterDead{},
	})
}

func step(c *gc.C, ctx *context, s stepper) {
	c.Logf("%#v", s)
	s.step(c, ctx)
}

type ensureStateWorker struct {
}

func (s ensureStateWorker) step(c *gc.C, ctx *context) {
	addresses, err := ctx.st.Addresses()
	if err != nil || len(addresses) == 0 {
		testing.AddStateServerMachine(c, ctx.st)
	}
	addresses, err = ctx.st.APIAddresses()
	c.Assert(err, gc.IsNil)
	c.Assert(addresses, gc.HasLen, 1)
}

type createCharm struct {
	revision  int
	badHooks  []string
	customize func(*gc.C, *context, string)
}

var charmHooks = []string{
	"install", "start", "config-changed", "upgrade-charm", "stop",
	"db-relation-joined", "db-relation-changed", "db-relation-departed",
	"db-relation-broken",
}

func (s createCharm) step(c *gc.C, ctx *context) {
	base := coretesting.Charms.ClonedDirPath(c.MkDir(), "wordpress")
	for _, name := range charmHooks {
		path := filepath.Join(base, "hooks", name)
		good := true
		for _, bad := range s.badHooks {
			if name == bad {
				good = false
			}
		}
		ctx.writeHook(c, path, good)
	}
	if s.customize != nil {
		s.customize(c, ctx, base)
	}
	dir, err := charm.ReadDir(base)
	c.Assert(err, gc.IsNil)
	err = dir.SetDiskRevision(s.revision)
	c.Assert(err, gc.IsNil)
	step(c, ctx, addCharm{dir, curl(s.revision)})
}

type addCharm struct {
	dir  *charm.Dir
	curl *charm.URL
}

func (s addCharm) step(c *gc.C, ctx *context) {
	var buf bytes.Buffer
	err := s.dir.BundleTo(&buf)
	c.Assert(err, gc.IsNil)
	body := buf.Bytes()
	hasher := sha256.New()
	_, err = io.Copy(hasher, &buf)
	c.Assert(err, gc.IsNil)
	hash := hex.EncodeToString(hasher.Sum(nil))
	key := fmt.Sprintf("/charms/%s/%d", s.dir.Meta().Name, s.dir.Revision())
	hurl, err := url.Parse(coretesting.Server.URL + key)
	c.Assert(err, gc.IsNil)
	ctx.charms[key] = coretesting.Response{200, nil, body}
	ctx.sch, err = ctx.st.AddCharm(s.dir, s.curl, hurl, hash)
	c.Assert(err, gc.IsNil)
}

type serveCharm struct{}

func (serveCharm) step(c *gc.C, ctx *context) {
	coretesting.Server.ResponseMap(1, ctx.charms)
}

type createServiceAndUnit struct {
	serviceName string
}

func (csau createServiceAndUnit) step(c *gc.C, ctx *context) {
	if csau.serviceName == "" {
		csau.serviceName = "u"
	}
	sch, err := ctx.st.Charm(curl(0))
	c.Assert(err, gc.IsNil)
	svc := ctx.s.AddTestingService(c, csau.serviceName, sch)
	unit, err := svc.AddUnit()
	c.Assert(err, gc.IsNil)

	// Assign the unit to a provisioned machine to match expected state.
	err = unit.AssignToNewMachine()
	c.Assert(err, gc.IsNil)
	mid, err := unit.AssignedMachineId()
	c.Assert(err, gc.IsNil)
	machine, err := ctx.st.Machine(mid)
	c.Assert(err, gc.IsNil)
	err = machine.SetProvisioned("i-exist", "fake_nonce", nil)
	c.Assert(err, gc.IsNil)
	ctx.svc = svc
	ctx.unit = unit

	ctx.s.APILogin(c, unit)
}

type createUniter struct{}

func (createUniter) step(c *gc.C, ctx *context) {
	step(c, ctx, ensureStateWorker{})
	step(c, ctx, createServiceAndUnit{})
	step(c, ctx, startUniter{})
	step(c, ctx, waitAddresses{})
}

type waitAddresses struct{}

func (waitAddresses) step(c *gc.C, ctx *context) {
	timeout := time.After(worstCase)
	for {
		select {
		case <-timeout:
			c.Fatalf("timed out waiting for unit addresses")
		case <-time.After(coretesting.ShortWait):
			err := ctx.unit.Refresh()
			if err != nil {
				c.Fatalf("unit refresh failed: %v", err)
			}
			// GZ 2013-07-10: Hardcoded values from dummy environ
			//                special cased here, questionable.
			private, _ := ctx.unit.PrivateAddress()
			if private != "private.dummy.address.example.com" {
				continue
			}
			public, _ := ctx.unit.PublicAddress()
			if public != "public.dummy.address.example.com" {
				continue
			}
			return
		}
	}
}

type startUniter struct {
	unitTag string
}

func (s startUniter) step(c *gc.C, ctx *context) {
	if s.unitTag == "" {
		s.unitTag = "unit-u-0"
	}
	if ctx.uniter != nil {
		panic("don't start two uniters!")
	}
	if ctx.s.uniter == nil {
		panic("API connection not established")
	}
	ctx.uniter = uniter.NewUniter(ctx.s.uniter, s.unitTag, ctx.dataDir)
	uniter.SetUniterObserver(ctx.uniter, ctx)
}

type waitUniterDead struct {
	err string
}

func (s waitUniterDead) step(c *gc.C, ctx *context) {
	if s.err != "" {
		err := s.waitDead(c, ctx)
		c.Assert(err, gc.ErrorMatches, s.err)
		return
	}
	// In the default case, we're waiting for worker.ErrTerminateAgent, but
	// the path to that error can be tricky. If the unit becomes Dead at an
	// inconvenient time, unrelated calls can fail -- as they should -- but
	// not be detected as worker.ErrTerminateAgent. In this case, we restart
	// the uniter and check that it fails as expected when starting up; this
	// mimics the behaviour of the unit agent and verifies that the UA will,
	// eventually, see the correct error and respond appropriately.
	err := s.waitDead(c, ctx)
	if err != worker.ErrTerminateAgent {
		step(c, ctx, startUniter{})
		err = s.waitDead(c, ctx)
	}
	c.Assert(err, gc.Equals, worker.ErrTerminateAgent)
	err = ctx.unit.Refresh()
	c.Assert(err, gc.IsNil)
	c.Assert(ctx.unit.Life(), gc.Equals, state.Dead)
}

func (s waitUniterDead) waitDead(c *gc.C, ctx *context) error {
	u := ctx.uniter
	ctx.uniter = nil
	timeout := time.After(worstCase)
	for {
		// The repeated StartSync is to ensure timely completion of this method
		// in the case(s) where a state change causes a uniter action which
		// causes a state change which causes a uniter action, in which case we
		// need more than one sync. At the moment there's only one situation
		// that causes this -- setting the unit's service to Dying -- but it's
		// not an intrinsically insane pattern of action (and helps to simplify
		// the filter code) so this test seems like a small price to pay.
		ctx.s.BackingState.StartSync()
		select {
		case <-u.Dead():
			return u.Wait()
		case <-time.After(coretesting.ShortWait):
			continue
		case <-timeout:
			c.Fatalf("uniter still alive")
		}
	}
}

type stopUniter struct {
	err string
}

func (s stopUniter) step(c *gc.C, ctx *context) {
	u := ctx.uniter
	ctx.uniter = nil
	err := u.Stop()
	if s.err == "" {
		c.Assert(err, gc.IsNil)
	} else {
		c.Assert(err, gc.ErrorMatches, s.err)
	}
}

type verifyWaiting struct{}

func (s verifyWaiting) step(c *gc.C, ctx *context) {
	step(c, ctx, stopUniter{})
	step(c, ctx, startUniter{})
	step(c, ctx, waitHooks{})
}

type verifyRunning struct {
	noHooks bool
}

func (s verifyRunning) step(c *gc.C, ctx *context) {
	step(c, ctx, stopUniter{})
	step(c, ctx, startUniter{})
	if s.noHooks {
		step(c, ctx, waitHooks{})
	} else {
		step(c, ctx, waitHooks{"config-changed"})
	}
}

type startupError struct {
	badHook string
}

func (s startupError) step(c *gc.C, ctx *context) {
	step(c, ctx, createCharm{badHooks: []string{s.badHook}})
	step(c, ctx, serveCharm{})
	step(c, ctx, createUniter{})
	step(c, ctx, waitUnit{
		status: params.StatusError,
		info:   fmt.Sprintf(`hook failed: %q`, s.badHook),
	})
	for _, hook := range []string{"install", "config-changed", "start"} {
		if hook == s.badHook {
			step(c, ctx, waitHooks{"fail-" + hook})
			break
		}
		step(c, ctx, waitHooks{hook})
	}
	step(c, ctx, verifyCharm{})
}

type quickStart struct{}

func (s quickStart) step(c *gc.C, ctx *context) {
	step(c, ctx, createCharm{})
	step(c, ctx, serveCharm{})
	step(c, ctx, createUniter{})
	step(c, ctx, waitUnit{status: params.StatusStarted})
	step(c, ctx, waitHooks{"install", "config-changed", "start"})
	step(c, ctx, verifyCharm{})
}

type quickStartRelation struct{}

func (s quickStartRelation) step(c *gc.C, ctx *context) {
	step(c, ctx, quickStart{})
	step(c, ctx, addRelation{})
	step(c, ctx, addRelationUnit{})
	step(c, ctx, waitHooks{"db-relation-joined mysql/0 db:0", "db-relation-changed mysql/0 db:0"})
	step(c, ctx, verifyRunning{})
}

type startupRelationError struct {
	badHook string
}

func (s startupRelationError) step(c *gc.C, ctx *context) {
	step(c, ctx, createCharm{badHooks: []string{s.badHook}})
	step(c, ctx, serveCharm{})
	step(c, ctx, createUniter{})
	step(c, ctx, waitUnit{status: params.StatusStarted})
	step(c, ctx, waitHooks{"install", "config-changed", "start"})
	step(c, ctx, verifyCharm{})
	step(c, ctx, addRelation{})
	step(c, ctx, addRelationUnit{})
}

type resolveError struct {
	resolved state.ResolvedMode
}

func (s resolveError) step(c *gc.C, ctx *context) {
	err := ctx.unit.SetResolved(s.resolved)
	c.Assert(err, gc.IsNil)
}

type waitUnit struct {
	status   params.Status
	info     string
	data     params.StatusData
	charm    int
	resolved state.ResolvedMode
}

func (s waitUnit) step(c *gc.C, ctx *context) {
	timeout := time.After(worstCase)
	for {
		ctx.s.BackingState.StartSync()
		select {
		case <-time.After(coretesting.ShortWait):
			err := ctx.unit.Refresh()
			if err != nil {
				c.Fatalf("cannot refresh unit: %v", err)
			}
			resolved := ctx.unit.Resolved()
			if resolved != s.resolved {
				c.Logf("want resolved mode %q, got %q; still waiting", s.resolved, resolved)
				continue
			}
			url, ok := ctx.unit.CharmURL()
			if !ok || *url != *curl(s.charm) {
				var got string
				if ok {
					got = url.String()
				}
				c.Logf("want unit charm %q, got %q; still waiting", curl(s.charm), got)
				continue
			}
			status, info, data, err := ctx.unit.Status()
			c.Assert(err, gc.IsNil)
			if status != s.status {
				c.Logf("want unit status %q, got %q; still waiting", s.status, status)
				continue
			}
			if info != s.info {
				c.Logf("want unit status info %q, got %q; still waiting", s.info, info)
				continue
			}
			if s.data != nil {
				if len(data) != len(s.data) {
					c.Logf("want %d unit status data value(s), got %d; still waiting", len(s.data), len(data))
					continue
				}
				for key, value := range s.data {
					if data[key] != value {
						c.Logf("want unit status data value %q for key %q, got %q; still waiting",
							value, key, data[key])
						continue
					}
				}
			}
			return
		case <-timeout:
			c.Fatalf("never reached desired status")
		}
	}
}

type waitHooks []string

func (s waitHooks) step(c *gc.C, ctx *context) {
	if len(s) == 0 {
		// Give unwanted hooks a moment to run...
		ctx.s.BackingState.StartSync()
		time.Sleep(coretesting.ShortWait)
	}
	ctx.hooks = append(ctx.hooks, s...)
	c.Logf("waiting for hooks: %#v", ctx.hooks)
	match, overshoot := ctx.matchHooks(c)
	if overshoot && len(s) == 0 {
		c.Fatalf("ran more hooks than expected")
	}
	if match {
		return
	}
	timeout := time.After(worstCase)
	for {
		ctx.s.BackingState.StartSync()
		select {
		case <-time.After(coretesting.ShortWait):
			if match, _ = ctx.matchHooks(c); match {
				return
			}
		case <-timeout:
			c.Fatalf("never got expected hooks")
		}
	}
}

type fixHook struct {
	name string
}

func (s fixHook) step(c *gc.C, ctx *context) {
	path := filepath.Join(ctx.path, "charm", "hooks", s.name)
	ctx.writeHook(c, path, true)
}

type changeConfig map[string]interface{}

func (s changeConfig) step(c *gc.C, ctx *context) {
	err := ctx.svc.UpdateConfigSettings(charm.Settings(s))
	c.Assert(err, gc.IsNil)
}

type upgradeCharm struct {
	revision int
	forced   bool
}

func (s upgradeCharm) step(c *gc.C, ctx *context) {
	sch, err := ctx.st.Charm(curl(s.revision))
	c.Assert(err, gc.IsNil)
	err = ctx.svc.SetCharm(sch, s.forced)
	c.Assert(err, gc.IsNil)
	serveCharm{}.step(c, ctx)
}

type verifyCharm struct {
	revision int
	dirty    bool
}

func (s verifyCharm) step(c *gc.C, ctx *context) {
	if !s.dirty {
		path := filepath.Join(ctx.path, "charm", "revision")
		content, err := ioutil.ReadFile(path)
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

type startUpgradeError struct{}

func (s startUpgradeError) step(c *gc.C, ctx *context) {
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
		verifyCharm{},

		createCharm{
			revision: 1,
			customize: func(c *gc.C, ctx *context, path string) {
				data := filepath.Join(path, "data")
				err := ioutil.WriteFile(data, []byte("<nelson>ha ha</nelson>"), 0644)
				c.Assert(err, gc.IsNil)
				ignore := filepath.Join(path, "ignore")
				err = ioutil.WriteFile(ignore, []byte("anything"), 0644)
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
		verifyCharm{dirty: true},
	}
	for _, s_ := range steps {
		step(c, ctx, s_)
	}
}

type addRelation struct {
	testing.JujuConnSuite
}

func (s addRelation) step(c *gc.C, ctx *context) {
	if ctx.relation != nil {
		panic("don't add two relations!")
	}
	if ctx.relatedSvc == nil {
		ctx.relatedSvc = ctx.s.AddTestingService(c, "mysql", ctx.s.AddTestingCharm(c, "mysql"))
	}
	eps, err := ctx.st.InferEndpoints([]string{"u", "mysql"})
	c.Assert(err, gc.IsNil)
	ctx.relation, err = ctx.st.AddRelation(eps...)
	c.Assert(err, gc.IsNil)
	ctx.relationUnits = map[string]*state.RelationUnit{}
}

type addRelationUnit struct{}

func (s addRelationUnit) step(c *gc.C, ctx *context) {
	u, err := ctx.relatedSvc.AddUnit()
	c.Assert(err, gc.IsNil)
	ru, err := ctx.relation.Unit(u)
	c.Assert(err, gc.IsNil)
	err = ru.EnterScope(nil)
	c.Assert(err, gc.IsNil)
	ctx.relationUnits[u.Name()] = ru
}

type changeRelationUnit struct {
	name string
}

func (s changeRelationUnit) step(c *gc.C, ctx *context) {
	settings, err := ctx.relationUnits[s.name].Settings()
	c.Assert(err, gc.IsNil)
	key := "madness?"
	raw, _ := settings.Get(key)
	val, _ := raw.(string)
	if val == "" {
		val = "this is juju"
	} else {
		val += "u"
	}
	settings.Set(key, val)
	_, err = settings.Write()
	c.Assert(err, gc.IsNil)
}

type removeRelationUnit struct {
	name string
}

func (s removeRelationUnit) step(c *gc.C, ctx *context) {
	err := ctx.relationUnits[s.name].LeaveScope()
	c.Assert(err, gc.IsNil)
	ctx.relationUnits[s.name] = nil
}

type relationState struct {
	removed bool
	life    state.Life
}

func (s relationState) step(c *gc.C, ctx *context) {
	err := ctx.relation.Refresh()
	if s.removed {
		c.Assert(err, jc.Satisfies, errors.IsNotFoundError)
		return
	}
	c.Assert(err, gc.IsNil)
	c.Assert(ctx.relation.Life(), gc.Equals, s.life)

}

type addSubordinateRelation struct {
	ifce string
}

func (s addSubordinateRelation) step(c *gc.C, ctx *context) {
	if _, err := ctx.st.Service("logging"); errors.IsNotFoundError(err) {
		ctx.s.AddTestingService(c, "logging", ctx.s.AddTestingCharm(c, "logging"))
	}
	eps, err := ctx.st.InferEndpoints([]string{"logging", "u:" + s.ifce})
	c.Assert(err, gc.IsNil)
	_, err = ctx.st.AddRelation(eps...)
	c.Assert(err, gc.IsNil)
}

type removeSubordinateRelation struct {
	ifce string
}

func (s removeSubordinateRelation) step(c *gc.C, ctx *context) {
	eps, err := ctx.st.InferEndpoints([]string{"logging", "u:" + s.ifce})
	c.Assert(err, gc.IsNil)
	rel, err := ctx.st.EndpointsRelation(eps...)
	c.Assert(err, gc.IsNil)
	err = rel.Destroy()
	c.Assert(err, gc.IsNil)
}

type waitSubordinateExists struct {
	name string
}

func (s waitSubordinateExists) step(c *gc.C, ctx *context) {
	timeout := time.After(worstCase)
	for {
		ctx.s.BackingState.StartSync()
		select {
		case <-timeout:
			c.Fatalf("subordinate was not created")
		case <-time.After(coretesting.ShortWait):
			var err error
			ctx.subordinate, err = ctx.st.Unit(s.name)
			if errors.IsNotFoundError(err) {
				continue
			}
			c.Assert(err, gc.IsNil)
			return
		}
	}
}

type waitSubordinateDying struct{}

func (waitSubordinateDying) step(c *gc.C, ctx *context) {
	timeout := time.After(worstCase)
	for {
		ctx.s.BackingState.StartSync()
		select {
		case <-timeout:
			c.Fatalf("subordinate was not made Dying")
		case <-time.After(coretesting.ShortWait):
			err := ctx.subordinate.Refresh()
			c.Assert(err, gc.IsNil)
			if ctx.subordinate.Life() != state.Dying {
				continue
			}
		}
		break
	}
}

type removeSubordinate struct{}

func (removeSubordinate) step(c *gc.C, ctx *context) {
	err := ctx.subordinate.EnsureDead()
	c.Assert(err, gc.IsNil)
	err = ctx.subordinate.Remove()
	c.Assert(err, gc.IsNil)
	ctx.subordinate = nil
}

type assertYaml struct {
	path   string
	expect map[string]interface{}
}

func (s assertYaml) step(c *gc.C, ctx *context) {
	data, err := ioutil.ReadFile(filepath.Join(ctx.path, s.path))
	c.Assert(err, gc.IsNil)
	actual := make(map[string]interface{})
	err = goyaml.Unmarshal(data, &actual)
	c.Assert(err, gc.IsNil)
	c.Assert(actual, gc.DeepEquals, s.expect)
}

type writeFile struct {
	path string
	mode os.FileMode
}

func (s writeFile) step(c *gc.C, ctx *context) {
	path := filepath.Join(ctx.path, s.path)
	dir := filepath.Dir(path)
	err := os.MkdirAll(dir, 0755)
	c.Assert(err, gc.IsNil)
	err = ioutil.WriteFile(path, nil, s.mode)
	c.Assert(err, gc.IsNil)
}

type chmod struct {
	path string
	mode os.FileMode
}

func (s chmod) step(c *gc.C, ctx *context) {
	path := filepath.Join(ctx.path, s.path)
	err := os.Chmod(path, s.mode)
	c.Assert(err, gc.IsNil)
}

type custom struct {
	f func(*gc.C, *context)
}

func (s custom) step(c *gc.C, ctx *context) {
	s.f(c, ctx)
}

var serviceDying = custom{func(c *gc.C, ctx *context) {
	c.Assert(ctx.svc.Destroy(), gc.IsNil)
}}

var relationDying = custom{func(c *gc.C, ctx *context) {
	c.Assert(ctx.relation.Destroy(), gc.IsNil)
}}

var unitDying = custom{func(c *gc.C, ctx *context) {
	c.Assert(ctx.unit.Destroy(), gc.IsNil)
}}

var unitDead = custom{func(c *gc.C, ctx *context) {
	c.Assert(ctx.unit.EnsureDead(), gc.IsNil)
}}

var subordinateDying = custom{func(c *gc.C, ctx *context) {
	c.Assert(ctx.subordinate.Destroy(), gc.IsNil)
}}

func curl(revision int) *charm.URL {
	return charm.MustParseURL("cs:quantal/wordpress").WithRevision(revision)
}

func appendHook(c *gc.C, charm, name, data string) {
	path := filepath.Join(charm, "hooks", name)
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0755)
	c.Assert(err, gc.IsNil)
	defer f.Close()
	_, err = f.Write([]byte(data))
	c.Assert(err, gc.IsNil)
}

func renameRelation(c *gc.C, charmPath, oldName, newName string) {
	path := filepath.Join(charmPath, "metadata.yaml")
	f, err := os.Open(path)
	c.Assert(err, gc.IsNil)
	defer f.Close()
	meta, err := charm.ReadMeta(f)
	c.Assert(err, gc.IsNil)

	replace := func(what map[string]charm.Relation) bool {
		for relName, relation := range what {
			if relName == oldName {
				what[newName] = relation
				delete(what, oldName)
				return true
			}
		}
		return false
	}
	replaced := replace(meta.Provides) || replace(meta.Requires) || replace(meta.Peers)
	c.Assert(replaced, gc.Equals, true, gc.Commentf("charm %q does not implement relation %q", charmPath, oldName))

	newmeta, err := goyaml.Marshal(meta)
	c.Assert(err, gc.IsNil)
	ioutil.WriteFile(path, newmeta, 0644)

	f, err = os.Open(path)
	c.Assert(err, gc.IsNil)
	defer f.Close()
	meta, err = charm.ReadMeta(f)
	c.Assert(err, gc.IsNil)
}

func createHookLock(c *gc.C, dataDir string) *fslock.Lock {
	lockDir := filepath.Join(dataDir, "locks")
	lock, err := fslock.NewLock(lockDir, "uniter-hook-execution")
	c.Assert(err, gc.IsNil)
	return lock
}

type acquireHookSyncLock struct {
	message string
}

func (s acquireHookSyncLock) step(c *gc.C, ctx *context) {
	lock := createHookLock(c, ctx.dataDir)
	c.Assert(lock.IsLocked(), gc.Equals, false)
	err := lock.Lock(s.message)
	c.Assert(err, gc.IsNil)
}

var releaseHookSyncLock = custom{func(c *gc.C, ctx *context) {
	lock := createHookLock(c, ctx.dataDir)
	// Force the release.
	err := lock.BreakLock()
	c.Assert(err, gc.IsNil)
}}

var verifyHookSyncLockUnlocked = custom{func(c *gc.C, ctx *context) {
	lock := createHookLock(c, ctx.dataDir)
	c.Assert(lock.IsLocked(), jc.IsFalse)
}}

var verifyHookSyncLockLocked = custom{func(c *gc.C, ctx *context) {
	lock := createHookLock(c, ctx.dataDir)
	c.Assert(lock.IsLocked(), jc.IsTrue)
}}

type setProxySettings osenv.ProxySettings

func (s setProxySettings) step(c *gc.C, ctx *context) {
	old, err := ctx.st.EnvironConfig()
	c.Assert(err, gc.IsNil)
	cfg, err := old.Apply(map[string]interface{}{
		"http-proxy":  s.Http,
		"https-proxy": s.Https,
		"ftp-proxy":   s.Ftp,
	})
	c.Assert(err, gc.IsNil)
	err = ctx.st.SetEnvironConfig(cfg, old)
	c.Assert(err, gc.IsNil)
	// wait for the new values...
	expected := (osenv.ProxySettings)(s)
	for attempt := coretesting.LongAttempt.Start(); attempt.Next(); {
		if ctx.uniter.GetProxyValues() == expected {
			return
		}
	}
	c.Fatal("settings didn't get noticed by the uniter")
}

type runCommands []string

func (cmds runCommands) step(c *gc.C, ctx *context) {
	commands := strings.Join(cmds, "\n")
	result, err := ctx.uniter.RunCommands(commands)
	c.Assert(err, gc.IsNil)
	c.Check(result.Code, gc.Equals, 0)
	c.Check(string(result.Stdout), gc.Equals, "")
	c.Check(string(result.Stderr), gc.Equals, "")
}

type asyncRunCommands []string

func (cmds asyncRunCommands) step(c *gc.C, ctx *context) {
	commands := strings.Join(cmds, "\n")
	socketPath := filepath.Join(ctx.path, uniter.RunListenerFile)

	go func() {
		// make sure the socket exists
		client, err := rpc.Dial("unix", socketPath)
		c.Assert(err, gc.IsNil)
		defer client.Close()

		var result utilexec.ExecResponse
		err = client.Call(uniter.JujuRunEndpoint, commands, &result)
		c.Assert(err, gc.IsNil)
		c.Check(result.Code, gc.Equals, 0)
		c.Check(string(result.Stdout), gc.Equals, "")
		c.Check(string(result.Stderr), gc.Equals, "")
	}()
}

type verifyFile struct {
	filename string
	content  string
}

func (verify verifyFile) fileExists() bool {
	_, err := os.Stat(verify.filename)
	return err == nil
}

func (verify verifyFile) checkContent(c *gc.C) {
	content, err := ioutil.ReadFile(verify.filename)
	c.Assert(err, gc.IsNil)
	c.Assert(string(content), gc.Equals, verify.content)
}

func (verify verifyFile) step(c *gc.C, ctx *context) {
	if verify.fileExists() {
		verify.checkContent(c)
		return
	}
	c.Logf("waiting for file: %s", verify.filename)
	timeout := time.After(worstCase)
	for {
		select {
		case <-time.After(coretesting.ShortWait):
			if verify.fileExists() {
				verify.checkContent(c)
				return
			}
		case <-timeout:
			c.Fatalf("file does not exist")
		}
	}
}

// verify that the file does not exist
type verifyNoFile struct {
	filename string
}

func (verify verifyNoFile) step(c *gc.C, ctx *context) {
	c.Assert(verify.filename, jc.DoesNotExist)
	// Wait a short time and check again.
	time.Sleep(coretesting.ShortWait)
	c.Assert(verify.filename, jc.DoesNotExist)
}
