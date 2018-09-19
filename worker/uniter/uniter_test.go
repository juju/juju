// Copyright 2012-2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter_test

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/juju/clock/testclock"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	ft "github.com/juju/testing/filetesting"
	gc "gopkg.in/check.v1"
	corecharm "gopkg.in/juju/charm.v6"

	"github.com/juju/juju/agent/tools"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/component/all"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state"
	"github.com/juju/juju/testcharms"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker/uniter/operation"
)

type UniterSuite struct {
	coretesting.GitSuite
	testing.JujuConnSuite
	dataDir  string
	oldLcAll string
	unitDir  string

	updateStatusHookTicker *manualTicker
}

var _ = gc.Suite(&UniterSuite{})

var leaseClock *testclock.Clock

// This guarantees that we get proper platform
// specific error directly from their source
// This works on both windows and unix
var errNotDir = syscall.ENOTDIR.Error()

func (s *UniterSuite) SetUpSuite(c *gc.C) {
	s.GitSuite.SetUpSuite(c)
	s.JujuConnSuite.SetUpSuite(c)
	s.dataDir = c.MkDir()
	toolsDir := tools.ToolsDir(s.dataDir, "unit-u-0")
	err := os.MkdirAll(toolsDir, 0755)
	c.Assert(err, jc.ErrorIsNil)
	// TODO(fwereade) GAAAAAAAAAAAAAAAAAH this is LUDICROUS.
	cmd := exec.Command(jujudBuildArgs[0], jujudBuildArgs[1:]...)
	cmd.Dir = toolsDir
	out, err := cmd.CombinedOutput()
	c.Logf(string(out))
	c.Assert(err, jc.ErrorIsNil)
	s.oldLcAll = os.Getenv("LC_ALL")
	os.Setenv("LC_ALL", "en_US")
	s.unitDir = filepath.Join(s.dataDir, "agents", "unit-u-0")
	all.RegisterForServer()
}

func (s *UniterSuite) TearDownSuite(c *gc.C) {
	os.Setenv("LC_ALL", s.oldLcAll)
	s.JujuConnSuite.TearDownSuite(c)
	s.GitSuite.TearDownSuite(c)
}

func (s *UniterSuite) SetUpTest(c *gc.C) {
	zone, err := time.LoadLocation("")
	c.Assert(err, jc.ErrorIsNil)
	now := time.Date(2030, 11, 11, 11, 11, 11, 11, zone)
	leaseClock = testclock.NewClock(now)
	s.updateStatusHookTicker = newManualTicker()
	s.GitSuite.SetUpTest(c)
	s.JujuConnSuite.SetUpTest(c)
	err = s.State.SetClockForTesting(leaseClock)
	c.Assert(err, jc.ErrorIsNil)
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
			createApplicationAndUnit{},
			startUniter{},
			waitUniterDead{err: `failed to initialize uniter for "unit-u-0": .*` + errNotDir},
		), ut(
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
			createDownloads{},
			startUniter{},
			waitAddresses{},
			waitUnitAgent{status: status.Idle},
			verifyDownloadsCleared{},
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
			waitUniterDead{err: `executing operation "install cs:quantal/wordpress-0": open .*` + errNotDir},
		), ut(
			"charm cannot be downloaded",
			createCharm{},
			// don't serve charm
			createUniter{},
			waitUniterDead{err: `preparing operation "install cs:quantal/wordpress-0": failed to download charm .* not found`},
		),
	})
}

type noopExecutor struct {
	operation.Executor
}

func (m *noopExecutor) Run(op operation.Operation) error {
	return errors.New("some error occurred")
}

func (s *UniterSuite) TestUniterStartupStatus(c *gc.C) {
	executorFunc := func(stateFilePath string, initialState operation.State, acquireLock func(string) (func(), error)) (operation.Executor, error) {
		e, err := operation.NewExecutor(stateFilePath, initialState, acquireLock)
		c.Assert(err, jc.ErrorIsNil)
		return &mockExecutor{e}, nil
	}
	s.runUniterTests(c, []uniterTest{
		ut(
			"unit status and message at startup",
			createCharm{},
			serveCharm{},
			ensureStateWorker{},
			createApplicationAndUnit{},
			startUniter{
				newExecutorFunc: executorFunc,
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
			waitHooks{"config-changed"},
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
			waitHooks{"start", "config-changed"},
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
		), ut(
			"steady state config change with config-get verification",
			createCharm{
				customize: func(c *gc.C, ctx *context, path string) {
					appendHook(c, path, "config-changed", appendConfigChanged)
				},
			},
			serveCharm{},
			createUniter{},
			waitUnitAgent{
				status: status.Idle,
			},
			waitHooks{"install", "leader-elected", "config-changed", "start"},
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

func (s *UniterSuite) TestUniterUpgradeOverwrite(c *gc.C) {
	//TODO(bogdanteleaga): Fix this on windows
	coretesting.SkipIfWindowsBug(c, "lp:1403084")
	//TODO(hml): Fix this on S390X, intermittent there.
	coretesting.SkipIfS390X(c, "lp:1534637")
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
			waitUnitAgent{
				status: status.Idle,
			},
			waitUnitAgent{
				statusGetter: unitStatusGetter,
				status:       status.Unknown,
			},
			waitHooks{"install", "leader-elected", "config-changed", "start"},

			createCharm{
				revision: 1,
				customize: func(c *gc.C, _ *context, path string) {
					content.Create(c, path)
				},
			},
			serveCharm{},
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

func (s *UniterSuite) TestUniterRelations(c *gc.C) {
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
		}, {
			"db-relation-departed mysql/0 db:0",
			"leader-settings-changed",
			"db-relation-broken db:0",
			"stop",
		}, {
			"db-relation-departed mysql/0 db:0",
			"db-relation-broken db:0",
			"leader-settings-changed",
			"stop",
		}, {
			"db-relation-departed mysql/0 db:0",
			"db-relation-broken db:0",
			"stop",
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
			"simple joined/changed/departed",
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
		), ut(
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
					script := uniterRelationsCustomizeScript
					appendHook(c, path, "config-changed", script)
				},
			},
			serveCharm{},
			createUniter{},
			waitUnitAgent{
				status: status.Idle,
			},
			waitUnitAgent{
				statusGetter: unitStatusGetter,
				status:       status.Unknown,
			},
			waitHooks{"install", "leader-elected", "config-changed", "start"},
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
			addAction{"action-log", nil},
			waitActionResults{[]actionResult{{
				name:    "action-log",
				results: map[string]interface{}{},
				status:  params.ActionCompleted,
			}}},
			waitUnitAgent{status: status.Idle},
			waitUnitAgent{
				statusGetter: unitStatusGetter,
				status:       status.Unknown,
			},
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
			addAction{"action-log-fail", nil},
			waitActionResults{[]actionResult{{
				name: "action-log-fail",
				results: map[string]interface{}{
					"foo": "still works",
				},
				message: "I'm afraid I can't let you do that, Dave.",
				status:  params.ActionFailed,
			}}},
			waitUnitAgent{status: status.Idle}, waitUnitAgent{
				statusGetter: unitStatusGetter,
				status:       status.Unknown,
			},
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
			addAction{"action-log-fail-error", nil},
			waitActionResults{[]actionResult{{
				name: "action-log-fail-error",
				results: map[string]interface{}{
					"foo": "still works",
				},
				message: "A real message",
				status:  params.ActionFailed,
			}}},
			waitUnitAgent{status: status.Idle},
			waitUnitAgent{
				statusGetter: unitStatusGetter,
				status:       status.Unknown,
			},
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
			waitUnitAgent{status: status.Idle},
			waitUnitAgent{
				statusGetter: unitStatusGetter,
				status:       status.Unknown,
			},
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
			addAction{
				name:   "snapshot",
				params: map[string]interface{}{"outfile": 2},
			},
			waitActionResults{[]actionResult{{
				name:    "snapshot",
				results: map[string]interface{}{},
				status:  params.ActionFailed,
				message: `cannot run "snapshot" action: validation failed: (root).outfile : must be of type string, given 2`,
			}}},
			waitUnitAgent{status: status.Idle},
			waitUnitAgent{
				statusGetter: unitStatusGetter,
				status:       status.Unknown,
			},
		), ut(
			"actions not defined in actions.yaml fail without causing a uniter error",
			createCharm{
				customize: func(c *gc.C, ctx *context, path string) {
					ctx.writeAction(c, path, "snapshot")
				},
			},
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
			addAction{"snapshot", map[string]interface{}{"outfile": "foo.bar"}},
			waitActionResults{[]actionResult{{
				name:    "snapshot",
				results: map[string]interface{}{},
				status:  params.ActionFailed,
				message: `cannot run "snapshot" action: not defined`,
			}}},
			waitUnitAgent{status: status.Idle},
			waitUnitAgent{
				statusGetter: unitStatusGetter,
				status:       status.Unknown,
			},
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
			createApplicationAndUnit{},
			addAction{"action-log", nil},
			addAction{"action-log", nil},
			addAction{"action-log", nil},
			startUniter{},
			waitAddresses{},
			waitUnitAgent{status: status.Idle},
			waitUnitAgent{
				statusGetter: unitStatusGetter,
				status:       status.Unknown,
			},
			waitHooks{"install", "leader-elected", "config-changed", "start"},
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
			waitUnitAgent{status: status.Idle},
			waitUnitAgent{
				statusGetter: unitStatusGetter,
				status:       status.Unknown,
			},
		), ut(
			"actions not implemented fail but are not errors",
			createCharm{
				customize: func(c *gc.C, ctx *context, path string) {
					ctx.writeActionsYaml(c, path, "action-log")
				},
			},
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
			addAction{"action-log", nil},
			waitActionResults{[]actionResult{{
				name:    "action-log",
				results: map[string]interface{}{},
				status:  params.ActionFailed,
				message: `action not implemented on unit "u/0"`,
			}}},
			waitUnitAgent{status: status.Idle},
			waitUnitAgent{
				statusGetter: unitStatusGetter,
				status:       status.Unknown,
			},
		), ut(
			"actions may run from ModeHookError, but do not clear the error",
			startupErrorWithCustomCharm{
				badHook: "start",
				customize: func(c *gc.C, ctx *context, path string) {
					ctx.writeAction(c, path, "action-log")
					ctx.writeActionsYaml(c, path, "action-log")
				},
			},
			addAction{"action-log", nil},
			waitUnitAgent{
				statusGetter: unitStatusGetter,
				status:       status.Error,
				info:         `hook failed: "start"`,
				data:         map[string]interface{}{"hook": "start"},
			},
			waitActionResults{[]actionResult{{
				name:    "action-log",
				results: map[string]interface{}{},
				status:  params.ActionCompleted,
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

func (s *UniterSuite) TestRebootDisabledInActions(c *gc.C) {
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
			createApplicationAndUnit{},
			addAction{"action-reboot", nil},
			startUniter{},
			waitAddresses{},
			waitUnitAgent{
				status: status.Idle,
			},
			waitUnitAgent{
				statusGetter: unitStatusGetter,
				status:       status.Unknown,
			},
			waitActionResults{[]actionResult{{
				name: "action-reboot",
				results: map[string]interface{}{
					"reboot-delayed": "good",
					"reboot-now":     "good",
				},
				status: params.ActionCompleted,
			}}},
		)})
}

func (s *UniterSuite) TestRebootFinishesHook(c *gc.C) {
	s.runUniterTests(c, []uniterTest{
		ut(
			"test that juju-reboot finishes hook, and reboots",
			createCharm{
				customize: func(c *gc.C, ctx *context, path string) {
					hpath := filepath.Join(path, "hooks", "install")
					ctx.writeExplicitHook(c, hpath, rebootHook)
				},
			},
			serveCharm{},
			ensureStateWorker{},
			createApplicationAndUnit{},
			startUniter{},
			waitAddresses{},
			waitUniterDead{err: "machine needs to reboot"},
			waitHooks{"install"},
			startUniter{},
			waitUnitAgent{
				status: status.Idle,
			},
			waitUnitAgent{
				statusGetter: unitStatusGetter,
				status:       status.Unknown,
			},
			waitHooks{"leader-elected", "config-changed", "start"},
		)})
}

func (s *UniterSuite) TestRebootNowKillsHook(c *gc.C) {
	s.runUniterTests(c, []uniterTest{
		ut(
			"test that juju-reboot --now kills hook and exits",
			createCharm{
				customize: func(c *gc.C, ctx *context, path string) {
					hpath := filepath.Join(path, "hooks", "install")
					ctx.writeExplicitHook(c, hpath, rebootNowHook)
				},
			},
			serveCharm{},
			ensureStateWorker{},
			createApplicationAndUnit{},
			startUniter{},
			waitAddresses{},
			waitUniterDead{err: "machine needs to reboot"},
			waitHooks{"install"},
			startUniter{},
			waitUnitAgent{
				status: status.Idle,
			},
			waitUnitAgent{
				statusGetter: unitStatusGetter,
				status:       status.Unknown,
			},
			waitHooks{"install", "leader-elected", "config-changed", "start"},
		)})
}

func (s *UniterSuite) TestRebootDisabledOnHookError(c *gc.C) {
	s.runUniterTests(c, []uniterTest{
		ut(
			"test juju-reboot will not happen if hook errors out",
			createCharm{
				customize: func(c *gc.C, ctx *context, path string) {
					hpath := filepath.Join(path, "hooks", "install")
					ctx.writeExplicitHook(c, hpath, badRebootHook)
				},
			},
			serveCharm{},
			ensureStateWorker{},
			createApplicationAndUnit{},
			startUniter{},
			waitAddresses{},
			waitUnitAgent{
				statusGetter: unitStatusGetter,
				status:       status.Error,
				info:         fmt.Sprintf(`hook failed: "install"`),
			},
		),
	})
}

func (s *UniterSuite) TestJujuRunExecutionSerialized(c *gc.C) {
	s.runUniterTests(c, []uniterTest{
		ut(
			"hook failed status should stay around after juju run",
			createCharm{badHooks: []string{"config-changed"}},
			serveCharm{},
			createUniter{},
			waitUnitAgent{
				statusGetter: unitStatusGetter,
				status:       status.Error,
				info:         `hook failed: "config-changed"`,
				data: map[string]interface{}{
					"hook": "config-changed",
				},
			},
			runCommands{"exit 0"},
			waitUnitAgent{
				statusGetter: unitStatusGetter,
				status:       status.Error,
				info:         `hook failed: "config-changed"`,
				data: map[string]interface{}{
					"hook": "config-changed",
				},
			},
		)})
}

func (s *UniterSuite) TestRebootFromJujuRun(c *gc.C) {
	//TODO(bogdanteleaga): Fix this on windows
	if runtime.GOOS == "windows" {
		c.Skip("bug 1403084: currently does not work on windows")
	}
	s.runUniterTests(c, []uniterTest{
		ut(
			"test juju-reboot",
			quickStart{},
			runCommands{"juju-reboot"},
			waitUniterDead{err: "machine needs to reboot"},
			startUniter{},
			waitHooks{"config-changed"},
		), ut(
			"test juju-reboot with bad hook",
			startupError{"install"},
			runCommands{"juju-reboot"},
			waitUniterDead{err: "machine needs to reboot"},
			startUniter{},
			waitHooks{},
		), ut(
			"test juju-reboot --now",
			quickStart{},
			runCommands{"juju-reboot --now"},
			waitUniterDead{err: "machine needs to reboot"},
			startUniter{},
			waitHooks{"config-changed"},
		), ut(
			"test juju-reboot --now with bad hook",
			startupError{"install"},
			runCommands{"juju-reboot --now"},
			waitUniterDead{err: "machine needs to reboot"},
			startUniter{},
			waitHooks{},
		),
	})
}

func (s *UniterSuite) TestLeadership(c *gc.C) {
	s.runUniterTests(c, []uniterTest{
		ut(
			"hook tools when leader",
			quickStart{},
			runCommands{"leader-set foo=bar baz=qux"},
			verifyLeaderSettings{"foo": "bar", "baz": "qux"},
		), ut(
			"hook tools when not leader",
			quickStart{minion: true},
			runCommands{leadershipScript},
		), ut(
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

func (m *mockExecutor) Run(op operation.Operation) error {
	// want to allow charm unpacking to occur
	if strings.HasPrefix(op.String(), "install") {
		return m.Executor.Run(op)
	}
	// but hooks should error
	return mockExecutorErr
}

func (s *UniterSuite) TestOperationErrorReported(c *gc.C) {
	executorFunc := func(stateFilePath string, initialState operation.State, acquireLock func(string) (func(), error)) (operation.Executor, error) {
		e, err := operation.NewExecutor(stateFilePath, initialState, acquireLock)
		c.Assert(err, jc.ErrorIsNil)
		return &mockExecutor{e}, nil
	}
	s.runUniterTests(c, []uniterTest{
		ut(
			"error running operations are reported",
			createCharm{},
			serveCharm{},
			createUniter{executorFunc: executorFunc},
			waitUnitAgent{
				status: status.Failed,
				info:   "resolver loop error",
			},
			expectError{".*some error occurred.*"},
		),
	})
}

func (s *UniterSuite) TestTranslateResolverError(c *gc.C) {
	executorFunc := func(stateFilePath string, initialState operation.State, acquireLock func(string) (func(), error)) (operation.Executor, error) {
		e, err := operation.NewExecutor(stateFilePath, initialState, acquireLock)
		c.Assert(err, jc.ErrorIsNil)
		return &mockExecutor{e}, nil
	}
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
				executorFunc:         executorFunc,
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
