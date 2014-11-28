// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter_test

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"net/rpc"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/juju/errors"
	"github.com/juju/names"
	gt "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	ft "github.com/juju/testing/filetesting"
	"github.com/juju/utils"
	utilexec "github.com/juju/utils/exec"
	"github.com/juju/utils/fslock"
	"github.com/juju/utils/proxy"
	gc "gopkg.in/check.v1"
	corecharm "gopkg.in/juju/charm.v4"
	goyaml "gopkg.in/yaml.v1"

	"github.com/juju/juju/agent/tools"
	"github.com/juju/juju/api"
	apiuniter "github.com/juju/juju/api/uniter"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/juju/testing"
	"github.com/juju/juju/network"
	"github.com/juju/juju/state"
	"github.com/juju/juju/testcharms"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/uniter"
	"github.com/juju/juju/worker/uniter/charm"
)

// worstCase is used for timeouts when timing out
// will fail the test. Raising this value should
// not affect the overall running time of the tests
// unless they fail.
const worstCase = coretesting.LongWait

type UniterSuite struct {
	coretesting.GitSuite
	testing.JujuConnSuite
	dataDir  string
	oldLcAll string
	unitDir  string

	st     *api.State
	uniter *apiuniter.State
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

func (s *UniterSuite) APILogin(c *gc.C, unit *state.Unit) {
	password, err := utils.RandomPassword()
	c.Assert(err, jc.ErrorIsNil)
	err = unit.SetPassword(password)
	c.Assert(err, jc.ErrorIsNil)
	s.st = s.OpenAPIAs(c, unit.Tag(), password)
	c.Assert(s.st, gc.NotNil)
	c.Logf("API: login as %q successful", unit.Tag())
	s.uniter, err = s.st.Uniter()
	c.Assert(err, jc.ErrorIsNil)
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
	charms        map[string][]byte
	hooks         []string
	sch           *state.Charm
	svc           *state.Service
	unit          *state.Unit
	uniter        *uniter.Uniter
	relatedSvc    *state.Service
	relation      *state.Relation
	relationUnits map[string]*state.RelationUnit
	subordinate   *state.Unit
	ticker        *uniter.ManualTicker

	mu             sync.Mutex
	hooksCompleted []string
}

var _ uniter.UniterExecutionObserver = (*context)(nil)

func (ctx *context) HookCompleted(hookName string) {
	ctx.mu.Lock()
	ctx.hooksCompleted = append(ctx.hooksCompleted, hookName)
	ctx.mu.Unlock()
}

func (ctx *context) HookFailed(hookName string) {
	ctx.mu.Lock()
	ctx.hooksCompleted = append(ctx.hooksCompleted, "fail-"+hookName)
	ctx.mu.Unlock()
}

func (ctx *context) run(c *gc.C, steps []stepper) {
	defer func() {
		if ctx.uniter != nil {
			err := ctx.uniter.Stop()
			c.Assert(err, jc.ErrorIsNil)
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

func (ctx *context) writeExplicitHook(c *gc.C, path string, contents string) {
	err := ioutil.WriteFile(path, []byte(contents), 0755)
	c.Assert(err, jc.ErrorIsNil)
}

func (ctx *context) writeHook(c *gc.C, path string, good bool) {
	hook := badHook
	if good {
		hook = goodHook
	}
	content := fmt.Sprintf(hook, filepath.Base(path))
	ctx.writeExplicitHook(c, path, content)
}

func (ctx *context) writeActions(c *gc.C, path string, names []string) {
	for _, name := range names {
		ctx.writeAction(c, path, name)
	}
}

func (ctx *context) writeMetricsYaml(c *gc.C, path string) {
	metricsYamlPath := filepath.Join(path, "metrics.yaml")
	var metricsYamlFull []byte = []byte(`
metrics:
  pings:
    type: gauge
    description: sample metric
`)
	err := ioutil.WriteFile(metricsYamlPath, []byte(metricsYamlFull), 0755)
	c.Assert(err, jc.ErrorIsNil)
}

func (ctx *context) writeAction(c *gc.C, path, name string) {
	var actions = map[string]string{
		"action-log": `
#!/bin/bash --norc
juju-log $JUJU_ENV_UUID action-log
`[1:],
		"snapshot": `
#!/bin/bash --norc
action-set outfile.name="snapshot-01.tar" outfile.size="10.3GB"
action-set outfile.size.magnitude="10.3" outfile.size.units="GB"
action-set completion.status="yes" completion.time="5m"
action-set completion="yes"
`[1:],
		"action-log-fail": `
#!/bin/bash --norc
action-fail "I'm afraid I can't let you do that, Dave."
action-set foo="still works"
`[1:],
		"action-log-fail-error": `
#!/bin/bash --norc
action-fail too many arguments
action-set foo="still works"
action-fail "A real message"
`[1:],
		"action-reboot": `
#!/bin/bash --norc
juju-reboot || action-set reboot-delayed="good"
juju-reboot --now || action-set reboot-now="good"
`[1:],
	}

	actionPath := filepath.Join(path, "actions", name)
	action := actions[name]
	err := ioutil.WriteFile(actionPath, []byte(action), 0755)
	c.Assert(err, jc.ErrorIsNil)
}

func (ctx *context) writeActionsYaml(c *gc.C, path string, names ...string) {
	var actionsYaml = map[string]string{
		"base": `
actions:
`[1:],
		"snapshot": `
   snapshot:
      description: Take a snapshot of the database.                           
      params:                                                                 
         title: "Snapshot"                                                    
         type: "object"                                                       
         properties:                                                          
            outfile:                                                          
               description: "The file to write out to."                       
               type: string                                                   
         required: ["outfile"]
`[1:],
		"action-log": `
   action-log:
      params:
`[1:],
		"action-log-fail": `
   action-log-fail:
      params:
`[1:],
		"action-log-fail-error": `
   action-log-fail-error:
      params:
`[1:],
		"action-reboot": `
   action-reboot:
      params:
`[1:],
	}
	actionsYamlPath := filepath.Join(path, "actions.yaml")
	var actionsYamlFull string
	// Build an appropriate actions.yaml
	if names[0] != "base" {
		names = append([]string{"base"}, names...)
	}
	for _, name := range names {
		actionsYamlFull = strings.Join(
			[]string{actionsYamlFull, actionsYaml[name]}, "\n")
	}
	err := ioutil.WriteFile(actionsYamlPath, []byte(actionsYamlFull), 0755)
	c.Assert(err, jc.ErrorIsNil)
}

func (ctx *context) matchHooks(c *gc.C) (match bool, overshoot bool) {
	ctx.mu.Lock()
	defer ctx.mu.Unlock()
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
		waitUniterDead{`ModeInstalling cs:quantal/wordpress-0: executing operation "install cs:quantal/wordpress-0": open .*: not a directory`},
	), ut(
		"charm cannot be downloaded",
		createCharm{},
		// don't serve charm
		createUniter{},
		waitUniterDead{`ModeInstalling cs:quantal/wordpress-0: preparing operation "install cs:quantal/wordpress-0": failed to download charm .* 404 Not Found`},
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
			data: map[string]interface{}{
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
			data: map[string]interface{}{
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
			data: map[string]interface{}{
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
			status: params.StatusStarted,
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
}

func (s *UniterSuite) TestUniterSteadyStateUpgrade(c *gc.C) {
	s.runUniterTests(c, steadyUpgradeTests)
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
				status: params.StatusStarted,
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
				status: params.StatusStarted,
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
			data: map[string]interface{}{
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
				status: params.StatusStarted,
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
				status: params.StatusStarted,
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
				status: params.StatusStarted,
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
				status: params.StatusStarted,
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

var upgradeConflictsTests = []uniterTest{
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
			status: params.StatusStarted,
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
			status: params.StatusStarted,
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
		path := filepath.Join(testDir, name)
		template := "echo juju run ${JUJU_UNIT_NAME} > %s.tmp; mv %s.tmp %s"
		return fmt.Sprintf(template, path, path, path)
	}
	adminTag := s.AdminUserTag(c)
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
			status: params.StatusStarted,
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
}

func (s *UniterSuite) TestUniterRelationErrors(c *gc.C) {
	s.runUniterTests(c, relationsErrorTests)
}

var meterStatusEventTests = []uniterTest{
	ut(
		"meter status event triggered by unit meter status change",
		quickStart{},
		changeMeterStatus{"AMBER", "Investigate charm."},
		waitHooks{"meter-status-changed"},
	),
}

func (s *UniterSuite) TestUniterMeterStatusChanged(c *gc.C) {
	s.runUniterTests(c, meterStatusEventTests)
}

var collectMetricsEventTests = []uniterTest{
	ut(
		"collect-metrics event triggered by manual timer",
		createCharm{
			customize: func(c *gc.C, ctx *context, path string) {
				ctx.writeMetricsYaml(c, path)
			},
		},
		serveCharm{},
		createUniter{},
		waitUnit{status: params.StatusStarted},
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
			status: params.StatusStarted,
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
			status: params.StatusStarted,
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
}

func (s *UniterSuite) TestUniterCollectMetrics(c *gc.C) {
	s.runUniterTests(c, collectMetricsEventTests)
}

var actionEventTests = []uniterTest{
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
		waitUnit{status: params.StatusStarted},
		waitHooks{"install", "config-changed", "start"},
		verifyCharm{},
		addAction{"action-log", nil},
		waitActionResults{[]actionResult{{
			name:    "action-log",
			results: map[string]interface{}{},
			status:  params.ActionCompleted,
		}}},
		waitUnit{status: params.StatusStarted},
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
		waitUnit{status: params.StatusStarted},
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
		waitUnit{status: params.StatusStarted},
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
		waitUnit{status: params.StatusStarted},
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
		waitUnit{status: params.StatusStarted},
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
		waitUnit{status: params.StatusStarted},
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
		waitUnit{status: params.StatusStarted},
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
		waitUnit{status: params.StatusStarted},
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
		waitUnit{status: params.StatusStarted},
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
		waitUnit{status: params.StatusStarted},
		waitHooks{"install", "config-changed", "start"},
		verifyCharm{},
		addAction{"snapshot", map[string]interface{}{"outfile": "foo.bar"}},
		waitActionResults{[]actionResult{{
			name:    "snapshot",
			results: map[string]interface{}{},
			status:  params.ActionFailed,
			message: `cannot run "snapshot" action: not defined`,
		}}},
		waitUnit{status: params.StatusStarted},
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
		waitUnit{status: params.StatusStarted},
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
		waitUnit{status: params.StatusStarted},
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
		waitUnit{status: params.StatusStarted},
		waitHooks{"install", "config-changed", "start"},
		verifyCharm{},
		addAction{"action-log", nil},
		waitActionResults{[]actionResult{{
			name:    "action-log",
			results: map[string]interface{}{},
			status:  params.ActionFailed,
			message: `action not implemented on unit "u/0"`,
		}}},
		waitUnit{status: params.StatusStarted},
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
		waitUnit{status: params.StatusStarted},
		waitActionResults{[]actionResult{{
			name:    "action-log",
			results: map[string]interface{}{},
			status:  params.ActionCompleted,
		}}},
		waitUnit{status: params.StatusStarted},
	),
}

func (s *UniterSuite) TestActionEvents(c *gc.C) {
	s.runUniterTests(c, actionEventTests)
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

// Assign the unit to a provisioned machine with dummy addresses set.
func assertAssignUnit(c *gc.C, st *state.State, u *state.Unit) {
	err := u.AssignToNewMachine()
	c.Assert(err, jc.ErrorIsNil)
	mid, err := u.AssignedMachineId()
	c.Assert(err, jc.ErrorIsNil)
	machine, err := st.Machine(mid)
	c.Assert(err, jc.ErrorIsNil)
	err = machine.SetProvisioned("i-exist", "fake_nonce", nil)
	c.Assert(err, jc.ErrorIsNil)
	err = machine.SetAddresses(network.Address{
		Type:  network.IPv4Address,
		Scope: network.ScopeCloudLocal,
		Value: "private.address.example.com",
	}, network.Address{
		Type:  network.IPv4Address,
		Scope: network.ScopePublic,
		Value: "public.address.example.com",
	})
	c.Assert(err, jc.ErrorIsNil)
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

var rebootHook = `
#!/bin/bash --norc
juju-reboot
`[1:]

var badRebootHook = `
#!/bin/bash --norc
juju-reboot
exit 1
`[1:]

var rebootNowHook = `
#!/bin/bash --norc

if [ -f "i_have_risen" ]
then
    exit 0
fi
touch i_have_risen
juju-reboot --now
`[1:]

var rebootTests = []uniterTest{
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
			status: params.StatusStarted,
		},
		waitActionResults{[]actionResult{{
			name: "action-reboot",
			results: map[string]interface{}{
				"reboot-delayed": "good",
				"reboot-now":     "good",
			},
			status: params.ActionCompleted,
		}}},
	),
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
		createServiceAndUnit{},
		startUniter{},
		waitAddresses{},
		waitUniterDead{"machine needs to reboot"},
		waitHooks{"install"},
		startUniter{},
		waitUnit{
			status: params.StatusStarted,
		},
		waitHooks{"config-changed", "start"},
	),
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
		createServiceAndUnit{},
		startUniter{},
		waitAddresses{},
		waitUniterDead{"machine needs to reboot"},
		waitHooks{"install"},
		startUniter{},
		waitUnit{
			status: params.StatusStarted,
		},
		waitHooks{"install", "config-changed", "start"},
	),
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
		createServiceAndUnit{},
		startUniter{},
		waitAddresses{},
		waitUnit{
			status: params.StatusError,
			info:   fmt.Sprintf(`hook failed: "install"`),
		},
	),
}

func (s *UniterSuite) TestReboot(c *gc.C) {
	s.runUniterTests(c, rebootTests)
}

var jujuRunRebootTests = []uniterTest{
	ut(
		"test juju-reboot",
		quickStart{},
		runCommands{"juju-reboot"},
		waitUniterDead{"machine needs to reboot"},
		startUniter{},
		waitHooks{"config-changed"},
	),
	ut(
		"test juju-reboot with bad hook",
		startupError{"install"},
		runCommands{"juju-reboot"},
		waitUniterDead{"machine needs to reboot"},
		startUniter{},
		waitHooks{},
	),
	ut(
		"test juju-reboot --now",
		quickStart{},
		runCommands{"juju-reboot --now"},
		waitUniterDead{"machine needs to reboot"},
		startUniter{},
		waitHooks{"config-changed"},
	),
	ut(
		"test juju-reboot --now with bad hook",
		startupError{"install"},
		runCommands{"juju-reboot --now"},
		waitUniterDead{"machine needs to reboot"},
		startUniter{},
		waitHooks{},
	),
}

func (s *UniterSuite) TestRebootFromJujuRun(c *gc.C) {
	s.runUniterTests(c, jujuRunRebootTests)
}

func step(c *gc.C, ctx *context, s stepper) {
	c.Logf("%#v", s)
	s.step(c, ctx)
}

type ensureStateWorker struct{}

func (s ensureStateWorker) step(c *gc.C, ctx *context) {
	addresses, err := ctx.st.Addresses()
	if err != nil || len(addresses) == 0 {
		addStateServerMachine(c, ctx.st)
	}
	addresses, err = ctx.st.APIAddressesFromMachines()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(addresses, gc.HasLen, 1)
}

func addStateServerMachine(c *gc.C, st *state.State) {
	// The AddStateServerMachine call will update the API host ports
	// to made-up addresses. We need valid addresses so that the uniter
	// can download charms from the API server.
	apiHostPorts, err := st.APIHostPorts()
	c.Assert(err, gc.IsNil)
	testing.AddStateServerMachine(c, st)
	err = st.SetAPIHostPorts(apiHostPorts)
	c.Assert(err, gc.IsNil)
}

type createCharm struct {
	revision  int
	badHooks  []string
	customize func(*gc.C, *context, string)
}

var charmHooks = []string{
	"install", "start", "config-changed", "upgrade-charm", "stop",
	"db-relation-joined", "db-relation-changed", "db-relation-departed",
	"db-relation-broken", "meter-status-changed", "collect-metrics",
}

func (s createCharm) step(c *gc.C, ctx *context) {
	base := testcharms.Repo.ClonedDirPath(c.MkDir(), "wordpress")
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
	dir, err := corecharm.ReadCharmDir(base)
	c.Assert(err, jc.ErrorIsNil)
	err = dir.SetDiskRevision(s.revision)
	c.Assert(err, jc.ErrorIsNil)
	step(c, ctx, addCharm{dir, curl(s.revision)})
}

type addCharm struct {
	dir  *corecharm.CharmDir
	curl *corecharm.URL
}

func (s addCharm) step(c *gc.C, ctx *context) {
	var buf bytes.Buffer
	err := s.dir.ArchiveTo(&buf)
	c.Assert(err, jc.ErrorIsNil)
	body := buf.Bytes()
	hash, _, err := utils.ReadSHA256(&buf)
	c.Assert(err, jc.ErrorIsNil)

	storagePath := fmt.Sprintf("/charms/%s/%d", s.dir.Meta().Name, s.dir.Revision())
	ctx.charms[storagePath] = body
	ctx.sch, err = ctx.st.AddCharm(s.dir, s.curl, storagePath, hash)
	c.Assert(err, jc.ErrorIsNil)
}

type serveCharm struct{}

func (s serveCharm) step(c *gc.C, ctx *context) {
	storage := ctx.st.Storage()
	for storagePath, data := range ctx.charms {
		err := storage.Put(storagePath, bytes.NewReader(data), int64(len(data)))
		c.Assert(err, jc.ErrorIsNil)
		delete(ctx.charms, storagePath)
	}
}

type createServiceAndUnit struct {
	serviceName string
}

func (csau createServiceAndUnit) step(c *gc.C, ctx *context) {
	if csau.serviceName == "" {
		csau.serviceName = "u"
	}
	sch, err := ctx.st.Charm(curl(0))
	c.Assert(err, jc.ErrorIsNil)
	svc := ctx.s.AddTestingService(c, csau.serviceName, sch)
	unit, err := svc.AddUnit()
	c.Assert(err, jc.ErrorIsNil)

	// Assign the unit to a provisioned machine to match expected state.
	assertAssignUnit(c, ctx.st, unit)
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
			if private != "private.address.example.com" {
				continue
			}
			public, _ := ctx.unit.PublicAddress()
			if public != "public.address.example.com" {
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
	tag, err := names.ParseUnitTag(s.unitTag)
	if err != nil {
		panic(err.Error())
	}
	locksDir := filepath.Join(ctx.dataDir, "locks")
	lock, err := fslock.NewLock(locksDir, "uniter-hook-execution")
	c.Assert(err, jc.ErrorIsNil)
	ctx.uniter = uniter.NewUniter(ctx.s.uniter, tag, ctx.dataDir, lock)
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
	c.Assert(err, jc.ErrorIsNil)
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
		c.Assert(err, jc.ErrorIsNil)
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
}

func (s verifyRunning) step(c *gc.C, ctx *context) {
	step(c, ctx, stopUniter{})
	step(c, ctx, startUniter{})
	step(c, ctx, waitHooks{"config-changed"})
}

type startupErrorWithCustomCharm struct {
	badHook   string
	customize func(*gc.C, *context, string)
}

func (s startupErrorWithCustomCharm) step(c *gc.C, ctx *context) {
	step(c, ctx, createCharm{
		badHooks:  []string{s.badHook},
		customize: s.customize,
	})
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
	c.Assert(err, jc.ErrorIsNil)
}

type waitUnit struct {
	status   params.Status
	info     string
	data     map[string]interface{}
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
			c.Assert(err, jc.ErrorIsNil)
			if string(status) != string(s.status) {
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

type actionResult struct {
	name    string
	results map[string]interface{}
	status  string
	message string
}

type waitActionResults struct {
	expectedResults []actionResult
}

func (s waitActionResults) step(c *gc.C, ctx *context) {
	resultsWatcher := ctx.st.WatchActionResults()
	defer func() {
		c.Assert(resultsWatcher.Stop(), gc.IsNil)
	}()
	timeout := time.After(worstCase)
	for {
		ctx.s.BackingState.StartSync()
		select {
		case <-time.After(coretesting.ShortWait):
			continue
		case <-timeout:
			c.Fatalf("timed out waiting for action results")
		case changes, ok := <-resultsWatcher.Changes():
			c.Logf("Got changes: %#v", changes)
			c.Assert(ok, jc.IsTrue)
			stateActionResults, err := ctx.unit.CompletedActions()
			c.Assert(err, jc.ErrorIsNil)
			if len(stateActionResults) != len(s.expectedResults) {
				continue
			}
			actualResults := make([]actionResult, len(stateActionResults))
			for i, result := range stateActionResults {
				results, message := result.Results()
				actualResults[i] = actionResult{
					name:    result.Name(),
					results: results,
					status:  string(result.Status()),
					message: message,
				}
			}
			assertActionResultsMatch(c, actualResults, s.expectedResults)
			return
		}
	}
}

func assertActionResultsMatch(c *gc.C, actualIn []actionResult, expectIn []actionResult) {
	matches := 0
	desiredMatches := len(actualIn)
	c.Assert(len(actualIn), gc.Equals, len(expectIn))
findMatch:
	for _, expectedItem := range expectIn {
		// find expectedItem in actualIn
		for j, actualItem := range actualIn {
			// If we find a match, remove both items from their
			// respective slices, increment match count, and restart.
			if reflect.DeepEqual(actualItem, expectedItem) {
				actualIn = append(actualIn[:j], actualIn[j+1:]...)
				matches++
				continue findMatch
			}
		}
		// if we finish the whole thing without finding a match, we failed.
		c.Assert(actualIn, jc.DeepEquals, expectIn)
	}

	c.Assert(matches, gc.Equals, desiredMatches)
}

type verifyNoActionResults struct{}

func (s verifyNoActionResults) step(c *gc.C, ctx *context) {
	time.Sleep(coretesting.ShortWait)
	result, err := ctx.unit.CompletedActions()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.HasLen, 0)
}

type fixHook struct {
	name string
}

func (s fixHook) step(c *gc.C, ctx *context) {
	path := filepath.Join(ctx.path, "charm", "hooks", s.name)
	ctx.writeHook(c, path, true)
}

type changeMeterStatus struct {
	code string
	info string
}

func (s changeMeterStatus) step(c *gc.C, ctx *context) {
	err := ctx.unit.SetMeterStatus(s.code, s.info)
	c.Assert(err, jc.ErrorIsNil)
}

type metricsTick struct{}

func (s metricsTick) step(c *gc.C, ctx *context) {
	err := ctx.ticker.Tick()
	c.Assert(err, jc.ErrorIsNil)
}

type changeConfig map[string]interface{}

func (s changeConfig) step(c *gc.C, ctx *context) {
	err := ctx.svc.UpdateConfigSettings(corecharm.Settings(s))
	c.Assert(err, jc.ErrorIsNil)
}

type addAction struct {
	name   string
	params map[string]interface{}
}

func (s addAction) step(c *gc.C, ctx *context) {
	_, err := ctx.unit.AddAction(s.name, s.params)
	c.Assert(err, jc.ErrorIsNil)
}

type upgradeCharm struct {
	revision int
	forced   bool
}

func (s upgradeCharm) step(c *gc.C, ctx *context) {
	curl := curl(s.revision)
	sch, err := ctx.st.Charm(curl)
	c.Assert(err, jc.ErrorIsNil)
	err = ctx.svc.SetCharm(sch, s.forced)
	c.Assert(err, jc.ErrorIsNil)
	serveCharm{}.step(c, ctx)
}

type verifyCharm struct {
	revision          int
	attemptedRevision int
	checkFiles        ft.Entries
}

func (s verifyCharm) step(c *gc.C, ctx *context) {
	s.checkFiles.Check(c, filepath.Join(ctx.path, "charm"))
	path := filepath.Join(ctx.path, "charm", "revision")
	content, err := ioutil.ReadFile(path)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(string(content), gc.Equals, strconv.Itoa(s.revision))
	checkRevision := s.revision
	if s.attemptedRevision > checkRevision {
		checkRevision = s.attemptedRevision
	}
	err = ctx.unit.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	url, ok := ctx.unit.CharmURL()
	c.Assert(ok, jc.IsTrue)
	c.Assert(url, gc.DeepEquals, curl(checkRevision))
}

type startUpgradeError struct{}

func (s startUpgradeError) step(c *gc.C, ctx *context) {
	steps := []stepper{
		createCharm{
			customize: func(c *gc.C, ctx *context, path string) {
				appendHook(c, path, "start", "chmod 555 $CHARM_DIR")
			},
		},
		serveCharm{},
		createUniter{},
		waitUnit{
			status: params.StatusStarted,
		},
		waitHooks{"install", "config-changed", "start"},
		verifyCharm{},

		createCharm{revision: 1},
		serveCharm{},
		upgradeCharm{revision: 1},
		waitUnit{
			status: params.StatusError,
			info:   "upgrade failed",
			charm:  1,
		},
		verifyWaiting{},
		verifyCharm{attemptedRevision: 1},
	}
	for _, s_ := range steps {
		step(c, ctx, s_)
	}
}

type verifyWaitingUpgradeError struct {
	revision int
}

func (s verifyWaitingUpgradeError) step(c *gc.C, ctx *context) {
	verifyCharmSteps := []stepper{
		waitUnit{
			status: params.StatusError,
			info:   "upgrade failed",
			charm:  s.revision,
		},
		verifyCharm{attemptedRevision: s.revision},
	}
	verifyWaitingSteps := []stepper{
		stopUniter{},
		custom{func(c *gc.C, ctx *context) {
			// By setting status to Started, and waiting for the restarted uniter
			// to reset the error status, we can avoid a race in which a subsequent
			// fixUpgradeError lands just before the restarting uniter retries the
			// upgrade; and thus puts us in an unexpected state for future steps.
			ctx.unit.SetStatus(state.StatusStarted, "", nil)
		}},
		startUniter{},
	}
	allSteps := append(verifyCharmSteps, verifyWaitingSteps...)
	allSteps = append(allSteps, verifyCharmSteps...)
	for _, s_ := range allSteps {
		step(c, ctx, s_)
	}
}

type fixUpgradeError struct{}

func (s fixUpgradeError) step(c *gc.C, ctx *context) {
	charmPath := filepath.Join(ctx.path, "charm")
	err := os.Chmod(charmPath, 0755)
	c.Assert(err, jc.ErrorIsNil)
}

type addRelation struct {
	waitJoin bool
}

func (s addRelation) step(c *gc.C, ctx *context) {
	if ctx.relation != nil {
		panic("don't add two relations!")
	}
	if ctx.relatedSvc == nil {
		ctx.relatedSvc = ctx.s.AddTestingService(c, "mysql", ctx.s.AddTestingCharm(c, "mysql"))
	}
	eps, err := ctx.st.InferEndpoints("u", "mysql")
	c.Assert(err, jc.ErrorIsNil)
	ctx.relation, err = ctx.st.AddRelation(eps...)
	c.Assert(err, jc.ErrorIsNil)
	ctx.relationUnits = map[string]*state.RelationUnit{}
	if !s.waitJoin {
		return
	}

	// It's hard to do this properly (watching scope) without perturbing other tests.
	ru, err := ctx.relation.Unit(ctx.unit)
	c.Assert(err, jc.ErrorIsNil)
	timeout := time.After(worstCase)
	for {
		c.Logf("waiting to join relation")
		select {
		case <-timeout:
			c.Fatalf("failed to join relation")
		case <-time.After(coretesting.ShortWait):
			inScope, err := ru.InScope()
			c.Assert(err, jc.ErrorIsNil)
			if inScope {
				return
			}
		}
	}
}

type addRelationUnit struct{}

func (s addRelationUnit) step(c *gc.C, ctx *context) {
	u, err := ctx.relatedSvc.AddUnit()
	c.Assert(err, jc.ErrorIsNil)
	ru, err := ctx.relation.Unit(u)
	c.Assert(err, jc.ErrorIsNil)
	err = ru.EnterScope(nil)
	c.Assert(err, jc.ErrorIsNil)
	ctx.relationUnits[u.Name()] = ru
}

type changeRelationUnit struct {
	name string
}

func (s changeRelationUnit) step(c *gc.C, ctx *context) {
	settings, err := ctx.relationUnits[s.name].Settings()
	c.Assert(err, jc.ErrorIsNil)
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
	c.Assert(err, jc.ErrorIsNil)
}

type removeRelationUnit struct {
	name string
}

func (s removeRelationUnit) step(c *gc.C, ctx *context) {
	err := ctx.relationUnits[s.name].LeaveScope()
	c.Assert(err, jc.ErrorIsNil)
	ctx.relationUnits[s.name] = nil
}

type relationState struct {
	removed bool
	life    state.Life
}

func (s relationState) step(c *gc.C, ctx *context) {
	err := ctx.relation.Refresh()
	if s.removed {
		c.Assert(err, jc.Satisfies, errors.IsNotFound)
		return
	}
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ctx.relation.Life(), gc.Equals, s.life)

}

type addSubordinateRelation struct {
	ifce string
}

func (s addSubordinateRelation) step(c *gc.C, ctx *context) {
	if _, err := ctx.st.Service("logging"); errors.IsNotFound(err) {
		ctx.s.AddTestingService(c, "logging", ctx.s.AddTestingCharm(c, "logging"))
	}
	eps, err := ctx.st.InferEndpoints("logging", "u:"+s.ifce)
	c.Assert(err, jc.ErrorIsNil)
	_, err = ctx.st.AddRelation(eps...)
	c.Assert(err, jc.ErrorIsNil)
}

type removeSubordinateRelation struct {
	ifce string
}

func (s removeSubordinateRelation) step(c *gc.C, ctx *context) {
	eps, err := ctx.st.InferEndpoints("logging", "u:"+s.ifce)
	c.Assert(err, jc.ErrorIsNil)
	rel, err := ctx.st.EndpointsRelation(eps...)
	c.Assert(err, jc.ErrorIsNil)
	err = rel.Destroy()
	c.Assert(err, jc.ErrorIsNil)
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
			if errors.IsNotFound(err) {
				continue
			}
			c.Assert(err, jc.ErrorIsNil)
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
			c.Assert(err, jc.ErrorIsNil)
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
	c.Assert(err, jc.ErrorIsNil)
	err = ctx.subordinate.Remove()
	c.Assert(err, jc.ErrorIsNil)
	ctx.subordinate = nil
}

type assertYaml struct {
	path   string
	expect map[string]interface{}
}

func (s assertYaml) step(c *gc.C, ctx *context) {
	data, err := ioutil.ReadFile(filepath.Join(ctx.path, s.path))
	c.Assert(err, jc.ErrorIsNil)
	actual := make(map[string]interface{})
	err = goyaml.Unmarshal(data, &actual)
	c.Assert(err, jc.ErrorIsNil)
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
	c.Assert(err, jc.ErrorIsNil)
	err = ioutil.WriteFile(path, nil, s.mode)
	c.Assert(err, jc.ErrorIsNil)
}

type chmod struct {
	path string
	mode os.FileMode
}

func (s chmod) step(c *gc.C, ctx *context) {
	path := filepath.Join(ctx.path, s.path)
	err := os.Chmod(path, s.mode)
	c.Assert(err, jc.ErrorIsNil)
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

func curl(revision int) *corecharm.URL {
	return corecharm.MustParseURL("cs:quantal/wordpress").WithRevision(revision)
}

func appendHook(c *gc.C, charm, name, data string) {
	path := filepath.Join(charm, "hooks", name)
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0755)
	c.Assert(err, jc.ErrorIsNil)
	defer f.Close()
	_, err = f.Write([]byte(data))
	c.Assert(err, jc.ErrorIsNil)
}

func renameRelation(c *gc.C, charmPath, oldName, newName string) {
	path := filepath.Join(charmPath, "metadata.yaml")
	f, err := os.Open(path)
	c.Assert(err, jc.ErrorIsNil)
	defer f.Close()
	meta, err := corecharm.ReadMeta(f)
	c.Assert(err, jc.ErrorIsNil)

	replace := func(what map[string]corecharm.Relation) bool {
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
	c.Assert(err, jc.ErrorIsNil)
	ioutil.WriteFile(path, newmeta, 0644)

	f, err = os.Open(path)
	c.Assert(err, jc.ErrorIsNil)
	defer f.Close()
	meta, err = corecharm.ReadMeta(f)
	c.Assert(err, jc.ErrorIsNil)
}

func createHookLock(c *gc.C, dataDir string) *fslock.Lock {
	lockDir := filepath.Join(dataDir, "locks")
	lock, err := fslock.NewLock(lockDir, "uniter-hook-execution")
	c.Assert(err, jc.ErrorIsNil)
	return lock
}

type acquireHookSyncLock struct {
	message string
}

func (s acquireHookSyncLock) step(c *gc.C, ctx *context) {
	lock := createHookLock(c, ctx.dataDir)
	c.Assert(lock.IsLocked(), jc.IsFalse)
	err := lock.Lock(s.message)
	c.Assert(err, jc.ErrorIsNil)
}

var releaseHookSyncLock = custom{func(c *gc.C, ctx *context) {
	lock := createHookLock(c, ctx.dataDir)
	// Force the release.
	err := lock.BreakLock()
	c.Assert(err, jc.ErrorIsNil)
}}

var verifyHookSyncLockUnlocked = custom{func(c *gc.C, ctx *context) {
	lock := createHookLock(c, ctx.dataDir)
	c.Assert(lock.IsLocked(), jc.IsFalse)
}}

var verifyHookSyncLockLocked = custom{func(c *gc.C, ctx *context) {
	lock := createHookLock(c, ctx.dataDir)
	c.Assert(lock.IsLocked(), jc.IsTrue)
}}

type setProxySettings proxy.Settings

func (s setProxySettings) step(c *gc.C, ctx *context) {
	attrs := map[string]interface{}{
		"http-proxy":  s.Http,
		"https-proxy": s.Https,
		"ftp-proxy":   s.Ftp,
		"no-proxy":    s.NoProxy,
	}
	err := ctx.st.UpdateEnvironConfig(attrs, nil, nil)
	c.Assert(err, jc.ErrorIsNil)

	isExpectedEnvironment := func() bool {
		for name, expect := range map[string]string{
			"http_proxy":  s.Http,
			"HTTP_PROXY":  s.Http,
			"https_proxy": s.Https,
			"HTTPS_PROXY": s.Https,
			"ftp_proxy":   s.Ftp,
			"FTP_PROXY":   s.Ftp,
			"no_proxy":    s.NoProxy,
			"NO_PROXY":    s.NoProxy,
		} {
			if actual := os.Getenv(name); actual != expect {
				c.Logf("%s not yet set to %s (got %s)", name, expect, actual)
				return false
			}
		}
		return true
	}

	// wait for the new values...
	for attempt := coretesting.LongAttempt.Start(); attempt.Next(); {
		if isExpectedEnvironment() {
			return
		}
	}
	c.Fatal("settings didn't get noticed by the uniter")
}

type relationRunCommands []string

func (cmds relationRunCommands) step(c *gc.C, ctx *context) {
	commands := strings.Join(cmds, "\n")
	args := uniter.RunCommandsArgs{
		Commands:       commands,
		RelationId:     0,
		RemoteUnitName: "",
	}
	result, err := ctx.uniter.RunCommands(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(result.Code, gc.Equals, 0)
	c.Check(string(result.Stdout), gc.Equals, "")
	c.Check(string(result.Stderr), gc.Equals, "")
}

type runCommands []string

func (cmds runCommands) step(c *gc.C, ctx *context) {
	commands := strings.Join(cmds, "\n")
	args := uniter.RunCommandsArgs{
		Commands:       commands,
		RelationId:     -1,
		RemoteUnitName: "",
	}
	result, err := ctx.uniter.RunCommands(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(result.Code, gc.Equals, 0)
	c.Check(string(result.Stdout), gc.Equals, "")
	c.Check(string(result.Stderr), gc.Equals, "")
}

type asyncRunCommands []string

func (cmds asyncRunCommands) step(c *gc.C, ctx *context) {
	commands := strings.Join(cmds, "\n")
	args := uniter.RunCommandsArgs{
		Commands:       commands,
		RelationId:     -1,
		RemoteUnitName: "",
	}

	socketPath := filepath.Join(ctx.path, "run.socket")

	go func() {
		// make sure the socket exists
		client, err := rpc.Dial("unix", socketPath)
		c.Assert(err, jc.ErrorIsNil)
		defer client.Close()

		var result utilexec.ExecResponse
		err = client.Call(uniter.JujuRunEndpoint, args, &result)
		c.Assert(err, jc.ErrorIsNil)
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
	c.Assert(err, jc.ErrorIsNil)
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

// prepareGitUniter runs a sequence of uniter tests with the manifest deployer
// replacement logic patched out, simulating the effect of running an older
// version of juju that exclusively used a git deployer. This is useful both
// for testing the new deployer-replacement code *and* for running the old
// tests against the new, patched code to check that the tweaks made to
// accommodate the manifest deployer do not change the original behaviour as
// simulated by the patched-out code.
type prepareGitUniter struct {
	prepSteps []stepper
}

func (s prepareGitUniter) step(c *gc.C, ctx *context) {
	c.Assert(ctx.uniter, gc.IsNil, gc.Commentf("please don't try to patch stuff while the uniter's running"))
	newDeployer := func(charmPath, dataPath string, bundles charm.BundleReader) (charm.Deployer, error) {
		return charm.NewGitDeployer(charmPath, dataPath, bundles), nil
	}
	restoreNewDeployer := gt.PatchValue(&charm.NewDeployer, newDeployer)
	defer restoreNewDeployer()

	fixDeployer := func(deployer *charm.Deployer) error {
		return nil
	}
	restoreFixDeployer := gt.PatchValue(&charm.FixDeployer, fixDeployer)
	defer restoreFixDeployer()

	for _, prepStep := range s.prepSteps {
		step(c, ctx, prepStep)
	}
	if ctx.uniter != nil {
		step(c, ctx, stopUniter{})
	}
}

type CollectMetricsTimerSuite struct{}

var _ = gc.Suite(&CollectMetricsTimerSuite{})

func (*CollectMetricsTimerSuite) TestTimer(c *gc.C) {
	now := time.Now()
	defaultInterval := coretesting.ShortWait / 5
	testCases := []struct {
		about        string
		now          time.Time
		lastRun      time.Time
		interval     time.Duration
		expectSignal bool
	}{{
		"Timer firing after delay.",
		now,
		now.Add(-defaultInterval / 2),
		defaultInterval,
		true,
	}, {
		"Timer firing the first time.",
		now,
		time.Unix(0, 0),
		defaultInterval,
		true,
	}, {
		"Timer not firing soon.",
		now,
		now,
		coretesting.ShortWait * 2,
		false,
	}}

	for i, t := range testCases {
		c.Logf("running test %d", i)
		sig := uniter.CollectMetricsTimer(t.now, t.lastRun, t.interval)
		select {
		case <-sig:
			if !t.expectSignal {
				c.Errorf("not expecting a signal")
			}
		case <-time.After(coretesting.ShortWait):
			if t.expectSignal {
				c.Errorf("expected a signal")
			}
		}
	}
}
