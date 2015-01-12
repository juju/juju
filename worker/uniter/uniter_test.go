// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter_test

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"

	jc "github.com/juju/testing/checkers"
	ft "github.com/juju/testing/filetesting"
	gc "gopkg.in/check.v1"
	corecharm "gopkg.in/juju/charm.v4"

	"github.com/juju/juju/agent/tools"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state"
	"github.com/juju/juju/testcharms"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker/uniter"
)

type UniterSuite struct {
	coretesting.GitSuite
	testing.JujuConnSuite
	dataDir  string
	oldLcAll string
	unitDir  string

	ticker *uniter.ManualTicker
}

var _ = gc.Suite(&UniterSuite{})

func (s *UniterSuite) SetUpSuite(c *gc.C) {
	s.GitSuite.SetUpSuite(c)
	s.JujuConnSuite.SetUpSuite(c)
	s.dataDir = c.MkDir()
	toolsDir := tools.ToolsDir(s.dataDir, "unit-u-0")
	err := os.MkdirAll(toolsDir, 0755)
	c.Assert(err, jc.ErrorIsNil)
	// TODO(fwereade) GAAAAAAAAAAAAAAAAAH this is LUDICROUS.
	cmd := exec.Command("go", "build", "github.com/juju/juju/cmd/jujud")
	cmd.Dir = toolsDir
	out, err := cmd.CombinedOutput()
	c.Logf(string(out))
	c.Assert(err, jc.ErrorIsNil)
	s.oldLcAll = os.Getenv("LC_ALL")
	os.Setenv("LC_ALL", "en_US")
	s.unitDir = filepath.Join(s.dataDir, "agents", "unit-u-0")
}

func (s *UniterSuite) TearDownSuite(c *gc.C) {
	os.Setenv("LC_ALL", s.oldLcAll)
	s.JujuConnSuite.TearDownSuite(c)
	s.GitSuite.TearDownSuite(c)
}

func (s *UniterSuite) SetUpTest(c *gc.C) {
	s.GitSuite.SetUpTest(c)
	s.JujuConnSuite.SetUpTest(c)
	s.ticker = uniter.NewManualTicker()
	s.PatchValue(uniter.ActiveMetricsTimer, s.ticker.ReturnTimer)
}

func (s *UniterSuite) TearDownTest(c *gc.C) {
	s.ResetContext(c)
	s.JujuConnSuite.TearDownTest(c)
	s.GitSuite.TearDownTest(c)
}

func (s *UniterSuite) Reset(c *gc.C) {
	s.JujuConnSuite.Reset(c)
	s.ResetContext(c)
}

func (s *UniterSuite) ResetContext(c *gc.C) {
	err := os.RemoveAll(s.unitDir)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *UniterSuite) runUniterTests(c *gc.C, uniterTests []uniterTest) {
	for i, t := range uniterTests {
		c.Logf("\ntest %d: %s\n", i, t.summary)
		func() {
			defer s.Reset(c)
			env, err := s.State.Environment()
			c.Assert(err, jc.ErrorIsNil)
			ctx := &context{
				s:       s,
				st:      s.State,
				uuid:    env.UUID(),
				path:    s.unitDir,
				dataDir: s.dataDir,
				charms:  make(map[string][]byte),
				ticker:  s.ticker,
			}
			ctx.run(c, t.steps)
		}()
	}
}

func (s *UniterSuite) TestUniterStartup(c *gc.C) {
	s.runUniterTests(c, []uniterTest{
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
	})
}

func (s *UniterSuite) TestUniterBootstrap(c *gc.C) {
	s.runUniterTests(c, []uniterTest{
		// Check error conditions during unit bootstrap phase.
		ut(
			"insane deployment",
			createCharm{},
			serveCharm{},
			writeFile{"charm", 0644},
			createUniter{},
			waitUniterDead{`ModeInstalling cs:quantal/wordpress-0: executing operation "install cs:quantal/wordpress-0": open .*: not a directory`},
		), ut(
			"charm cannot be downloaded",
			createCharm{},
			// don't serve charm
			createUniter{},
			waitUniterDead{`ModeInstalling cs:quantal/wordpress-0: preparing operation "install cs:quantal/wordpress-0": failed to download charm .* 404 Not Found`},
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
			waitUnit{
				status: params.StatusActive,
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
				data: map[string]interface{}{
					"hook": "install",
				},
			},
			waitHooks{"fail-install"},
			fixHook{"install"},
			verifyWaiting{},

			resolveError{state.ResolvedRetryHooks},
			waitUnit{
				status: params.StatusActive,
			},
			waitHooks{"install", "config-changed", "start"},
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
			waitUnit{
				status: params.StatusActive,
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
				data: map[string]interface{}{
					"hook": "start",
				},
			},
			waitHooks{"fail-start"},
			verifyWaiting{},

			fixHook{"start"},
			resolveError{state.ResolvedRetryHooks},
			waitUnit{
				status: params.StatusActive,
			},
			waitHooks{"start", "config-changed"},
			verifyRunning{},
		),
	})
}

func (s *UniterSuite) TestUniterMultipleErrors(c *gc.C) {
	s.runUniterTests(c, []uniterTest{
		ut(
			"resolved is cleared before moving on to next hook",
			createCharm{badHooks: []string{"install", "config-changed", "start"}},
			serveCharm{},
			createUniter{},
			waitUnit{
				status: params.StatusError,
				info:   `hook failed: "install"`,
				data: map[string]interface{}{
					"hook": "install",
				},
			},
			resolveError{state.ResolvedNoHooks},
			waitUnit{
				status: params.StatusError,
				info:   `hook failed: "config-changed"`,
				data: map[string]interface{}{
					"hook": "config-changed",
				},
			},
			resolveError{state.ResolvedNoHooks},
			waitUnit{
				status: params.StatusError,
				info:   `hook failed: "start"`,
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
			waitUnit{
				status: params.StatusActive,
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
				data: map[string]interface{}{
					"hook": "config-changed",
				},
			},
			waitHooks{"fail-config-changed"},
			verifyWaiting{},

			fixHook{"config-changed"},
			resolveError{state.ResolvedRetryHooks},
			waitUnit{
				status: params.StatusActive,
			},
			waitHooks{"config-changed", "start"},
			verifyRunning{},
		), ut(
			"steady state config change with config-get verification",
			createCharm{
				customize: func(c *gc.C, ctx *context, path string) {
					appendHook(c, path, "config-changed", "config-get --format yaml --output config.out")
				},
			},
			serveCharm{},
			createUniter{},
			waitUnit{
				status: params.StatusActive,
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
		),
	})
}

func (s *UniterSuite) TestUniterHookSynchronisation(c *gc.C) {
	s.runUniterTests(c, []uniterTest{
		ut(
			"verify config change hook not run while lock held",
			quickStart{},
			acquireHookSyncLock{},
			changeConfig{"blog-title": "Goodness Gracious Me"},
			waitHooks{},
			releaseHookSyncLock,
			waitHooks{"config-changed"},
		), ut(
			"verify held lock by this unit is broken",
			acquireHookSyncLock{"u/0:fake"},
			quickStart{},
			verifyHookSyncLockUnlocked,
		), ut(
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
			waitUnit{status: params.StatusActive},
			waitHooks{"install", "config-changed", "start"},
		),
	})
}

func (s *UniterSuite) TestUniterDyingReaction(c *gc.C) {
	s.runUniterTests(c, []uniterTest{
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
			waitUnit{
				status: params.StatusActive,
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
				status: params.StatusActive,
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
				data: map[string]interface{}{
					"hook": "upgrade-charm",
				},
				charm: 1,
			},
			waitHooks{"fail-upgrade-charm"},
			verifyCharm{revision: 1},
			verifyWaiting{},

			resolveError{state.ResolvedNoHooks},
			waitUnit{
				status: params.StatusActive,
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
				data: map[string]interface{}{
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
				data: map[string]interface{}{
					"hook": "upgrade-charm",
				},
				charm: 1,
			},
			waitHooks{"fail-upgrade-charm"},
			verifyWaiting{},

			fixHook{"upgrade-charm"},
			resolveError{state.ResolvedRetryHooks},
			waitUnit{
				status: params.StatusActive,
				charm:  1,
			},
			waitHooks{"upgrade-charm", "config-changed"},
			verifyRunning{},
		), ut(
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
	})
}

func (s *UniterSuite) TestUniterUpgradeOverwrite(c *gc.C) {
	makeTest := func(description string, content, extraChecks ft.Entries) uniterTest {
		return ut(description,
			createCharm{
				// This is the base charm which all upgrade tests start out running.
				customize: func(c *gc.C, ctx *context, path string) {
					ft.Entries{
						ft.Dir{"dir", 0755},
						ft.File{"file", "blah", 0644},
						ft.Symlink{"symlink", "file"},
					}.Create(c, path)
					// Note that it creates "dir/user-file" at runtime, which may be
					// preserved or removed depending on the test.
					script := "echo content > dir/user-file && chmod 755 dir/user-file"
					appendHook(c, path, "start", script)
				},
			},
			serveCharm{},
			createUniter{},
			waitUnit{
				status: params.StatusActive,
			},
			waitHooks{"install", "config-changed", "start"},

			createCharm{
				revision: 1,
				customize: func(c *gc.C, _ *context, path string) {
					content.Create(c, path)
				},
			},
			serveCharm{},
			upgradeCharm{revision: 1},
			waitUnit{
				status: params.StatusActive,
				charm:  1,
			},
			waitHooks{"upgrade-charm", "config-changed"},
			verifyCharm{revision: 1},
			custom{func(c *gc.C, ctx *context) {
				path := filepath.Join(ctx.path, "charm")
				content.Check(c, path)
				extraChecks.Check(c, path)
			}},
			verifyRunning{},
		)
	}

	s.runUniterTests(c, []uniterTest{
		makeTest(
			"files overwite files, dirs, symlinks",
			ft.Entries{
				ft.File{"file", "new", 0755},
				ft.File{"dir", "new", 0755},
				ft.File{"symlink", "new", 0755},
			},
			ft.Entries{
				ft.Removed{"dir/user-file"},
			},
		), makeTest(
			"symlinks overwite files, dirs, symlinks",
			ft.Entries{
				ft.Symlink{"file", "new"},
				ft.Symlink{"dir", "new"},
				ft.Symlink{"symlink", "new"},
			},
			ft.Entries{
				ft.Removed{"dir/user-file"},
			},
		), makeTest(
			"dirs overwite files, symlinks; merge dirs",
			ft.Entries{
				ft.Dir{"file", 0755},
				ft.Dir{"dir", 0755},
				ft.File{"dir/charm-file", "charm-content", 0644},
				ft.Dir{"symlink", 0755},
			},
			ft.Entries{
				ft.File{"dir/user-file", "content\n", 0755},
			},
		),
	})
}

func (s *UniterSuite) TestUniterErrorStateUpgrade(c *gc.C) {
	s.runUniterTests(c, []uniterTest{
		// Upgrade scenarios from error state.
		ut(
			"error state unforced upgrade (ignored until started state)",
			startupError{"start"},
			createCharm{revision: 1},
			upgradeCharm{revision: 1},
			waitUnit{
				status: params.StatusError,
				info:   `hook failed: "start"`,
				data: map[string]interface{}{
					"hook": "start",
				},
			},
			waitHooks{},
			verifyCharm{},
			verifyWaiting{},

			resolveError{state.ResolvedNoHooks},
			waitUnit{
				status: params.StatusActive,
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
			waitUnit{
				status: params.StatusActive,
				charm:  1,
			},
			waitHooks{"config-changed"},
			verifyRunning{},
		),
	})
}

func (s *UniterSuite) TestUniterDeployerConversion(c *gc.C) {
	deployerConversionTests := []uniterTest{
		ut(
			"install normally, check not using git",
			quickStart{},
			verifyCharm{
				checkFiles: ft.Entries{ft.Removed{".git"}},
			},
		), ut(
			"install with git, restart in steady state",
			prepareGitUniter{[]stepper{
				quickStart{},
				verifyGitCharm{},
				stopUniter{},
			}},
			startUniter{},
			waitHooks{"config-changed"},

			// At this point, the deployer has been converted, but the
			// charm directory itself hasn't; the *next* deployment will
			// actually hit the charm directory and strip out the git
			// stuff.
			createCharm{revision: 1},
			upgradeCharm{revision: 1},
			waitHooks{"upgrade-charm", "config-changed"},
			waitUnit{
				status: params.StatusActive,
				charm:  1,
			},
			verifyCharm{
				revision:   1,
				checkFiles: ft.Entries{ft.Removed{".git"}},
			},
			verifyRunning{},
		), ut(
			"install with git, get conflicted, mark resolved",
			prepareGitUniter{[]stepper{
				startGitUpgradeError{},
				stopUniter{},
			}},
			startUniter{},

			resolveError{state.ResolvedNoHooks},
			waitHooks{"upgrade-charm", "config-changed"},
			waitUnit{
				status: params.StatusActive,
				charm:  1,
			},
			verifyCharm{revision: 1},
			verifyRunning{},

			// Due to the uncertainties around marking upgrade conflicts resolved,
			// the charm directory again remains unconverted, although the deployer
			// should have been fixed. Again, we check this by running another
			// upgrade and verifying the .git dir is then removed.
			createCharm{revision: 2},
			upgradeCharm{revision: 2},
			waitHooks{"upgrade-charm", "config-changed"},
			waitUnit{
				status: params.StatusActive,
				charm:  2,
			},
			verifyCharm{
				revision:   2,
				checkFiles: ft.Entries{ft.Removed{".git"}},
			},
			verifyRunning{},
		), ut(
			"install with git, get conflicted, force an upgrade",
			prepareGitUniter{[]stepper{
				startGitUpgradeError{},
				stopUniter{},
			}},
			startUniter{},

			createCharm{
				revision: 2,
				customize: func(c *gc.C, ctx *context, path string) {
					ft.File{"data", "OVERWRITE!", 0644}.Create(c, path)
				},
			},
			serveCharm{},
			upgradeCharm{revision: 2, forced: true},
			waitHooks{"upgrade-charm", "config-changed"},
			waitUnit{
				status: params.StatusActive,
				charm:  2,
			},

			// A forced upgrade allows us to swap out the git deployer *and*
			// the .git dir inside the charm immediately; check we did so.
			verifyCharm{
				revision: 2,
				checkFiles: ft.Entries{
					ft.Removed{".git"},
					ft.File{"data", "OVERWRITE!", 0644},
				},
			},
			verifyRunning{},
		),
	}
	s.runUniterTests(c, deployerConversionTests)
}

func (s *UniterSuite) TestUniterUpgradeConflicts(c *gc.C) {
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
			waitUnit{
				status: params.StatusActive,
				charm:  1,
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
			waitUnit{
				status: params.StatusActive,
				charm:  2,
			},
			verifyCharm{revision: 2},
		), ut(
			"upgrade conflict service dying",
			startUpgradeError{},
			serviceDying,
			verifyWaitingUpgradeError{revision: 1},
			fixUpgradeError{},
			resolveError{state.ResolvedNoHooks},
			waitHooks{"upgrade-charm", "config-changed", "stop"},
			waitUniterDead{},
		), ut(
			"upgrade conflict unit dying",
			startUpgradeError{},
			unitDying,
			verifyWaitingUpgradeError{revision: 1},
			fixUpgradeError{},
			resolveError{state.ResolvedNoHooks},
			waitHooks{"upgrade-charm", "config-changed", "stop"},
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

func (s *UniterSuite) TestUniterUpgradeGitConflicts(c *gc.C) {
	// These tests are copies of the old git-deployer-related tests, to test that
	// the uniter with the manifest-deployer work patched out still works how it
	// used to; thus demonstrating that the *other* tests that verify manifest
	// deployer behaviour in the presence of an old git deployer are working against
	// an accurate representation of the base state.
	// The only actual behaviour change is that we no longer commit changes after
	// each hook execution; this is reflected by checking that it's dirty in a couple
	// of places where we once checked it was not.

	s.runUniterTests(c, []uniterTest{
		// Upgrade scenarios - handling conflicts.
		ugt(
			"upgrade: conflicting files",
			startGitUpgradeError{},

			// NOTE: this is just dumbly committing the conflicts, but AFAICT this
			// is the only reasonable solution; if the user tells us it's resolved
			// we have to take their word for it.
			resolveError{state.ResolvedNoHooks},
			waitHooks{"upgrade-charm", "config-changed"},
			waitUnit{
				status: params.StatusActive,
				charm:  1,
			},
			verifyGitCharm{revision: 1},
		), ugt(
			`upgrade: conflicting directories`,
			createCharm{
				customize: func(c *gc.C, ctx *context, path string) {
					err := os.Mkdir(filepath.Join(path, "data"), 0755)
					c.Assert(err, jc.ErrorIsNil)
					appendHook(c, path, "start", "echo DATA > data/newfile")
				},
			},
			serveCharm{},
			createUniter{},
			waitUnit{
				status: params.StatusActive,
			},
			waitHooks{"install", "config-changed", "start"},
			verifyGitCharm{dirty: true},

			createCharm{
				revision: 1,
				customize: func(c *gc.C, ctx *context, path string) {
					data := filepath.Join(path, "data")
					err := ioutil.WriteFile(data, []byte("<nelson>ha ha</nelson>"), 0644)
					c.Assert(err, jc.ErrorIsNil)
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
				status: params.StatusActive,
				charm:  1,
			},
			verifyGitCharm{revision: 1},
		), ugt(
			"upgrade conflict resolved with forced upgrade",
			startGitUpgradeError{},
			createCharm{
				revision: 2,
				customize: func(c *gc.C, ctx *context, path string) {
					otherdata := filepath.Join(path, "otherdata")
					err := ioutil.WriteFile(otherdata, []byte("blah"), 0644)
					c.Assert(err, jc.ErrorIsNil)
				},
			},
			serveCharm{},
			upgradeCharm{revision: 2, forced: true},
			waitUnit{
				status: params.StatusActive,
				charm:  2,
			},
			waitHooks{"upgrade-charm", "config-changed"},
			verifyGitCharm{revision: 2},
			custom{func(c *gc.C, ctx *context) {
				// otherdata should exist (in v2)
				otherdata, err := ioutil.ReadFile(filepath.Join(ctx.path, "charm", "otherdata"))
				c.Assert(err, jc.ErrorIsNil)
				c.Assert(string(otherdata), gc.Equals, "blah")

				// ignore should not (only in v1)
				_, err = os.Stat(filepath.Join(ctx.path, "charm", "ignore"))
				c.Assert(err, jc.Satisfies, os.IsNotExist)

				// data should contain what was written in the start hook
				data, err := ioutil.ReadFile(filepath.Join(ctx.path, "charm", "data"))
				c.Assert(err, jc.ErrorIsNil)
				c.Assert(string(data), gc.Equals, "STARTDATA\n")
			}},
		), ugt(
			"upgrade conflict service dying",
			startGitUpgradeError{},
			serviceDying,
			verifyWaiting{},
			resolveError{state.ResolvedNoHooks},
			waitHooks{"upgrade-charm", "config-changed", "stop"},
			waitUniterDead{},
		), ugt(
			"upgrade conflict unit dying",
			startGitUpgradeError{},
			unitDying,
			verifyWaiting{},
			resolveError{state.ResolvedNoHooks},
			waitHooks{"upgrade-charm", "config-changed", "stop"},
			waitUniterDead{},
		), ugt(
			"upgrade conflict unit dead",
			startGitUpgradeError{},
			unitDead,
			waitUniterDead{},
			waitHooks{},
		),
	})
}

func (s *UniterSuite) TestRunCommand(c *gc.C) {
	testDir := c.MkDir()
	testFile := func(name string) string {
		return filepath.Join(testDir, name)
	}
	echoUnitNameToFile := func(name string) string {
		path := filepath.Join(testDir, name)
		template := "echo juju run ${JUJU_UNIT_NAME} > %s.tmp; mv %s.tmp %s"
		return fmt.Sprintf(template, path, path, path)
	}
	adminTag := s.AdminUserTag(c)

	s.runUniterTests(c, []uniterTest{
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
				adminTag.String() + "\nprivate.address.example.com\npublic.address.example.com\n",
			},
		), ut(
			"run commands: jujuc environment",
			quickStartRelation{},
			relationRunCommands{
				fmt.Sprintf("echo $JUJU_RELATION_ID > %s", testFile("jujuc-env.output")),
				fmt.Sprintf("echo $JUJU_REMOTE_UNIT >> %s", testFile("jujuc-env.output")),
			},
			verifyFile{
				testFile("jujuc-env.output"),
				"db:0\nmysql/0\n",
			},
		), ut(
			"run commands: proxy settings set",
			quickStartRelation{},
			setProxySettings{Http: "http", Https: "https", Ftp: "ftp", NoProxy: "localhost"},
			runCommands{
				fmt.Sprintf("echo $http_proxy > %s", testFile("proxy.output")),
				fmt.Sprintf("echo $HTTP_PROXY >> %s", testFile("proxy.output")),
				fmt.Sprintf("echo $https_proxy >> %s", testFile("proxy.output")),
				fmt.Sprintf("echo $HTTPS_PROXY >> %s", testFile("proxy.output")),
				fmt.Sprintf("echo $ftp_proxy >> %s", testFile("proxy.output")),
				fmt.Sprintf("echo $FTP_PROXY >> %s", testFile("proxy.output")),
				fmt.Sprintf("echo $no_proxy >> %s", testFile("proxy.output")),
				fmt.Sprintf("echo $NO_PROXY >> %s", testFile("proxy.output")),
			},
			verifyFile{
				testFile("proxy.output"),
				"http\nhttp\nhttps\nhttps\nftp\nftp\nlocalhost\nlocalhost\n",
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
	})
}

func (s *UniterSuite) TestUniterRelations(c *gc.C) {
	s.runUniterTests(c, []uniterTest{
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
			// uniter bug -- it should be the responsibility of `juju remove-unit
			// --force` to cause the unit to leave any relation scopes it may be
			// in -- but it's worth noting here all the same.
		), ut(
			"unknown local relation dir is removed",
			quickStartRelation{},
			stopUniter{},
			custom{func(c *gc.C, ctx *context) {
				ft.Dir{"state/relations/90210", 0755}.Create(c, ctx.path)
			}},
			startUniter{},
			waitHooks{"config-changed"},
			custom{func(c *gc.C, ctx *context) {
				ft.Removed{"state/relations/90210"}.Check(c, ctx.path)
			}},
		), ut(
			"all relations are available to config-changed on bounce, even if state dir is missing",
			createCharm{
				customize: func(c *gc.C, ctx *context, path string) {
					script := "relation-ids db > relations.out && chmod 644 relations.out"
					appendHook(c, path, "config-changed", script)
				},
			},
			serveCharm{},
			createUniter{},
			waitUnit{
				status: params.StatusActive,
			},
			waitHooks{"install", "config-changed", "start"},
			addRelation{waitJoin: true},
			stopUniter{},
			custom{func(c *gc.C, ctx *context) {
				// Check the state dir was created, and remove it.
				path := fmt.Sprintf("state/relations/%d", ctx.relation.Id())
				ft.Dir{path, 0755}.Check(c, ctx.path)
				ft.Removed{path}.Create(c, ctx.path)

				// Check that config-changed didn't record any relations, because
				// they shouldn't been available until after the start hook.
				ft.File{"charm/relations.out", "", 0644}.Check(c, ctx.path)
			}},
			startUniter{},
			waitHooks{"config-changed"},
			custom{func(c *gc.C, ctx *context) {
				// Check the state dir was recreated.
				path := fmt.Sprintf("state/relations/%d", ctx.relation.Id())
				ft.Dir{path, 0755}.Check(c, ctx.path)

				// Check that config-changed did record the joined relations.
				data := fmt.Sprintf("db:%d\n", ctx.relation.Id())
				ft.File{"charm/relations.out", data, 0644}.Check(c, ctx.path)
			}},
		),
	})
}

func (s *UniterSuite) TestUniterRelationErrors(c *gc.C) {
	s.runUniterTests(c, []uniterTest{
		ut(
			"hook error during join of a relation",
			startupRelationError{"db-relation-joined"},
			waitUnit{
				status: params.StatusError,
				info:   `hook failed: "db-relation-joined"`,
				data: map[string]interface{}{
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
			waitUnit{
				status: params.StatusError,
				info:   `hook failed: "db-relation-departed"`,
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
			waitUnit{
				status: params.StatusError,
				info:   `hook failed: "db-relation-broken"`,
				data: map[string]interface{}{
					"hook":        "db-relation-broken",
					"relation-id": 0,
				},
			},
		),
	})
}

func (s *UniterSuite) TestUniterMeterStatusChanged(c *gc.C) {
	s.runUniterTests(c, []uniterTest{
		ut(
			"meter status event triggered by unit meter status change",
			quickStart{},
			changeMeterStatus{"AMBER", "Investigate charm."},
			waitHooks{"meter-status-changed"},
		),
	})
}

func (s *UniterSuite) TestUniterCollectMetrics(c *gc.C) {
	s.runUniterTests(c, []uniterTest{
		ut(
			"collect-metrics event triggered by manual timer",
			createCharm{
				customize: func(c *gc.C, ctx *context, path string) {
					ctx.writeMetricsYaml(c, path)
				},
			},
			serveCharm{},
			createUniter{},
			waitUnit{status: params.StatusActive},
			waitHooks{"install", "config-changed", "start"},
			verifyCharm{},
			metricsTick{},
			waitHooks{"collect-metrics"},
		),
		ut(
			"collect-metrics resumed after hook error",
			startupErrorWithCustomCharm{
				badHook: "config-changed",
				customize: func(c *gc.C, ctx *context, path string) {
					ctx.writeMetricsYaml(c, path)
				},
			},
			metricsTick{},
			fixHook{"config-changed"},
			resolveError{state.ResolvedRetryHooks},
			waitUnit{
				status: params.StatusActive,
			},
			waitHooks{"config-changed", "start", "collect-metrics"},
			verifyRunning{},
		),
		ut(
			"collect-metrics state maintained during uniter restart",
			startupErrorWithCustomCharm{
				badHook: "config-changed",
				customize: func(c *gc.C, ctx *context, path string) {
					ctx.writeMetricsYaml(c, path)
				},
			},
			metricsTick{},
			fixHook{"config-changed"},
			stopUniter{},
			startUniter{},
			resolveError{state.ResolvedRetryHooks},
			waitUnit{
				status: params.StatusActive,
			},
			waitHooks{"config-changed", "start", "collect-metrics"},
			verifyRunning{},
		),
		ut(
			"collect-metrics event not triggered for non-metered charm",
			quickStart{},
			metricsTick{},
			waitHooks{},
		),
	})
}

func (s *UniterSuite) TestActionEvents(c *gc.C) {
	s.runUniterTests(c, []uniterTest{
		ut(
			"simple action event: defined in actions.yaml, no args",
			createCharm{
				customize: func(c *gc.C, ctx *context, path string) {
					ctx.writeAction(c, path, "action-log")
					ctx.writeActionsYaml(c, path, "action-log")
				},
			},
			serveCharm{},
			ensureStateWorker{},
			createServiceAndUnit{},
			startUniter{},
			waitAddresses{},
			waitUnit{status: params.StatusActive},
			waitHooks{"install", "config-changed", "start"},
			verifyCharm{},
			addAction{"action-log", nil},
			waitActionResults{[]actionResult{{
				name:    "action-log",
				results: map[string]interface{}{},
				status:  params.ActionCompleted,
			}}},
			waitUnit{status: params.StatusActive},
		), ut(
			"action-fail causes the action to fail with a message",
			createCharm{
				customize: func(c *gc.C, ctx *context, path string) {
					ctx.writeAction(c, path, "action-log-fail")
					ctx.writeActionsYaml(c, path, "action-log-fail")
				},
			},
			serveCharm{},
			ensureStateWorker{},
			createServiceAndUnit{},
			startUniter{},
			waitAddresses{},
			waitUnit{status: params.StatusActive},
			waitHooks{"install", "config-changed", "start"},
			verifyCharm{},
			addAction{"action-log-fail", nil},
			waitActionResults{[]actionResult{{
				name: "action-log-fail",
				results: map[string]interface{}{
					"foo": "still works",
				},
				message: "I'm afraid I can't let you do that, Dave.",
				status:  params.ActionFailed,
			}}},
			waitUnit{status: params.StatusActive},
		), ut(
			"action-fail with the wrong arguments fails but is not an error",
			createCharm{
				customize: func(c *gc.C, ctx *context, path string) {
					ctx.writeAction(c, path, "action-log-fail-error")
					ctx.writeActionsYaml(c, path, "action-log-fail-error")
				},
			},
			serveCharm{},
			ensureStateWorker{},
			createServiceAndUnit{},
			startUniter{},
			waitAddresses{},
			waitUnit{status: params.StatusActive},
			waitHooks{"install", "config-changed", "start"},
			verifyCharm{},
			addAction{"action-log-fail-error", nil},
			waitActionResults{[]actionResult{{
				name: "action-log-fail-error",
				results: map[string]interface{}{
					"foo": "still works",
				},
				message: "A real message",
				status:  params.ActionFailed,
			}}},
			waitUnit{status: params.StatusActive},
		), ut(
			"actions with correct params passed are not an error",
			createCharm{
				customize: func(c *gc.C, ctx *context, path string) {
					ctx.writeAction(c, path, "snapshot")
					ctx.writeActionsYaml(c, path, "snapshot")
				},
			},
			serveCharm{},
			ensureStateWorker{},
			createServiceAndUnit{},
			startUniter{},
			waitAddresses{},
			waitUnit{status: params.StatusActive},
			waitHooks{"install", "config-changed", "start"},
			verifyCharm{},
			addAction{
				name:   "snapshot",
				params: map[string]interface{}{"outfile": "foo.bar"},
			},
			waitActionResults{[]actionResult{{
				name: "snapshot",
				results: map[string]interface{}{
					"outfile": map[string]interface{}{
						"name": "snapshot-01.tar",
						"size": map[string]interface{}{
							"magnitude": "10.3",
							"units":     "GB",
						},
					},
					"completion": "yes",
				},
				status: params.ActionCompleted,
			}}},
			waitUnit{status: params.StatusActive},
		), ut(
			"actions with incorrect params passed are not an error but fail",
			createCharm{
				customize: func(c *gc.C, ctx *context, path string) {
					ctx.writeAction(c, path, "snapshot")
					ctx.writeActionsYaml(c, path, "snapshot")
				},
			},
			serveCharm{},
			ensureStateWorker{},
			createServiceAndUnit{},
			startUniter{},
			waitAddresses{},
			waitUnit{status: params.StatusActive},
			waitHooks{"install", "config-changed", "start"},
			verifyCharm{},
			addAction{
				name:   "snapshot",
				params: map[string]interface{}{"outfile": 2},
			},
			waitActionResults{[]actionResult{{
				name:    "snapshot",
				results: map[string]interface{}{},
				status:  params.ActionFailed,
				message: `cannot run "snapshot" action: JSON validation failed: (root).outfile : must be of type string, given 2`,
			}}},
			waitUnit{status: params.StatusActive},
		), ut(
			"actions not defined in actions.yaml fail without causing a uniter error",
			createCharm{
				customize: func(c *gc.C, ctx *context, path string) {
					ctx.writeAction(c, path, "snapshot")
				},
			},
			serveCharm{},
			ensureStateWorker{},
			createServiceAndUnit{},
			startUniter{},
			waitAddresses{},
			waitUnit{status: params.StatusActive},
			waitHooks{"install", "config-changed", "start"},
			verifyCharm{},
			addAction{"snapshot", map[string]interface{}{"outfile": "foo.bar"}},
			waitActionResults{[]actionResult{{
				name:    "snapshot",
				results: map[string]interface{}{},
				status:  params.ActionFailed,
				message: `cannot run "snapshot" action: not defined`,
			}}},
			waitUnit{status: params.StatusActive},
		), ut(
			"pending actions get consumed",
			createCharm{
				customize: func(c *gc.C, ctx *context, path string) {
					ctx.writeAction(c, path, "action-log")
					ctx.writeActionsYaml(c, path, "action-log")
				},
			},
			serveCharm{},
			ensureStateWorker{},
			createServiceAndUnit{},
			addAction{"action-log", nil},
			addAction{"action-log", nil},
			addAction{"action-log", nil},
			startUniter{},
			waitAddresses{},
			waitUnit{status: params.StatusActive},
			waitHooks{"install", "config-changed", "start"},
			verifyCharm{},
			waitActionResults{[]actionResult{{
				name:    "action-log",
				results: map[string]interface{}{},
				status:  params.ActionCompleted,
			}, {
				name:    "action-log",
				results: map[string]interface{}{},
				status:  params.ActionCompleted,
			}, {
				name:    "action-log",
				results: map[string]interface{}{},
				status:  params.ActionCompleted,
			}}},
			waitUnit{status: params.StatusActive},
		), ut(
			"actions not implemented fail but are not errors",
			createCharm{
				customize: func(c *gc.C, ctx *context, path string) {
					ctx.writeActionsYaml(c, path, "action-log")
				},
			},
			serveCharm{},
			ensureStateWorker{},
			createServiceAndUnit{},
			startUniter{},
			waitAddresses{},
			waitUnit{status: params.StatusActive},
			waitHooks{"install", "config-changed", "start"},
			verifyCharm{},
			addAction{"action-log", nil},
			waitActionResults{[]actionResult{{
				name:    "action-log",
				results: map[string]interface{}{},
				status:  params.ActionFailed,
				message: `action not implemented on unit "u/0"`,
			}}},
			waitUnit{status: params.StatusActive},
		), ut(
			"actions are not attempted from ModeHookError and do not clear the error",
			startupErrorWithCustomCharm{
				badHook: "start",
				customize: func(c *gc.C, ctx *context, path string) {
					ctx.writeAction(c, path, "action-log")
					ctx.writeActionsYaml(c, path, "action-log")
				},
			},
			addAction{"action-log", nil},
			waitUnit{
				status: params.StatusError,
				info:   `hook failed: "start"`,
				data: map[string]interface{}{
					"hook": "start",
				},
			},
			verifyNoActionResults{},
			verifyWaiting{},
			resolveError{state.ResolvedNoHooks},
			waitUnit{status: params.StatusActive},
			waitActionResults{[]actionResult{{
				name:    "action-log",
				results: map[string]interface{}{},
				status:  params.ActionCompleted,
			}}},
			waitUnit{status: params.StatusActive},
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
			waitHooks{"stop"},
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
		s:       s,
		st:      s.State,
		path:    filepath.Join(s.dataDir, "agents", "unit-u-0"),
		dataDir: s.dataDir,
		charms:  make(map[string][]byte),
	}

	addStateServerMachine(c, ctx.st)

	// Create the subordinate service.
	dir := testcharms.Repo.ClonedDir(c.MkDir(), "logging")
	curl, err := corecharm.ParseURL("cs:quantal/logging")
	c.Assert(err, jc.ErrorIsNil)
	curl = curl.WithRevision(dir.Revision())
	step(c, ctx, addCharm{dir, curl})
	ctx.svc = s.AddTestingService(c, "u", ctx.sch)

	// Create the principal service and add a relation.
	wps := s.AddTestingService(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	wpu, err := wps.AddUnit()
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

func (s *UniterSuite) TestReboot(c *gc.C) {
	s.runUniterTests(c, []uniterTest{
		ut(
			"test that juju-reboot disabled in actions",
			createCharm{
				customize: func(c *gc.C, ctx *context, path string) {
					ctx.writeAction(c, path, "action-reboot")
					ctx.writeActionsYaml(c, path, "action-reboot")
				},
			},
			serveCharm{},
			ensureStateWorker{},
			createServiceAndUnit{},
			addAction{"action-reboot", nil},
			startUniter{},
			waitAddresses{},
			waitUnit{
				status: params.StatusActive,
			},
			waitActionResults{[]actionResult{{
				name: "action-reboot",
				results: map[string]interface{}{
					"reboot-delayed": "good",
					"reboot-now":     "good",
				},
				status: params.ActionCompleted,
			}}},
		), ut(
			"test that juju-reboot finishes hook, and reboots",
			createCharm{
				customize: func(c *gc.C, ctx *context, path string) {
					hpath := filepath.Join(path, "hooks", "install")
					ctx.writeExplicitHook(c, hpath, rebootHook)
				},
			},
			serveCharm{},
			ensureStateWorker{},
			createServiceAndUnit{},
			startUniter{},
			waitAddresses{},
			waitUniterDead{"machine needs to reboot"},
			waitHooks{"install"},
			startUniter{},
			waitUnit{
				status: params.StatusActive,
			},
			waitHooks{"config-changed", "start"},
		), ut(
			"test that juju-reboot --now kills hook and exits",
			createCharm{
				customize: func(c *gc.C, ctx *context, path string) {
					hpath := filepath.Join(path, "hooks", "install")
					ctx.writeExplicitHook(c, hpath, rebootNowHook)
				},
			},
			serveCharm{},
			ensureStateWorker{},
			createServiceAndUnit{},
			startUniter{},
			waitAddresses{},
			waitUniterDead{"machine needs to reboot"},
			waitHooks{"install"},
			startUniter{},
			waitUnit{
				status: params.StatusActive,
			},
			waitHooks{"install", "config-changed", "start"},
		), ut(
			"test juju-reboot will not happen if hook errors out",
			createCharm{
				customize: func(c *gc.C, ctx *context, path string) {
					hpath := filepath.Join(path, "hooks", "install")
					ctx.writeExplicitHook(c, hpath, badRebootHook)
				},
			},
			serveCharm{},
			ensureStateWorker{},
			createServiceAndUnit{},
			startUniter{},
			waitAddresses{},
			waitUnit{
				status: params.StatusError,
				info:   fmt.Sprintf(`hook failed: "install"`),
			},
		),
	})
}

func (s *UniterSuite) TestRebootFromJujuRun(c *gc.C) {
	s.runUniterTests(c, []uniterTest{
		ut(
			"test juju-reboot",
			quickStart{},
			runCommands{"juju-reboot"},
			waitUniterDead{"machine needs to reboot"},
			startUniter{},
			waitHooks{"config-changed"},
		), ut(
			"test juju-reboot with bad hook",
			startupError{"install"},
			runCommands{"juju-reboot"},
			waitUniterDead{"machine needs to reboot"},
			startUniter{},
			waitHooks{},
		), ut(
			"test juju-reboot --now",
			quickStart{},
			runCommands{"juju-reboot --now"},
			waitUniterDead{"machine needs to reboot"},
			startUniter{},
			waitHooks{"config-changed"},
		), ut(
			"test juju-reboot --now with bad hook",
			startupError{"install"},
			runCommands{"juju-reboot --now"},
			waitUniterDead{"machine needs to reboot"},
			startUniter{},
			waitHooks{},
		),
	})
}
