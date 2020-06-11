// Copyright 2012-2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter_test

import (
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"

	corecharm "github.com/juju/charm/v7"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/agent/tools"
	"github.com/juju/juju/component/all"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state"
	"github.com/juju/juju/testcharms"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker/uniter"
	"github.com/juju/juju/worker/uniter/operation"
	"github.com/juju/juju/worker/uniter/remotestate"
)

type UniterSuite struct {
	testing.JujuConnSuite
	dataDir string
	unitDir string

	updateStatusHookTicker *manualTicker
	runner                 *mockRunner
	deployer               *mockDeployer
}

var _ = gc.Suite(&UniterSuite{})

// This guarantees that we get proper platform
// specific error directly from their source
// This works on both windows and unix
var errNotDir = syscall.ENOTDIR.Error()

func (s *UniterSuite) SetUpSuite(c *gc.C) {
	s.JujuConnSuite.SetUpSuite(c)
	s.dataDir = c.MkDir()
	toolsDir := tools.ToolsDir(s.dataDir, "unit-u-0")
	err := os.MkdirAll(toolsDir, 0755)
	c.Assert(err, jc.ErrorIsNil)

	s.PatchEnvironment("LC_ALL", "en_US")
	s.unitDir = filepath.Join(s.dataDir, "agents", "unit-u-0")
	err = all.RegisterForServer()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *UniterSuite) SetUpTest(c *gc.C) {
	s.updateStatusHookTicker = newManualTicker()
	s.runner = &mockRunner{}
	s.deployer = &mockDeployer{}
	s.JujuConnSuite.SetUpTest(c)
}

func (s *UniterSuite) TearDownTest(c *gc.C) {
	s.ResetContext(c)
	s.JujuConnSuite.TearDownTest(c)
}

func (s *UniterSuite) Reset(c *gc.C) {
	s.JujuConnSuite.Reset(c)
	s.ResetContext(c)
}

func (s *UniterSuite) ResetContext(c *gc.C) {
	s.runner = &mockRunner{}
	s.deployer = &mockDeployer{}
	err := os.RemoveAll(s.unitDir)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *UniterSuite) runUniterTests(c *gc.C, uniterTests []uniterTest) {
	for i, t := range uniterTests {
		c.Logf("\ntest %d: %s\n", i, t.summary)
		func() {
			defer s.Reset(c)

			ctx := &context{
				s:                      s,
				st:                     s.State,
				uuid:                   s.State.ModelUUID(),
				path:                   s.unitDir,
				dataDir:                s.dataDir,
				charms:                 make(map[string][]byte),
				leaseManager:           s.LeaseManager,
				updateStatusHookTicker: s.updateStatusHookTicker,
				charmDirGuard:          &mockCharmDirGuard{},
				runner:                 s.runner,
				deployer:               s.deployer,
			}
			ctx.run(c, t.steps)
		}()
	}
}

func (s *UniterSuite) runUniterTest(c *gc.C, steps ...stepper) {
	ctx := &context{
		s:                      s,
		st:                     s.State,
		uuid:                   s.State.ModelUUID(),
		path:                   s.unitDir,
		dataDir:                s.dataDir,
		charms:                 make(map[string][]byte),
		leaseManager:           s.LeaseManager,
		updateStatusHookTicker: s.updateStatusHookTicker,
		charmDirGuard:          &mockCharmDirGuard{},
		runner:                 s.runner,
		deployer:               s.deployer,
	}
	ctx.run(c, steps)
}

func (s *UniterSuite) TestUniterStartup(c *gc.C) {
	s.runUniterTests(c, []uniterTest{
		// Check conditions that can cause the uniter to fail to start.
		ut(
			"unknown unit",
			// We still need to create a unit, because that's when we also
			// connect to the API, but here we use a different application
			// (and hence unit) name.
			createCharm{},
			createApplicationAndUnit{applicationName: "w"},
			startUniter{unitTag: "unit-u-0"},
			waitUniterDead{err: `failed to initialize uniter for "unit-u-0": permission denied`},
		),
	})
}

func (s *UniterSuite) TestPreviousDownloadsCleared(c *gc.C) {
	s.runUniterTests(c, []uniterTest{
		ut(
			"Ensure stale download files are cleared on uniter startup",
			createCharm{},
			serveCharm{},
			ensureStateWorker{},
			createApplicationAndUnit{},
			startUniter{},
			waitAddresses{},
			waitUnitAgent{status: status.Idle},
			verifyDeployed{},
		),
	})
}

func (s *UniterSuite) TestUniterBootstrap(c *gc.C) {
	//TODO(bogdanteleaga): Fix this on windows
	if runtime.GOOS == "windows" {
		c.Skip("bug 1403084: currently does not work on windows")
	}
	s.runUniterTests(c, []uniterTest{
		// Check error conditions during unit bootstrap phase.
		ut(
			"insane deployment",
			createCharm{},
			serveCharm{},
			writeFile{"charm", 0644},
			createUniter{},
			waitUniterDead{err: `executing operation "install cs:quantal/wordpress-0": .*` + errNotDir},
		), ut(
			"charm cannot be downloaded",
			createCharm{},
			// don't serve charm
			createUniter{},
			waitUniterDead{err: `preparing operation "install cs:quantal/wordpress-0": failed to download charm .* not found`},
		),
	})
}

func (s *UniterSuite) TestUniterStartupStatus(c *gc.C) {
	s.runUniterTests(c, []uniterTest{
		ut(
			"unit status and message at startup",
			createCharm{},
			serveCharm{},
			ensureStateWorker{},
			createApplicationAndUnit{},
			startUniter{
				newExecutorFunc: executorFunc(c),
			},
			waitUnitAgent{
				statusGetter: unitStatusGetter,
				status:       status.Waiting,
				info:         status.MessageInitializingAgent,
			},
			waitUnitAgent{
				status: status.Failed,
				info:   "resolver loop error",
			},
			expectError{".*some error occurred.*"},
		),
	})
}

func (s *UniterSuite) TestUniterStartupStatusCharmProfile(c *gc.C) {
	// addCharmProfile customises the wordpress charm's metadata,
	// adding an lxd profile for the charm. We do it here rather
	// than in the charm itself to avoid modifying all of the other
	// scenarios.
	addCharmProfile := func(c *gc.C, ctx *context, path string) {
		f, err := os.OpenFile(filepath.Join(path, "lxd-profile.yaml"), os.O_RDWR|os.O_CREATE, 0644)
		c.Assert(err, jc.ErrorIsNil)
		defer func() {
			err := f.Close()
			c.Assert(err, jc.ErrorIsNil)
		}()
		_, err = io.WriteString(f, `
config:
  security.nesting: "false"
  security.privileged: "true"`)
		c.Assert(err, jc.ErrorIsNil)
	}

	s.runUniterTests(c, []uniterTest{
		ut(
			"unit status and message at startup, charm waiting for profile",
			createCharm{customize: addCharmProfile},
			serveCharm{},
			ensureStateWorker{},
			createApplicationAndUnit{},
			startUniter{
				newExecutorFunc: executorFunc(c),
			},
			waitUnitAgent{
				statusGetter: unitStatusGetter,
				status:       status.Waiting,
				info:         "required charm profile not yet applied to machine",
			},
			expectError{"required charm profile on machine not found"},
		),
		ut(
			"unit status and message at startup, charm profile found",
			createCharm{customize: addCharmProfile},
			serveCharm{},
			ensureStateWorker{},
			createApplicationAndUnit{},
			addCharmProfileToMachine{profiles: []string{"default", "juju-model-u-0"}},
			startUniter{
				newExecutorFunc: executorFunc(c),
			},
			waitUnitAgent{
				statusGetter: unitStatusGetter,
				status:       status.Waiting,
				info:         status.MessageInitializingAgent,
			},
			verifyRunning{},
		),
	})
}

func (s *UniterSuite) TestUniterInstallHook(c *gc.C) {
	s.runUniterTests(c, []uniterTest{
		ut(
			"install hook fail and resolve",
			startupError{"install"},
			verifyWaiting{},

			resolveError{state.ResolvedNoHooks},
			waitUnitAgent{
				status: status.Idle,
			},
			waitUnitAgent{
				statusGetter: unitStatusGetter,
				status:       status.Unknown,
			},
			waitHooks{"leader-elected", "config-changed", "start"},
		), ut(
			"install hook fail and retry",
			startupError{"install"},
			verifyWaiting{},

			resolveError{state.ResolvedRetryHooks},
			waitUnitAgent{
				statusGetter: unitStatusGetter,
				status:       status.Error,
				info:         `hook failed: "install"`,
				data: map[string]interface{}{
					"hook": "install",
				},
			},
			waitHooks{"fail-install"},
			fixHook{"install"},
			verifyWaiting{},

			resolveError{state.ResolvedRetryHooks},
			waitUnitAgent{
				status: status.Idle,
			},
			waitHooks{"install", "leader-elected", "config-changed", "start"},
		),
	})
}

func (s *UniterSuite) TestUniterUpdateStatusHook(c *gc.C) {
	s.runUniterTests(c, []uniterTest{
		ut(
			"update status hook runs on timer",
			createCharm{},
			serveCharm{},
			createUniter{},
			waitHooks(startupHooks(false)),
			waitUnitAgent{status: status.Idle},
			updateStatusHookTick{},
			waitHooks{"update-status"},
		),
	})
}

func (s *UniterSuite) TestNoUniterUpdateStatusHookInError(c *gc.C) {
	s.runUniterTests(c, []uniterTest{
		ut(
			"update status hook doesn't run if in error",
			startupError{"start"},
			waitHooks{},
			updateStatusHookTick{},
			waitHooks{},

			// Resolve and hook should run.
			resolveError{state.ResolvedNoHooks},
			waitUnitAgent{
				status: status.Idle,
			},
			waitHooks{},
			updateStatusHookTick{},
			waitHooks{"update-status"},
		),
	})
}

func (s *UniterSuite) TestUniterStartHook(c *gc.C) {
	s.runUniterTests(c, []uniterTest{
		ut(
			"start hook fail and resolve",
			startupError{"start"},
			verifyWaiting{},

			resolveError{state.ResolvedNoHooks},
			waitUnitAgent{
				status: status.Idle,
			},
			waitUnitAgent{
				statusGetter: unitStatusGetter,
				status:       status.Maintenance,
				info:         "installing charm software",
			},
			waitHooks{},
			verifyRunning{},
		), ut(
			"start hook fail and retry",
			startupError{"start"},
			verifyWaiting{},

			resolveError{state.ResolvedRetryHooks},
			waitUnitAgent{
				statusGetter: unitStatusGetter,
				status:       status.Error,
				info:         `hook failed: "start"`,
				data: map[string]interface{}{
					"hook": "start",
				},
			},
			waitHooks{"fail-start"},
			verifyWaiting{},

			fixHook{"start"},
			resolveError{state.ResolvedRetryHooks},
			waitUnitAgent{
				status: status.Idle,
			},
			waitHooks{"start"},
			verifyRunning{},
		), ut(
			"start hook after reboot",
			quickStart{},
			stopUniter{},
			startUniter{
				rebootQuerier: fakeRebootQuerier{
					rebootDetected: true,
				},
			},
			// Since the unit has already been started before and
			// a reboot was detected, we expect the uniter to
			// queue a start hook to notify the charms about the
			// reboot.
			waitHooks{"start"},
			verifyRunning{},
		),
	})
}

func (s *UniterSuite) TestUniterMultipleErrors(c *gc.C) {
	s.runUniterTests(c, []uniterTest{
		ut(
			"resolved is cleared before moving on to next hook",
			createCharm{badHooks: []string{"install", "leader-elected", "config-changed", "start"}},
			serveCharm{},
			createUniter{},
			waitUnitAgent{
				statusGetter: unitStatusGetter,
				status:       status.Error,
				info:         `hook failed: "install"`,
				data: map[string]interface{}{
					"hook": "install",
				},
			},
			resolveError{state.ResolvedNoHooks},
			waitUnitAgent{
				statusGetter: unitStatusGetter,
				status:       status.Error,
				info:         `hook failed: "leader-elected"`,
				data: map[string]interface{}{
					"hook": "leader-elected",
				},
			},
			resolveError{state.ResolvedNoHooks},
			waitUnitAgent{
				statusGetter: unitStatusGetter,
				status:       status.Error,
				info:         `hook failed: "config-changed"`,
				data: map[string]interface{}{
					"hook": "config-changed",
				},
			},
			resolveError{state.ResolvedNoHooks},
			waitUnitAgent{
				statusGetter: unitStatusGetter,
				status:       status.Error,
				info:         `hook failed: "start"`,
				data: map[string]interface{}{
					"hook": "start",
				},
			},
		),
	})
}

func (s *UniterSuite) TestUniterConfigChangedHook(c *gc.C) {
	s.runUniterTests(c, []uniterTest{
		ut(
			"config-changed hook fail and resolve",
			startupError{"config-changed"},
			verifyWaiting{},

			// Note: we'll run another config-changed as soon as we hit the
			// started state, so the broken hook would actually prevent us
			// from advancing at all if we didn't fix it.
			fixHook{"config-changed"},
			resolveError{state.ResolvedNoHooks},
			waitUnitAgent{
				status: status.Idle,
			},
			waitUnitAgent{
				statusGetter: unitStatusGetter,
				status:       status.Unknown,
			},
			// TODO(axw) confirm with fwereade that this is correct.
			// Previously we would see "start", "config-changed".
			// I don't think we should see another config-changed,
			// since config did not change since we resolved the
			// failed one above.
			waitHooks{"start"},
			// If we'd accidentally retried that hook, somehow, we would get
			// an extra config-changed as we entered started; see that we don't.
			waitHooks{},
			verifyRunning{},
		), ut(
			"config-changed hook fail and retry",
			startupError{"config-changed"},
			verifyWaiting{},

			resolveError{state.ResolvedRetryHooks},
			waitUnitAgent{
				statusGetter: unitStatusGetter,
				status:       status.Error,
				info:         `hook failed: "config-changed"`,
				data: map[string]interface{}{
					"hook": "config-changed",
				},
			},
			waitHooks{"fail-config-changed"},
			verifyWaiting{},

			fixHook{"config-changed"},
			resolveError{state.ResolvedRetryHooks},
			waitUnitAgent{
				status: status.Idle,
			},
			waitHooks{"config-changed", "start"},
			verifyRunning{},
		),
	})
}

func (s *UniterSuite) TestUniterHookSynchronisation(c *gc.C) {
	var lock hookLock
	s.runUniterTests(c, []uniterTest{
		ut(
			"verify config change hook not run while lock held",
			quickStart{},
			lock.acquire(),
			changeConfig{"blog-title": "Goodness Gracious Me"},
			waitHooks{},
			lock.release(),
			waitHooks{"config-changed"},
		), ut(
			"verify held lock by another unit is not broken",
			lock.acquire(),
			// Can't use quickstart as it has a built in waitHooks.
			createCharm{},
			serveCharm{},
			ensureStateWorker{},
			createApplicationAndUnit{},
			startUniter{},
			waitAddresses{},
			waitHooks{},
			lock.release(),
			waitUnitAgent{status: status.Idle},
			waitHooks{"install", "leader-elected", "config-changed", "start"},
		),
	})
}

func (s *UniterSuite) TestUniterDyingReaction(c *gc.C) {
	s.runUniterTests(c, []uniterTest{
		// Reaction to entity deaths.
		ut(
			"steady state unit dying",
			quickStart{},
			unitDying,
			waitHooks{"leader-settings-changed", "stop"},
			waitUniterDead{},
		), ut(
			"steady state unit dead",
			quickStart{},
			unitDead,
			waitUniterDead{},
			waitHooks{},
		), ut(
			"hook error unit dying",
			startupError{"start"},
			unitDying,
			verifyWaiting{},
			fixHook{"start"},
			resolveError{state.ResolvedRetryHooks},
			waitHooks{"start", "leader-settings-changed", "stop"},
			waitUniterDead{},
		), ut(
			"hook error unit dead",
			startupError{"start"},
			unitDead,
			waitUniterDead{},
			waitHooks{},
		),
	})
}

func (s *UniterSuite) TestUniterSteadyStateUpgrade(c *gc.C) {
	s.runUniterTests(c, []uniterTest{
		// Upgrade scenarios from steady state.
		ut(
			"steady state upgrade",
			quickStart{},
			createCharm{revision: 1},
			upgradeCharm{revision: 1},
			waitUnitAgent{
				status: status.Idle,
				charm:  1,
			},
			waitUnitAgent{
				statusGetter: unitStatusGetter,
				status:       status.Unknown,
				charm:        1,
			},
			waitHooks{"upgrade-charm", "config-changed"},
			verifyCharm{revision: 1},
			verifyRunning{},
		),
	})
}

func (s *UniterSuite) TestUniterSteadyStateUpgradeForce(c *gc.C) {
	s.runUniterTests(c, []uniterTest{
		ut(
			"steady state forced upgrade (identical behaviour)",
			quickStart{},
			createCharm{revision: 1},
			upgradeCharm{revision: 1, forced: true},
			waitUnitAgent{
				status: status.Idle,
				charm:  1,
			},
			waitUnitAgent{
				statusGetter: unitStatusGetter,
				status:       status.Unknown,
				charm:        1,
			},
			waitHooks{"upgrade-charm", "config-changed"},
			verifyCharm{revision: 1},
			verifyRunning{},
		),
	})
}

func (s *UniterSuite) TestUniterSteadyStateUpgradeResolve(c *gc.C) {
	s.runUniterTests(c, []uniterTest{
		ut(
			"steady state upgrade hook fail and resolve",
			quickStart{},
			createCharm{revision: 1, badHooks: []string{"upgrade-charm"}},
			upgradeCharm{revision: 1},
			waitUnitAgent{
				statusGetter: unitStatusGetter,
				status:       status.Error,
				info:         `hook failed: "upgrade-charm"`,
				data: map[string]interface{}{
					"hook": "upgrade-charm",
				},
				charm: 1,
			},
			waitHooks{"fail-upgrade-charm"},
			verifyCharm{revision: 1},
			verifyWaiting{},

			resolveError{state.ResolvedNoHooks},
			waitUnitAgent{
				status: status.Idle,
				charm:  1,
			},
			waitUnitAgent{
				statusGetter: unitStatusGetter,
				status:       status.Unknown,
				charm:        1,
			},
			waitHooks{"config-changed"},
			verifyRunning{},
		),
	})
}

func (s *UniterSuite) TestUniterSteadyStateUpgradeRetry(c *gc.C) {
	s.runUniterTests(c, []uniterTest{
		ut(
			"steady state upgrade hook fail and retry",
			quickStart{},
			createCharm{revision: 1, badHooks: []string{"upgrade-charm"}},
			upgradeCharm{revision: 1},
			waitUnitAgent{
				statusGetter: unitStatusGetter,
				status:       status.Error,
				info:         `hook failed: "upgrade-charm"`,
				data: map[string]interface{}{
					"hook": "upgrade-charm",
				},
				charm: 1,
			},
			waitHooks{"fail-upgrade-charm"},
			verifyCharm{revision: 1},
			verifyWaiting{},

			resolveError{state.ResolvedRetryHooks},
			waitUnitAgent{
				statusGetter: unitStatusGetter,
				status:       status.Error,
				info:         `hook failed: "upgrade-charm"`,
				data: map[string]interface{}{
					"hook": "upgrade-charm",
				},
				charm: 1,
			},
			waitHooks{"fail-upgrade-charm"},
			verifyWaiting{},

			fixHook{"upgrade-charm"},
			resolveError{state.ResolvedRetryHooks},
			waitUnitAgent{
				status: status.Idle,
				charm:  1,
			},
			waitHooks{"upgrade-charm", "config-changed"},
			verifyRunning{},
		),
	})
}

func (s *UniterSuite) TestUpdateResourceCausesUpgrade(c *gc.C) {
	// appendStorageMetadata customises the wordpress charm's metadata,
	// adding a "wp-content" filesystem store. We do it here rather
	// than in the charm itself to avoid modifying all of the other
	// scenarios.
	appendResource := func(c *gc.C, ctx *context, path string) {
		f, err := os.OpenFile(filepath.Join(path, "metadata.yaml"), os.O_RDWR|os.O_APPEND, 0644)
		c.Assert(err, jc.ErrorIsNil)
		defer func() {
			err := f.Close()
			c.Assert(err, jc.ErrorIsNil)
		}()
		_, err = io.WriteString(f, `
resources:
  data:
    Type: file
    filename: filename.tgz
    comment: One line that is useful when operators need to push it.`)
		c.Assert(err, jc.ErrorIsNil)
	}
	s.runUniterTests(c, []uniterTest{
		ut(
			"update resource causes upgrade",

			// These steps are just copied from quickstart with a customized
			// createCharm.
			createCharm{customize: appendResource},
			serveCharm{},
			createUniter{},
			waitUnitAgent{status: status.Idle},
			waitHooks(startupHooks(false)),
			verifyCharm{},

			pushResource{},
			waitHooks{"upgrade-charm", "config-changed"},
		),
	})
}

func (s *UniterSuite) TestUniterErrorStateUnforcedUpgrade(c *gc.C) {
	s.runUniterTests(c, []uniterTest{
		// Upgrade scenarios from error state.
		ut(
			"error state unforced upgrade (ignored until started state)",
			startupError{"start"},
			createCharm{revision: 1},
			upgradeCharm{revision: 1},
			waitUnitAgent{
				statusGetter: unitStatusGetter,
				status:       status.Error,
				info:         `hook failed: "start"`,
				data: map[string]interface{}{
					"hook": "start",
				},
			},
			waitHooks{},
			verifyCharm{},
			verifyWaiting{},

			resolveError{state.ResolvedNoHooks},
			waitUnitAgent{
				status: status.Idle,
				charm:  1,
			},
			waitUnitAgent{
				statusGetter: unitStatusGetter,
				status:       status.Maintenance,
				info:         "installing charm software",
				charm:        1,
			},
			waitHooks{"upgrade-charm", "config-changed"},
			verifyCharm{revision: 1},
			verifyRunning{},
		)})
}

func (s *UniterSuite) TestUniterErrorStateForcedUpgrade(c *gc.C) {
	s.runUniterTests(c, []uniterTest{
		ut(
			"error state forced upgrade",
			startupError{"start"},
			createCharm{revision: 1},
			upgradeCharm{revision: 1, forced: true},
			// It's not possible to tell directly from state when the upgrade is
			// complete, because the new unit charm URL is set at the upgrade
			// process's point of no return (before actually deploying, but after
			// the charm has been downloaded and verified). However, it's still
			// useful to wait until that point...
			waitUnitAgent{
				statusGetter: unitStatusGetter,
				status:       status.Error,
				info:         `hook failed: "start"`,
				data: map[string]interface{}{
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
			waitUnitAgent{
				status: status.Idle,
				charm:  1,
			},
			waitHooks{"config-changed"},
			verifyRunning{},
		),
	})
}

func (s *UniterSuite) TestUniterUpgradeConflicts(c *gc.C) {
	coretesting.SkipIfPPC64EL(c, "lp:1448308")
	//TODO(bogdanteleaga): Fix this on windows
	if runtime.GOOS == "windows" {
		c.Skip("bug 1403084: currently does not work on windows")
	}
	s.runUniterTests(c, []uniterTest{
		// Upgrade scenarios - handling conflicts.
		ut(
			"upgrade: resolving doesn't help until underlying problem is fixed",
			startUpgradeError{},
			resolveError{state.ResolvedNoHooks},
			verifyWaitingUpgradeError{revision: 1},
			fixUpgradeError{},
			resolveError{state.ResolvedNoHooks},
			waitHooks{"upgrade-charm", "config-changed"},
			waitUnitAgent{
				status: status.Idle,
				charm:  1,
			},
			waitUnitAgent{
				statusGetter: unitStatusGetter,
				status:       status.Unknown,
				charm:        1,
			},
			verifyCharm{revision: 1},
		), ut(
			`upgrade: forced upgrade does work without explicit resolution if underlying problem was fixed`,
			startUpgradeError{},
			resolveError{state.ResolvedNoHooks},
			verifyWaitingUpgradeError{revision: 1},
			fixUpgradeError{},
			createCharm{revision: 2},
			serveCharm{},
			upgradeCharm{revision: 2, forced: true},
			waitHooks{"upgrade-charm", "config-changed"},
			waitUnitAgent{
				status: status.Idle,
				charm:  2,
			},
			waitUnitAgent{
				statusGetter: unitStatusGetter,
				status:       status.Unknown,
				charm:        2,
			},
			verifyCharm{revision: 2},
		), ut(
			"upgrade conflict unit dying",
			startUpgradeError{},
			unitDying,
			verifyWaitingUpgradeError{revision: 1},
			fixUpgradeError{},
			resolveError{state.ResolvedNoHooks},
			waitHooks{"upgrade-charm", "config-changed", "leader-settings-changed", "stop"},
			waitUniterDead{},
		), ut(
			"upgrade conflict unit dead",
			startUpgradeError{},
			unitDead,
			waitUniterDead{},
			waitHooks{},
			fixUpgradeError{},
		),
	})
}

func (s *UniterSuite) TestUniterRelationsSimpleJoinedChangedDeparted(c *gc.C) {
	s.runUniterTest(c,
		quickStartRelation{},
		addRelationUnit{},
		waitHooks{
			"db-relation-joined mysql/1 db:0",
			"db-relation-changed mysql/1 db:0",
		},
		changeRelationUnit{"mysql/0"},
		waitHooks{"db-relation-changed mysql/0 db:0"},
		removeRelationUnit{"mysql/1"},
		waitHooks{"db-relation-departed mysql/1 db:0"},
		verifyRunning{},
	)
}

func (s *UniterSuite) TestUniterRelations(c *gc.C) {
	loggo.GetLogger("juju.apiserver").SetLogLevel(loggo.TRACE)
	waitDyingHooks := custom{func(c *gc.C, ctx *context) {
		// There is no ordering relationship between relation hooks and
		// leader-settings-changed hooks; and while we're dying we may
		// never get to leader-settings-changed before it's time to run
		// the stop (as we might not react to a config change in time).
		// It's actually clearer to just list the possible orders:
		possibles := [][]string{{
			"leader-settings-changed",
			"db-relation-departed mysql/0 db:0",
			"db-relation-broken db:0",
			"stop",
			"remove",
		}, {
			"db-relation-departed mysql/0 db:0",
			"leader-settings-changed",
			"db-relation-broken db:0",
			"stop",
			"remove",
		}, {
			"db-relation-departed mysql/0 db:0",
			"db-relation-broken db:0",
			"leader-settings-changed",
			"stop",
			"remove",
		}, {
			"db-relation-departed mysql/0 db:0",
			"db-relation-broken db:0",
			"stop",
			"remove",
		}}
		unchecked := ctx.hooksCompleted[len(ctx.hooks):]
		for _, possible := range possibles {
			if ok, _ := jc.DeepEqual(unchecked, possible); ok {
				return
			}
		}
		c.Fatalf("unexpected hooks: %v", unchecked)
	}}
	s.runUniterTests(c, []uniterTest{
		// Relations.
		ut(
			"relation becomes dying; unit is not last remaining member",
			quickStartRelation{},
			relationDying,
			waitHooks{
				"db-relation-departed mysql/0 db:0",
				"db-relation-broken db:0",
			},
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
			"unit becomes dying while in a relation",
			quickStartRelation{},
			unitDying,
			waitUniterDead{},
			waitDyingHooks,
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
			// uniter bug -- it should be the responsibility of `juju remove-unit
			// --force` to cause the unit to leave any relation scopes it may be
			// in -- but it's worth noting here all the same.
		), ut(
			"unknown local relation dir is removed",
			quickStartRelation{},
			stopUniter{},
			startUniter{},
			// We need some synchronisation point here to ensure that the uniter
			// has entered the correct place in the resolving loop. Now that we are
			// no longer always executing config-changed, we poke the config just so
			// we can get the event to give us the synchronisation point.
			changeConfig{"blog-title": "Goodness Gracious Me"},
			waitHooks{"config-changed"},
		),
	})
}

func (s *UniterSuite) TestUniterRelationErrors(c *gc.C) {
	s.runUniterTests(c, []uniterTest{
		ut(
			"hook error during join of a relation",
			startupRelationError{"db-relation-joined"},
			waitUnitAgent{
				statusGetter: unitStatusGetter,
				status:       status.Error,
				info:         `hook failed: "db-relation-joined"`,
				data: map[string]interface{}{
					"hook":        "db-relation-joined",
					"relation-id": 0,
					"remote-unit": "mysql/0",
				},
			},
		), ut(
			"hook error during change of a relation",
			startupRelationError{"db-relation-changed"},
			waitUnitAgent{
				statusGetter: unitStatusGetter,
				status:       status.Error,
				info:         `hook failed: "db-relation-changed"`,
				data: map[string]interface{}{
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
			waitUnitAgent{
				statusGetter: unitStatusGetter,
				status:       status.Error,
				info:         `hook failed: "db-relation-departed"`,
				data: map[string]interface{}{
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
			waitUnitAgent{
				statusGetter: unitStatusGetter,
				status:       status.Error,
				info:         `hook failed: "db-relation-broken"`,
				data: map[string]interface{}{
					"hook":        "db-relation-broken",
					"relation-id": 0,
				},
			},
		),
	})
}

func (s *UniterSuite) TestRunCommand(c *gc.C) {
	s.runUniterTests(c, []uniterTest{
		ut(
			"run commands",
			quickStart{},
			runCommands{"test"},
		),
	})
}

func (s *UniterSuite) TestRunAction(c *gc.C) {
	s.runUniterTests(c, []uniterTest{
		ut(
			"simple action",
			createCharm{},
			serveCharm{},
			ensureStateWorker{},
			createApplicationAndUnit{},
			startUniter{},
			waitAddresses{},
			waitUnitAgent{status: status.Idle},
			waitUnitAgent{
				statusGetter: unitStatusGetter,
				status:       status.Unknown,
			},
			waitHooks{"install", "leader-elected", "config-changed", "start"},
			verifyCharm{},
			addAction{"fakeaction", map[string]interface{}{"foo": "bar"}},
			waitActionInvocation{[]actionData{{
				actionName: "fakeaction",
				args:       []string{"foo=bar"},
			}}},
			waitUnitAgent{status: status.Idle},
			waitUnitAgent{
				statusGetter: unitStatusGetter,
				status:       status.Unknown,
			},
		), ut(
			"pending expectedActions get consumed",
			createCharm{},
			serveCharm{},
			ensureStateWorker{},
			createApplicationAndUnit{},
			addAction{"fakeaction", map[string]interface{}{"foo": "bar"}},
			addAction{"fakeaction", nil},
			addAction{"fakeaction", nil},
			startUniter{},
			waitAddresses{},
			waitUnitAgent{status: status.Idle},
			waitUnitAgent{
				statusGetter: unitStatusGetter,
				status:       status.Unknown,
			},
			waitHooks{"install", "leader-elected", "config-changed", "start"},
			verifyCharm{},
			waitActionInvocation{[]actionData{{
				actionName: "fakeaction",
				args:       []string{"foo=bar"},
			}, {
				actionName: "fakeaction",
			}, {
				actionName: "fakeaction",
			}}},
			waitUnitAgent{status: status.Idle},
			waitUnitAgent{
				statusGetter: unitStatusGetter,
				status:       status.Unknown,
			},
		), ut(
			"expectedActions may run from ModeHookError, but do not clear the error",
			startupError{
				badHook: "start",
			},
			addAction{"fakeaction", nil},
			waitUnitAgent{
				statusGetter: unitStatusGetter,
				status:       status.Error,
				info:         `hook failed: "start"`,
				data:         map[string]interface{}{"hook": "start"},
			},
			waitActionInvocation{[]actionData{{
				actionName: "fakeaction",
			}}},
			waitUnitAgent{
				statusGetter: unitStatusGetter,
				status:       status.Error,
				info:         `hook failed: "start"`,
				data:         map[string]interface{}{"hook": "start"},
			},
			verifyWaiting{},
			resolveError{state.ResolvedNoHooks},
			waitUnitAgent{status: status.Idle},
			waitUnitAgent{
				statusGetter: unitStatusGetter,
				status:       status.Maintenance,
				info:         "installing charm software",
			},
		),
	})
}

func (s *UniterSuite) TestUniterSubordinates(c *gc.C) {
	s.runUniterTests(c, []uniterTest{
		// Subordinates.
		ut(
			"unit becomes dying while subordinates exist",
			quickStart{},
			addSubordinateRelation{"juju-info"},
			waitSubordinateExists{"logging/0"},
			unitDying,
			waitSubordinateDying{},
			waitHooks{"leader-settings-changed", "stop"},
			verifyWaiting{},
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
	})
}

func (s *UniterSuite) TestSubordinateDying(c *gc.C) {
	// Create a test context for later use.
	ctx := &context{
		s:                      s,
		st:                     s.State,
		path:                   filepath.Join(s.dataDir, "agents", "unit-u-0"),
		dataDir:                s.dataDir,
		charms:                 make(map[string][]byte),
		leaseManager:           s.LeaseManager,
		updateStatusHookTicker: s.updateStatusHookTicker,
		charmDirGuard:          &mockCharmDirGuard{},
		runner:                 s.runner,
		deployer:               s.deployer,
	}

	addControllerMachine(c, ctx.st)

	// Create the subordinate application.
	dir := testcharms.Repo.ClonedDir(c.MkDir(), "logging")
	curl, err := corecharm.ParseURL("cs:quantal/logging")
	c.Assert(err, jc.ErrorIsNil)
	curl = curl.WithRevision(dir.Revision())
	step(c, ctx, addCharm{dir, curl})
	ctx.application = s.AddTestingApplication(c, "u", ctx.sch)

	// Create the principal application and add a relation.
	wps := s.AddTestingApplication(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	wpu, err := wps.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	eps, err := s.State.InferEndpoints("wordpress", "u")
	c.Assert(err, jc.ErrorIsNil)
	rel, err := s.State.AddRelation(eps...)
	c.Assert(err, jc.ErrorIsNil)
	assertAssignUnit(c, s.State, wpu)

	// Create the subordinate unit by entering scope as the principal.
	wpru, err := rel.Unit(wpu)
	c.Assert(err, jc.ErrorIsNil)
	err = wpru.EnterScope(nil)
	c.Assert(err, jc.ErrorIsNil)
	ctx.unit, err = s.State.Unit("u/0")
	c.Assert(err, jc.ErrorIsNil)
	ctx.apiLogin(c)

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

func (s *UniterSuite) TestLeadership(c *gc.C) {
	s.runUniterTests(c, []uniterTest{
		ut(
			"leader-elected triggers when elected",
			quickStart{minion: true},
			forceLeader{},
			waitHooks{"leader-elected"},
		), ut(
			"leader-settings-changed triggers when leader settings change",
			quickStart{minion: true},
			setLeaderSettings{"ping": "pong"},
			waitHooks{"leader-settings-changed"},
		), ut(
			"leader-settings-changed triggers when bounced",
			quickStart{minion: true},
			verifyRunning{minion: true},
		), ut(
			"leader-settings-changed triggers when deposed (while stopped)",
			quickStart{},
			stopUniter{},
			forceMinion{},
			verifyRunning{minion: true},
		),
	})
}

func (s *UniterSuite) TestLeadershipUnexpectedDepose(c *gc.C) {
	s.runUniterTests(c, []uniterTest{
		ut(
			// NOTE: this is a strange and ugly test, intended to detect what
			// *would* happen if the uniter suddenly failed to renew its lease;
			// it depends on an artificially shortened tracker refresh time to
			// run in a reasonable amount of time.
			"leader-settings-changed triggers when deposed (while running)",
			quickStart{},
			forceMinion{},
			waitHooks{"leader-settings-changed"},
		),
	})
}

func (s *UniterSuite) TestStorage(c *gc.C) {
	// appendStorageMetadata customises the wordpress charm's metadata,
	// adding a "wp-content" filesystem store. We do it here rather
	// than in the charm itself to avoid modifying all of the other
	// scenarios.
	appendStorageMetadata := func(c *gc.C, ctx *context, path string) {
		f, err := os.OpenFile(filepath.Join(path, "metadata.yaml"), os.O_RDWR|os.O_APPEND, 0644)
		c.Assert(err, jc.ErrorIsNil)
		defer func() {
			err := f.Close()
			c.Assert(err, jc.ErrorIsNil)
		}()
		_, err = io.WriteString(f, `
storage:
  wp-content:
    type: filesystem
    multiple:
      range: 0-
`[1:])
		c.Assert(err, jc.ErrorIsNil)
	}
	storageConstraints := map[string]state.StorageConstraints{
		"wp-content": {Count: 1},
	}
	s.runUniterTests(c, []uniterTest{
		ut(
			"test that storage-attached is called",
			createCharm{customize: appendStorageMetadata},
			serveCharm{},
			ensureStateWorker{},
			createApplicationAndUnit{storage: storageConstraints},
			provisionStorage{},
			startUniter{},
			waitAddresses{},
			waitHooks{"wp-content-storage-attached"},
			waitHooks(startupHooks(false)),
		), ut(
			"test that storage-detaching is called before stop",
			createCharm{customize: appendStorageMetadata},
			serveCharm{},
			ensureStateWorker{},
			createApplicationAndUnit{storage: storageConstraints},
			provisionStorage{},
			startUniter{},
			waitAddresses{},
			waitHooks{"wp-content-storage-attached"},
			waitHooks(startupHooks(false)),
			unitDying,
			waitHooks{"leader-settings-changed"},
			// "stop" hook is not called until storage is detached
			waitHooks{"wp-content-storage-detaching", "stop"},
			verifyStorageDetached{},
			waitUniterDead{},
		), ut(
			"test that storage-detaching is called only if previously attached",
			createCharm{customize: appendStorageMetadata},
			serveCharm{},
			ensureStateWorker{},
			createApplicationAndUnit{storage: storageConstraints},
			// provision and destroy the storage before the uniter starts,
			// to ensure it never sees the storage as attached
			provisionStorage{},
			destroyStorageAttachment{},
			startUniter{},
			waitHooks(startupHooks(false)),
			unitDying,
			// storage-detaching is not called because it was never attached
			waitHooks{"leader-settings-changed", "stop"},
			verifyStorageDetached{},
			waitUniterDead{},
		), ut(
			"test that delay-provisioned storage does not block forever",
			createCharm{customize: appendStorageMetadata},
			serveCharm{},
			ensureStateWorker{},
			createApplicationAndUnit{storage: storageConstraints},
			startUniter{},
			// no hooks should be run, as storage isn't provisioned
			waitHooks{},
			provisionStorage{},
			waitHooks{"wp-content-storage-attached"},
			waitHooks(startupHooks(false)),
		), ut(
			"test that unprovisioned storage does not block unit termination",
			createCharm{customize: appendStorageMetadata},
			serveCharm{},
			ensureStateWorker{},
			createApplicationAndUnit{storage: storageConstraints},
			unitDying,
			startUniter{},
			// no hooks should be run, and unit agent should terminate
			waitHooks{},
			waitUniterDead{},
		),
		// TODO(axw) test that storage-attached is run for new
		// storage attachments before upgrade-charm is run. This
		// requires additions to state to add storage when a charm
		// is upgraded.
	})
}

var mockExecutorErr = errors.New("some error occurred")

type mockExecutor struct {
	operation.Executor
}

func (m *mockExecutor) Run(op operation.Operation, rs <-chan remotestate.Snapshot) error {
	// want to allow charm unpacking to occur
	if strings.HasPrefix(op.String(), "install") {
		return m.Executor.Run(op, rs)
	}
	// but hooks should error
	return mockExecutorErr
}

func (s *UniterSuite) TestOperationErrorReported(c *gc.C) {
	s.runUniterTests(c, []uniterTest{
		ut(
			"error running operations are reported",
			createCharm{},
			serveCharm{},
			createUniter{executorFunc: executorFunc(c)},
			waitUnitAgent{
				status: status.Failed,
				info:   "resolver loop error",
			},
			expectError{".*some error occurred.*"},
		),
	})
}

func (s *UniterSuite) TestTranslateResolverError(c *gc.C) {
	translateResolverErr := func(in error) error {
		c.Check(errors.Cause(in), gc.Equals, mockExecutorErr)
		return errors.New("some other error")
	}
	s.runUniterTests(c, []uniterTest{
		ut(
			"resolver errors are translated",
			createCharm{},
			serveCharm{},
			createUniter{
				executorFunc:         executorFunc(c),
				translateResolverErr: translateResolverErr,
			},
			waitUnitAgent{
				status: status.Failed,
				info:   "resolver loop error",
			},
			expectError{".*some other error.*"},
		),
	})
}

func executorFunc(c *gc.C) uniter.NewOperationExecutorFunc {
	return func(cfg operation.ExecutorConfig) (operation.Executor, error) {
		e, err := operation.NewExecutor(cfg)
		c.Assert(err, jc.ErrorIsNil)
		return &mockExecutor{e}, nil
	}
}
