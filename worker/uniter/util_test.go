// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter_test

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	pebbleclient "github.com/canonical/pebble/client"
	corecharm "github.com/juju/charm/v8"
	"github.com/juju/clock"
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/juju/api/secretsmanager"
	"github.com/juju/juju/core/secrets"
	"github.com/juju/loggo"
	"github.com/juju/mutex"
	"github.com/juju/names/v4"
	gt "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	ft "github.com/juju/testing/filetesting"
	"github.com/juju/utils/v2"
	"github.com/juju/worker/v3"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api"
	apiuniter "github.com/juju/juju/api/uniter"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/leadership"
	corelease "github.com/juju/juju/core/lease"
	"github.com/juju/juju/core/machinelock"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/core/watcher/watchertest"
	"github.com/juju/juju/juju/testing"
	"github.com/juju/juju/resource/resourcetesting"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/storage"
	"github.com/juju/juju/testcharms"
	coretesting "github.com/juju/juju/testing"
	jworker "github.com/juju/juju/worker"
	"github.com/juju/juju/worker/fortress"
	"github.com/juju/juju/worker/uniter"
	"github.com/juju/juju/worker/uniter/charm"
	"github.com/juju/juju/worker/uniter/operation"
	"github.com/juju/juju/worker/uniter/remotestate"
	"github.com/juju/juju/worker/uniter/runner"
	runnercontext "github.com/juju/juju/worker/uniter/runner/context"
)

var (
	// (achilleasa) 2019-10-11:
	// These addresses must always be IPs. If not, the facade code
	// (NetworksForRelation in particular) will attempt to resolve them and
	// cause the uniter tests to fail with an "unknown host" error.
	dummyPrivateAddress = network.NewSpaceAddress("172.0.30.1", network.WithScope(network.ScopeCloudLocal))
	dummyPublicAddress  = network.NewSpaceAddress("1.1.1.1", network.WithScope(network.ScopePublic))
)

// worstCase is used for timeouts when timing out
// will fail the test. Raising this value should
// not affect the overall running time of the tests
// unless they fail.
const worstCase = 100 * coretesting.LongWait

// Assign the unit to a provisioned machine with dummy addresses set.
func assertAssignUnit(c *gc.C, st *state.State, u *state.Unit) {
	err := u.AssignToNewMachine()
	c.Assert(err, jc.ErrorIsNil)
	mid, err := u.AssignedMachineId()
	c.Assert(err, jc.ErrorIsNil)
	machine, err := st.Machine(mid)
	c.Assert(err, jc.ErrorIsNil)
	err = machine.SetProvisioned("i-exist", "", "fake_nonce", nil)
	c.Assert(err, jc.ErrorIsNil)
	err = machine.SetProviderAddresses(dummyPrivateAddress, dummyPublicAddress)
	c.Assert(err, jc.ErrorIsNil)
}

// Assign the unit to a provisioned machine with dummy addresses set.
func assertAssignUnitLXDContainer(c *gc.C, st *state.State, u *state.Unit) {
	machine, err := st.AddMachineInsideNewMachine(
		state.MachineTemplate{
			Series: "quantal",
			Jobs:   []state.MachineJob{state.JobHostUnits},
		},
		state.MachineTemplate{ // parent
			Series: "quantal",
			Jobs:   []state.MachineJob{state.JobHostUnits},
		},
		instance.LXD,
	)
	c.Assert(err, jc.ErrorIsNil)
	err = u.AssignToMachine(machine)
	c.Assert(err, jc.ErrorIsNil)
	err = machine.SetProvisioned("i-exist", "", "fake_nonce", nil)
	c.Assert(err, jc.ErrorIsNil)
	err = machine.SetProviderAddresses(dummyPrivateAddress, dummyPublicAddress)
	c.Assert(err, jc.ErrorIsNil)
}

type testContext struct {
	uuid                   string
	path                   string
	dataDir                string
	s                      *UniterSuite
	st                     *state.State
	api                    *apiuniter.State
	apiConn                api.Connection
	leaseManager           corelease.Manager
	leaderTracker          *mockLeaderTracker
	charmDirGuard          *mockCharmDirGuard
	charms                 map[string][]byte
	hooks                  []string
	sch                    *state.Charm
	application            *state.Application
	unit                   *state.Unit
	uniter                 *uniter.Uniter
	relatedSvc             *state.Application
	relation               *state.Relation
	relationUnits          map[string]*state.RelationUnit
	subordinate            *state.Unit
	updateStatusHookTicker *manualTicker
	containerNames         []string
	pebbleClients          map[string]*fakePebbleClient
	secretsRotateCh        chan []string
	secretsFacade          *secretsmanager.Client
	err                    string

	wg             sync.WaitGroup
	mu             sync.Mutex
	hooksCompleted []string
	runner         *mockRunner
	deployer       *mockDeployer
}

var _ uniter.UniterExecutionObserver = (*testContext)(nil)

// HookCompleted implements the UniterExecutionObserver interface.
func (ctx *testContext) HookCompleted(hookName string) {
	ctx.mu.Lock()
	ctx.hooksCompleted = append(ctx.hooksCompleted, hookName)
	ctx.mu.Unlock()
}

// HookFailed implements the UniterExecutionObserver interface.
func (ctx *testContext) HookFailed(hookName string) {
	ctx.mu.Lock()
	ctx.hooksCompleted = append(ctx.hooksCompleted, "fail-"+hookName)
	ctx.mu.Unlock()
}

func (ctx *testContext) setExpectedError(err string) {
	ctx.mu.Lock()
	ctx.err = err
	ctx.mu.Unlock()
}

func (ctx *testContext) run(c *gc.C, steps []stepper) {
	defer func() {
		if ctx.uniter != nil {
			err := worker.Stop(ctx.uniter)
			if ctx.err == "" {
				if errors.Cause(err) == mutex.ErrCancelled {
					// This can happen if the uniter lock acquire was
					// temporarily blocked by test code holding the
					// lock (like in waitHooks). The acquire call is
					// delaying but then gets cancelled, and that
					// error bubbles up to here.
					// lp:1635664
					c.Logf("ignoring lock acquire cancelled by stop")
					return
				}
				c.Assert(err, jc.ErrorIsNil)
			} else {
				c.Assert(err, gc.ErrorMatches, ctx.err)
			}
		}
	}()
	for i, s := range steps {
		c.Logf("step %d:\n", i)
		step(c, ctx, s)
	}
}

func (ctx *testContext) apiLogin(c *gc.C) {
	password, err := utils.RandomPassword()
	c.Assert(err, jc.ErrorIsNil)
	err = ctx.unit.SetPassword(password)
	c.Assert(err, jc.ErrorIsNil)
	apiConn := ctx.s.OpenAPIAs(c, ctx.unit.Tag(), password)
	c.Assert(apiConn, gc.NotNil)
	c.Logf("API: login as %q successful", ctx.unit.Tag())
	testApi, err := apiConn.Uniter()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(testApi, gc.NotNil)
	ctx.api = testApi
	ctx.apiConn = apiConn
	ctx.leaderTracker = newMockLeaderTracker(ctx)
	ctx.leaderTracker.setLeader(c, true)
	ctx.secretsFacade = secretsmanager.NewClient(apiConn)
}

func (ctx *testContext) matchHooks(c *gc.C) (match, cannotMatch, overshoot bool) {
	ctx.mu.Lock()
	defer ctx.mu.Unlock()
	c.Logf("actual hooks: %#v", ctx.hooksCompleted)
	c.Logf("expected hooks: %#v", ctx.hooks)

	// If hooks are automatically retried, this may cause stutter in the actual observed
	// hooks depending on timing of the test steps. For the purposes of evaluating expected
	// hooks, the loop below skips over any retried, failed hooks
	// (up to the allowed retry limit for tests which is at most 2 in practice).

	const allowedHookRetryCount = 2

	previousFailedHook := ""
	retryCount := 0
	totalDuplicateFails := 0
	numCompletedHooks := len(ctx.hooksCompleted)
	numExpectedHooks := len(ctx.hooks)

	for hooksIndex := 0; hooksIndex < numExpectedHooks; {
		hooksCompletedIndex := hooksIndex + totalDuplicateFails
		if hooksCompletedIndex >= len(ctx.hooksCompleted) {
			// not all hooks have fired yet
			return false, false, false
		}
		completedHook := ctx.hooksCompleted[hooksCompletedIndex]
		if completedHook != ctx.hooks[hooksIndex] {
			if completedHook == previousFailedHook && retryCount < allowedHookRetryCount {
				retryCount++
				totalDuplicateFails++
				continue
			}
			cannotMatch = true
			return false, cannotMatch, false
		}
		hooksIndex++
		if strings.HasPrefix(completedHook, "fail-") {
			previousFailedHook = completedHook
		} else {
			retryCount = 0
			previousFailedHook = ""
		}
	}

	// Ensure any duplicate hook failures at the end of the sequence are counted.
	for i := 0; i < numCompletedHooks-numExpectedHooks; i++ {
		if ctx.hooksCompleted[numExpectedHooks+i] != previousFailedHook {
			break
		}
		totalDuplicateFails++
	}
	return true, false, numCompletedHooks > numExpectedHooks+totalDuplicateFails
}

type uniterTest struct {
	summary string
	steps   []stepper
}

func ut(summary string, steps ...stepper) uniterTest {
	return uniterTest{summary, steps}
}

type stepper interface {
	step(c *gc.C, ctx *testContext)
}

func step(c *gc.C, ctx *testContext, s stepper) {
	c.Logf("%#v", s)
	s.step(c, ctx)
}

type ensureStateWorker struct{}

func (s ensureStateWorker) step(c *gc.C, ctx *testContext) {
	addresses, err := ctx.st.Addresses()
	if err != nil || len(addresses) == 0 {
		addControllerMachine(c, ctx.st)
	}
}

func addControllerMachine(c *gc.C, st *state.State) {
	// The AddControllerMachine call will update the API host ports
	// to made-up addresses. We need valid addresses so that the uniter
	// can download charms from the API server.
	apiHostPorts, err := st.APIHostPortsForClients()
	c.Assert(err, gc.IsNil)
	testing.AddControllerMachine(c, st)
	err = st.SetAPIHostPorts(apiHostPorts)
	c.Assert(err, gc.IsNil)
}

type createCharm struct {
	revision  int
	badHooks  []string
	customize func(*gc.C, *testContext, string)
}

func startupHooks(minion bool) []string {
	leaderHook := "leader-elected"
	if minion {
		leaderHook = "leader-settings-changed"
	}
	return []string{"install", leaderHook, "config-changed", "start"}
}

func (s createCharm) step(c *gc.C, ctx *testContext) {
	base := testcharms.Repo.ClonedDirPath(c.MkDir(), "wordpress")
	if s.customize != nil {
		s.customize(c, ctx, base)
	}
	if len(s.badHooks) > 0 {
		ctx.runner.hooksWithErrors = set.NewStrings(s.badHooks...)
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

func (s addCharm) step(c *gc.C, ctx *testContext) {
	var buf bytes.Buffer
	err := s.dir.ArchiveTo(&buf)
	c.Assert(err, jc.ErrorIsNil)
	body := buf.Bytes()
	hash, _, err := utils.ReadSHA256(&buf)
	c.Assert(err, jc.ErrorIsNil)

	storagePath := fmt.Sprintf("/charms/%s/%d", s.dir.Meta().Name, s.dir.Revision())
	ctx.charms[storagePath] = body
	info := state.CharmInfo{
		Charm:       s.dir,
		ID:          s.curl,
		StoragePath: storagePath,
		SHA256:      hash,
	}

	ctx.sch, err = ctx.st.AddCharm(info)
	c.Assert(err, jc.ErrorIsNil)
}

type serveCharm struct{}

func (s serveCharm) step(c *gc.C, ctx *testContext) {
	testStorage := storage.NewStorage(ctx.st.ModelUUID(), ctx.st.MongoSession())
	for storagePath, data := range ctx.charms {
		err := testStorage.Put(storagePath, bytes.NewReader(data), int64(len(data)))
		c.Assert(err, jc.ErrorIsNil)
		delete(ctx.charms, storagePath)
	}
}

type addCharmProfileToMachine struct {
	profiles []string
}

func (acpm addCharmProfileToMachine) step(c *gc.C, ctx *testContext) {
	machineId, err := ctx.unit.AssignedMachineId()
	c.Assert(err, jc.ErrorIsNil)
	machine, err := ctx.st.Machine(machineId)
	c.Assert(err, jc.ErrorIsNil)
	err = machine.SetCharmProfiles(acpm.profiles)
	c.Assert(err, jc.ErrorIsNil)
}

type createApplicationAndUnit struct {
	applicationName string
	storage         map[string]state.StorageConstraints
	container       bool
}

func (csau createApplicationAndUnit) step(c *gc.C, ctx *testContext) {
	if csau.applicationName == "" {
		csau.applicationName = "u"
	}
	sch, err := ctx.st.Charm(curl(0))
	c.Assert(err, jc.ErrorIsNil)
	app := ctx.s.AddTestingApplicationWithStorage(c, csau.applicationName, sch, csau.storage)
	unit, err := app.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	err = unit.SetCharmURL(curl(0))
	c.Assert(err, jc.ErrorIsNil)

	// Assign the unit to a provisioned machine to match expected state.
	if csau.container {
		assertAssignUnitLXDContainer(c, ctx.st, unit)
	} else {
		assertAssignUnit(c, ctx.st, unit)
	}

	ctx.application = app
	ctx.unit = unit

	ctx.apiLogin(c)
}

type deleteUnit struct{}

func (d deleteUnit) step(c *gc.C, ctx *testContext) {
	ctx.unit.DestroyWithForce(true, time.Duration(0))
}

type createUniter struct {
	minion               bool
	executorFunc         uniter.NewOperationExecutorFunc
	translateResolverErr func(error) error
}

func (s createUniter) step(c *gc.C, ctx *testContext) {
	step(c, ctx, ensureStateWorker{})
	step(c, ctx, createApplicationAndUnit{})
	if s.minion {
		step(c, ctx, forceMinion{})
	}
	step(c, ctx, startUniter{
		newExecutorFunc:      s.executorFunc,
		translateResolverErr: s.translateResolverErr,
		unitTag:              ctx.unit.Tag().String(),
	})
	step(c, ctx, waitAddresses{})
}

type waitAddresses struct{}

func (waitAddresses) step(c *gc.C, ctx *testContext) {
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
			if private.Value != dummyPrivateAddress.Value {
				continue
			}
			public, _ := ctx.unit.PublicAddress()
			if public.Value != dummyPublicAddress.Value {
				continue
			}
			return
		}
	}
}

type startUniter struct {
	unitTag              string
	newExecutorFunc      uniter.NewOperationExecutorFunc
	translateResolverErr func(error) error
	rebootQuerier        uniter.RebootQuerier
}

type fakeRebootQuerier struct {
	rebootDetected bool
}

func (q fakeRebootQuerier) Query(names.Tag) (bool, error) {
	return q.rebootDetected, nil
}

type fakeRebootQuerierTrueOnce struct {
	times  int
	result map[int]bool
}

func (q *fakeRebootQuerierTrueOnce) Query(_ names.Tag) (bool, error) {
	retVal := q.result[q.times]
	q.times += 1
	return retVal, nil
}

// mimicRealRebootQuerier returns a reboot querier which mimics
// the behavior of the uniter without a reboot.
func mimicRealRebootQuerier() uniter.RebootQuerier {
	return &fakeRebootQuerierTrueOnce{result: map[int]bool{0: rebootDetected, 1: rebootNotDetected, 2: rebootNotDetected}}
}

func (s startUniter) step(c *gc.C, ctx *testContext) {
	if s.unitTag == "" {
		s.unitTag = "unit-u-0"
	}
	if ctx.uniter != nil {
		panic("don't start two uniters!")
	}
	if ctx.api == nil {
		panic("API connection not established")
	}
	if ctx.runner == nil {
		panic("process runner not set up")
	}
	if ctx.runner == nil {
		panic("deployer not set up")
	}
	if s.rebootQuerier == nil {
		s.rebootQuerier = mimicRealRebootQuerier()
	}
	tag, err := names.ParseUnitTag(s.unitTag)
	if err != nil {
		panic(err.Error())
	}
	downloader := api.NewCharmDownloader(ctx.apiConn)
	operationExecutor := operation.NewExecutor
	if s.newExecutorFunc != nil {
		operationExecutor = s.newExecutorFunc
	}

	uniterParams := uniter.UniterParams{
		UniterFacade: ctx.api,
		UnitTag:      tag,
		ModelType:    model.IAAS,
		LeadershipTrackerFunc: func(_ names.UnitTag) leadership.TrackerWorker {
			return ctx.leaderTracker
		},
		CharmDirGuard:        ctx.charmDirGuard,
		DataDir:              ctx.dataDir,
		Downloader:           downloader,
		MachineLock:          processLock,
		UpdateStatusSignal:   ctx.updateStatusHookTicker.ReturnTimer(),
		NewOperationExecutor: operationExecutor,
		NewProcessRunner: func(context runnercontext.Context, paths runnercontext.Paths, remoteExecutor runner.ExecFunc) runner.Runner {
			ctx.runner.ctx = context
			return ctx.runner
		},
		NewDeployer: func(charmPath, dataPath string, bundles charm.BundleReader, logger charm.Logger) (charm.Deployer, error) {
			ctx.deployer.charmPath = charmPath
			ctx.deployer.dataPath = dataPath
			ctx.deployer.bundles = bundles
			return ctx.deployer, nil
		},
		TranslateResolverErr: s.translateResolverErr,
		Observer:             ctx,
		// TODO(axw) 2015-11-02 #1512191
		// update tests that rely on timing to advance clock
		// appropriately.
		Clock:          clock.WallClock,
		RebootQuerier:  s.rebootQuerier,
		Logger:         loggo.GetLogger("test"),
		ContainerNames: ctx.containerNames,
		NewPebbleClient: func(cfg *pebbleclient.Config) (uniter.PebbleClient, error) {
			res := pebbleSocketPathRegexp.FindAllStringSubmatch(cfg.Socket, 1)
			if res == nil {
				return nil, errors.NotFoundf("container")
			}
			client, ok := ctx.pebbleClients[res[0][1]]
			if !ok {
				return nil, errors.NotFoundf("container")
			}
			return client, nil
		},
		SecretRotateWatcherFunc: func(u names.UnitTag, secretsChanged chan []string) (worker.Worker, error) {
			c.Assert(u.String(), gc.Equals, s.unitTag)
			ctx.secretsRotateCh = secretsChanged
			return watchertest.NewMockStringsWatcher(ctx.secretsRotateCh), nil
		},
		SecretsFacade: ctx.secretsFacade,
	}
	ctx.uniter, err = uniter.NewUniter(&uniterParams)
	c.Assert(err, jc.ErrorIsNil)
}

type waitUniterDead struct {
	err string
}

func (s waitUniterDead) step(c *gc.C, ctx *testContext) {
	if s.err != "" {
		err := s.waitDead(c, ctx)
		c.Log(errors.ErrorStack(err))
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
	if err != jworker.ErrTerminateAgent {
		step(c, ctx, startUniter{})
		err = s.waitDead(c, ctx)
	}
	c.Assert(err, gc.Equals, jworker.ErrTerminateAgent)
	err = ctx.unit.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ctx.unit.Life(), gc.Equals, state.Dead)
}

func (s waitUniterDead) waitDead(c *gc.C, ctx *testContext) error {
	u := ctx.uniter
	ctx.uniter = nil

	wait := make(chan error, 1)
	go func() {
		wait <- u.Wait()
	}()

	ctx.s.BackingState.StartSync()
	select {
	case err := <-wait:
		return err
	case <-time.After(worstCase):
		u.Kill()
		c.Fatalf("uniter still alive")
	}
	panic("unreachable")
}

type stopUniter struct {
	err string
}

func (s stopUniter) step(c *gc.C, ctx *testContext) {
	u := ctx.uniter
	if u == nil {
		c.Logf("uniter not started, skipping stopUniter{}")
		return
	}
	ctx.uniter = nil
	err := worker.Stop(u)
	if s.err == "" {
		c.Assert(err, jc.ErrorIsNil)
	} else {
		c.Assert(err, gc.ErrorMatches, s.err)
	}
}

type verifyWaiting struct{}

func (s verifyWaiting) step(c *gc.C, ctx *testContext) {
	step(c, ctx, stopUniter{})
	step(c, ctx, startUniter{rebootQuerier: fakeRebootQuerier{rebootNotDetected}})
	step(c, ctx, waitHooks{})
}

type verifyRunning struct {
	minion bool
}

func (s verifyRunning) step(c *gc.C, ctx *testContext) {
	step(c, ctx, stopUniter{})
	step(c, ctx, startUniter{rebootQuerier: fakeRebootQuerier{rebootNotDetected}})
	var hooks []string
	if s.minion {
		hooks = append(hooks, "leader-settings-changed")
	}
	// We don't expect config-changed to always run on agent restart
	// anymore.
	step(c, ctx, waitHooks(hooks))
}

type startupError struct {
	badHook string
}

func (s startupError) step(c *gc.C, ctx *testContext) {
	step(c, ctx, createCharm{badHooks: []string{s.badHook}})
	step(c, ctx, serveCharm{})
	step(c, ctx, createUniter{})
	step(c, ctx, waitUnitAgent{
		statusGetter: unitStatusGetter,
		status:       status.Error,
		info:         fmt.Sprintf(`hook failed: %q`, s.badHook),
	})
	for _, hook := range startupHooks(false) {
		if hook == s.badHook {
			step(c, ctx, waitHooks{"fail-" + hook})
			break
		}
		step(c, ctx, waitHooks{hook})
	}
	step(c, ctx, verifyCharm{})
}

type verifyDeployed struct{}

func (s verifyDeployed) step(c *gc.C, ctx *testContext) {
	c.Assert(ctx.deployer.curl, jc.DeepEquals, curl(0))
	c.Assert(ctx.deployer.deployed, jc.IsTrue)
}

type quickStart struct {
	minion bool
}

func (s quickStart) step(c *gc.C, ctx *testContext) {
	step(c, ctx, createCharm{})
	step(c, ctx, serveCharm{})
	step(c, ctx, createUniter{minion: s.minion})
	step(c, ctx, waitUnitAgent{status: status.Idle})
	step(c, ctx, waitHooks(startupHooks(s.minion)))
	step(c, ctx, verifyCharm{})
}

type quickStartRelation struct{}

func (s quickStartRelation) step(c *gc.C, ctx *testContext) {
	step(c, ctx, quickStart{})
	step(c, ctx, addRelation{})
	step(c, ctx, addRelationUnit{})
	step(c, ctx, waitHooks{"db-relation-joined mysql/0 db:0", "db-relation-changed mysql/0 db:0"})
	step(c, ctx, verifyRunning{})
}

type startupRelationError struct {
	badHook string
}

func (s startupRelationError) step(c *gc.C, ctx *testContext) {
	step(c, ctx, createCharm{badHooks: []string{s.badHook}})
	step(c, ctx, serveCharm{})
	step(c, ctx, createUniter{})
	step(c, ctx, waitUnitAgent{status: status.Idle})
	step(c, ctx, waitHooks(startupHooks(false)))
	step(c, ctx, verifyCharm{})
	step(c, ctx, addRelation{})
	step(c, ctx, addRelationUnit{})
}

type resolveError struct {
	resolved state.ResolvedMode
}

func (s resolveError) step(c *gc.C, ctx *testContext) {
	err := ctx.unit.SetResolved(s.resolved)
	c.Assert(err, jc.ErrorIsNil)
}

type statusfunc func() (status.StatusInfo, error)

var unitStatusGetter = func(ctx *testContext) statusfunc {
	return func() (status.StatusInfo, error) {
		return ctx.unit.Status()
	}
}

var agentStatusGetter = func(ctx *testContext) statusfunc {
	return func() (status.StatusInfo, error) {
		return ctx.unit.AgentStatus()
	}
}

type waitUnitAgent struct {
	statusGetter func(ctx *testContext) statusfunc
	status       status.Status
	info         string
	data         map[string]interface{}
	charm        int
	resolved     state.ResolvedMode
}

func (s waitUnitAgent) step(c *gc.C, ctx *testContext) {
	if s.statusGetter == nil {
		s.statusGetter = agentStatusGetter
	}
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
			statusInfo, err := s.statusGetter(ctx)()
			c.Assert(err, jc.ErrorIsNil)
			if string(statusInfo.Status) != string(s.status) {
				c.Logf("want unit status %q, got %q; still waiting", s.status, statusInfo.Status)
				continue
			}
			if statusInfo.Message != s.info {
				c.Logf("want unit status info %q, got %q; still waiting", s.info, statusInfo.Message)
				continue
			}
			if s.data != nil {
				if len(statusInfo.Data) != len(s.data) {
					wantKeys := []string{}
					for k := range s.data {
						wantKeys = append(wantKeys, k)
					}
					sort.Strings(wantKeys)
					gotKeys := []string{}
					for k := range statusInfo.Data {
						gotKeys = append(gotKeys, k)
					}
					sort.Strings(gotKeys)
					c.Logf("want {%s} status data value(s), got {%s}; still waiting", strings.Join(wantKeys, ", "), strings.Join(gotKeys, ", "))
					continue
				}
				for key, value := range s.data {
					if statusInfo.Data[key] != value {
						c.Logf("want status data value %q for key %q, got %q; still waiting",
							value, key, statusInfo.Data[key])
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

func (s waitHooks) step(c *gc.C, ctx *testContext) {
	if len(s) == 0 {
		// Give unwanted hooks a moment to run...
		ctx.s.BackingState.StartSync()
		time.Sleep(coretesting.ShortWait)
	}
	ctx.hooks = append(ctx.hooks, s...)
	c.Logf("waiting for hooks: %#v", ctx.hooks)
	match, cannotMatch, overshoot := ctx.matchHooks(c)
	if overshoot && len(s) == 0 {
		c.Fatalf("ran more hooks than expected")
	}
	if cannotMatch {
		c.Fatalf("hooks did not match expected")
	}
	waitExecutionLockReleased := func() {
		timeout := make(chan struct{})
		go func() {
			<-time.After(worstCase)
			close(timeout)
		}()
		releaser, err := processLock.Acquire(machinelock.Spec{
			Worker:  "uniter-test",
			Comment: "waitHooks",
			Cancel:  timeout,
		})
		if err != nil {
			c.Fatalf("failed to acquire execution lock: %v", err)
		}
		releaser()
	}
	if match {
		if len(s) > 0 {
			// only check for lock release if there were hooks
			// run; hooks *not* running may be due to the lock
			// being held.
			waitExecutionLockReleased()
		}
		return
	}
	timeout := time.After(worstCase)
	for {
		ctx.s.BackingState.StartSync()
		select {
		case <-time.After(coretesting.ShortWait):
			if match, cannotMatch, _ = ctx.matchHooks(c); match {
				waitExecutionLockReleased()
				return
			} else if cannotMatch {
				c.Fatalf("unexpected hook triggered")
			}
		case <-timeout:
			c.Fatalf("never got expected hooks")
		}
	}
}

type actionData struct {
	actionName string
	args       []string
}

type waitActionInvocation struct {
	expectedActions []actionData
}

func (s waitActionInvocation) step(c *gc.C, ctx *testContext) {
	timeout := time.After(worstCase)
	for {
		select {
		case <-time.After(coretesting.ShortWait):
			ranActions := ctx.runner.ranActions()
			if len(ranActions) != len(s.expectedActions) {
				continue
			}
			assertActionsMatch(c, ranActions, s.expectedActions)
			return
		case <-timeout:
			c.Fatalf("timed out waiting for action invocation")
		}
	}
}

func assertActionsMatch(c *gc.C, actualIn []actionData, expectIn []actionData) {
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

type fixHook struct {
	name string
}

func (s fixHook) step(_ *gc.C, ctx *testContext) {
	if ctx.runner.hooksWithErrors != nil {
		ctx.runner.hooksWithErrors.Remove(s.name)
	}
}

type updateStatusHookTick struct{}

func (s updateStatusHookTick) step(c *gc.C, ctx *testContext) {
	err := ctx.updateStatusHookTicker.Tick()
	c.Assert(err, jc.ErrorIsNil)
}

type changeConfig map[string]interface{}

func (s changeConfig) step(c *gc.C, ctx *testContext) {
	err := ctx.application.UpdateCharmConfig(model.GenerationMaster, corecharm.Settings(s))
	c.Assert(err, jc.ErrorIsNil)
}

type addAction struct {
	name   string
	params map[string]interface{}
}

func (s addAction) step(c *gc.C, ctx *testContext) {
	m, err := ctx.st.Model()
	c.Assert(err, jc.ErrorIsNil)
	operationID, err := m.EnqueueOperation("a test", 1)
	c.Assert(err, jc.ErrorIsNil)
	_, err = m.EnqueueAction(operationID, ctx.unit.Tag(), s.name, s.params, nil)
	c.Assert(err, jc.ErrorIsNil)
}

type upgradeCharm struct {
	revision int
	forced   bool
}

func (s upgradeCharm) step(c *gc.C, ctx *testContext) {
	curl := curl(s.revision)
	sch, err := ctx.st.Charm(curl)
	c.Assert(err, jc.ErrorIsNil)
	cfg := state.SetCharmConfig{
		Charm:      sch,
		ForceUnits: s.forced,
	}
	// Make sure we upload the charm before changing it in the DB.
	serveCharm{}.step(c, ctx)
	err = ctx.application.SetCharm(cfg)
	c.Assert(err, jc.ErrorIsNil)
}

type verifyCharm struct {
	revision          int
	attemptedRevision int
	checkFiles        ft.Entries
}

func (s verifyCharm) step(c *gc.C, ctx *testContext) {
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

type pushResource struct{}

func (s pushResource) step(c *gc.C, ctx *testContext) {
	opened := resourcetesting.NewResource(c, &gt.Stub{}, "data", ctx.unit.ApplicationName(), "the bytes")

	res, err := ctx.st.Resources()
	c.Assert(err, jc.ErrorIsNil)
	_, err = res.SetResource(
		ctx.unit.ApplicationName(),
		opened.Username,
		opened.Resource.Resource,
		opened.ReadCloser,
		state.IncrementCharmModifiedVersion,
	)
	c.Assert(err, jc.ErrorIsNil)
}

type startUpgradeError struct{}

func (s startUpgradeError) step(c *gc.C, ctx *testContext) {
	steps := []stepper{
		createCharm{},
		serveCharm{},
		createUniter{},
		waitUnitAgent{
			status: status.Idle,
		},
		waitHooks(startupHooks(false)),
		verifyCharm{},

		createCharm{
			revision: 1,
			customize: func(c *gc.C, ctx *testContext, path string) {
				ctx.deployer.err = charm.ErrConflict
			},
		},
		serveCharm{},
		upgradeCharm{revision: 1},
		waitUnitAgent{
			statusGetter: unitStatusGetter,
			status:       status.Error,
			info:         "upgrade failed",
			charm:        1,
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

func (s verifyWaitingUpgradeError) step(c *gc.C, ctx *testContext) {
	verifyCharmSteps := []stepper{
		waitUnitAgent{
			statusGetter: unitStatusGetter,
			status:       status.Error,
			info:         "upgrade failed",
			charm:        s.revision,
		},
		verifyCharm{attemptedRevision: s.revision},
	}
	verifyWaitingSteps := []stepper{
		stopUniter{},
		custom{func(c *gc.C, ctx *testContext) {
			// By setting status to Idle, and waiting for the restarted uniter
			// to reset the error status, we can avoid a race in which a subsequent
			// fixUpgradeError lands just before the restarting uniter retries the
			// upgrade; and thus puts us in an unexpected state for future steps.
			now := time.Now()
			sInfo := status.StatusInfo{
				Status:  status.Idle,
				Message: "",
				Since:   &now,
			}
			err := ctx.unit.SetAgentStatus(sInfo)
			c.Check(err, jc.ErrorIsNil)
		}},
		startUniter{rebootQuerier: &fakeRebootQuerier{rebootNotDetected}},
	}
	allSteps := append(verifyCharmSteps, verifyWaitingSteps...)
	allSteps = append(allSteps, verifyCharmSteps...)
	for _, s_ := range allSteps {
		step(c, ctx, s_)
	}
}

type fixUpgradeError struct{}

func (s fixUpgradeError) step(_ *gc.C, ctx *testContext) {
	ctx.deployer.err = nil
}

type addRelation struct {
	waitJoin bool
}

func (s addRelation) step(c *gc.C, ctx *testContext) {
	if ctx.relation != nil {
		panic("don't add two relations!")
	}
	if ctx.relatedSvc == nil {
		ctx.relatedSvc = ctx.s.AddTestingApplication(c, "mysql", ctx.s.AddTestingCharm(c, "mysql"))
	}
	eps, err := ctx.st.InferEndpoints(ctx.application.Name(), "mysql")
	c.Assert(err, jc.ErrorIsNil)
	ctx.relation, err = ctx.st.AddRelation(eps...)
	c.Assert(err, jc.ErrorIsNil)
	ctx.relationUnits = map[string]*state.RelationUnit{}
	step(c, ctx, waitHooks{"db-relation-created mysql db:0"})
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

func (s addRelationUnit) step(c *gc.C, ctx *testContext) {
	u, err := ctx.relatedSvc.AddUnit(state.AddUnitParams{})
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

func (s changeRelationUnit) step(c *gc.C, ctx *testContext) {
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

func (s removeRelationUnit) step(c *gc.C, ctx *testContext) {
	err := ctx.relationUnits[s.name].LeaveScope()
	c.Assert(err, jc.ErrorIsNil)
	ctx.relationUnits[s.name] = nil
}

type relationState struct {
	removed bool
	life    state.Life
}

func (s relationState) step(c *gc.C, ctx *testContext) {
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

func (s addSubordinateRelation) step(c *gc.C, ctx *testContext) {
	if _, err := ctx.st.Application("logging"); errors.IsNotFound(err) {
		ctx.s.AddTestingApplication(c, "logging", ctx.s.AddTestingCharm(c, "logging"))
	}
	eps, err := ctx.st.InferEndpoints("logging", "u:"+s.ifce)
	c.Assert(err, jc.ErrorIsNil)
	_, err = ctx.st.AddRelation(eps...)
	c.Assert(err, jc.ErrorIsNil)
}

type removeSubordinateRelation struct {
	ifce string
}

func (s removeSubordinateRelation) step(c *gc.C, ctx *testContext) {
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

func (s waitSubordinateExists) step(c *gc.C, ctx *testContext) {
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

func (waitSubordinateDying) step(c *gc.C, ctx *testContext) {
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

func (removeSubordinate) step(c *gc.C, ctx *testContext) {
	err := ctx.subordinate.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
	err = ctx.subordinate.Remove()
	c.Assert(err, jc.ErrorIsNil)
	ctx.subordinate = nil
}

type writeFile struct {
	path string
	mode os.FileMode
}

func (s writeFile) step(c *gc.C, ctx *testContext) {
	path := filepath.Join(ctx.path, s.path)
	dir := filepath.Dir(path)
	err := os.MkdirAll(dir, 0755)
	c.Assert(err, jc.ErrorIsNil)
	err = ioutil.WriteFile(path, nil, s.mode)
	c.Assert(err, jc.ErrorIsNil)
}

type removeCharmDir struct{}

func (s removeCharmDir) step(c *gc.C, ctx *testContext) {
	path := filepath.Join(ctx.path, "charm")
	err := os.RemoveAll(path)
	c.Assert(err, jc.ErrorIsNil)
}

type custom struct {
	f func(*gc.C, *testContext)
}

func (s custom) step(c *gc.C, ctx *testContext) {
	s.f(c, ctx)
}

var relationDying = custom{func(c *gc.C, ctx *testContext) {
	c.Check(ctx.relation.Refresh(), gc.IsNil)
	c.Assert(ctx.relation.Destroy(), gc.IsNil)
}}

var unitDying = custom{func(c *gc.C, ctx *testContext) {
	c.Assert(ctx.unit.Destroy(), gc.IsNil)
}}

var unitDead = custom{func(c *gc.C, ctx *testContext) {
	c.Assert(ctx.unit.EnsureDead(), gc.IsNil)
}}

var subordinateDying = custom{func(c *gc.C, ctx *testContext) {
	c.Assert(ctx.subordinate.Destroy(), gc.IsNil)
}}

func curl(revision int) *corecharm.URL {
	return corecharm.MustParseURL("cs:quantal/wordpress").WithRevision(revision)
}

type hookLock struct {
	releaser func()
}

type hookStep struct {
	stepFunc func(*gc.C, *testContext)
}

func (h *hookStep) step(c *gc.C, ctx *testContext) {
	h.stepFunc(c, ctx)
}

func (h *hookLock) acquire() *hookStep {
	return &hookStep{stepFunc: func(c *gc.C, ctx *testContext) {
		releaser, err := processLock.Acquire(machinelock.Spec{
			Worker:  "uniter-test",
			Comment: "hookLock",
			Cancel:  make(chan struct{}), // clearly suboptimal
		})
		c.Assert(err, jc.ErrorIsNil)
		h.releaser = releaser
	}}
}

func (h *hookLock) release() *hookStep {
	return &hookStep{stepFunc: func(c *gc.C, ctx *testContext) {
		c.Assert(h.releaser, gc.NotNil)
		h.releaser()
		h.releaser = nil
	}}
}

type runCommands []string

func (cmds runCommands) step(c *gc.C, ctx *testContext) {
	commands := strings.Join(cmds, "\n")
	args := uniter.RunCommandsArgs{
		Commands:       commands,
		RelationId:     -1,
		RemoteUnitName: "",
		UnitName:       "u/0",
	}
	result, err := ctx.uniter.RunCommands(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(result.Code, gc.Equals, 0)
	c.Check(string(result.Stdout), gc.Equals, "test on workload")
	c.Check(string(result.Stderr), gc.Equals, "")
}

type forceMinion struct{}

func (forceMinion) step(c *gc.C, ctx *testContext) {
	ctx.leaderTracker.setLeader(c, false)
}

type forceLeader struct{}

func (forceLeader) step(c *gc.C, ctx *testContext) {
	ctx.leaderTracker.setLeader(c, true)
}

func newMockLeaderTracker(ctx *testContext) *mockLeaderTracker {
	return &mockLeaderTracker{
		ctx: ctx,
	}
}

type mockLeaderTracker struct {
	mu       sync.Mutex
	ctx      *testContext
	isLeader bool
	waiting  []chan struct{}
}

func (mock *mockLeaderTracker) Kill() {
	return
}

func (mock *mockLeaderTracker) Wait() error {
	return nil
}

func (mock *mockLeaderTracker) ApplicationName() string {
	return mock.ctx.application.Name()
}

func (mock *mockLeaderTracker) ClaimDuration() time.Duration {
	return 30 * time.Second
}

func (mock *mockLeaderTracker) ClaimLeader() leadership.Ticket {
	mock.mu.Lock()
	defer mock.mu.Unlock()
	if mock.isLeader {
		return fastTicket{true}
	}
	return fastTicket{}
}

func (mock *mockLeaderTracker) WaitLeader() leadership.Ticket {
	mock.mu.Lock()
	defer mock.mu.Unlock()
	if mock.isLeader {
		return fastTicket{}
	}
	return mock.waitTicket()
}

func (mock *mockLeaderTracker) WaitMinion() leadership.Ticket {
	mock.mu.Lock()
	defer mock.mu.Unlock()
	if !mock.isLeader {
		return fastTicket{}
	}
	return mock.waitTicket()
}

func (mock *mockLeaderTracker) waitTicket() leadership.Ticket {
	// very internal, expects mu to be locked already
	ch := make(chan struct{})
	mock.waiting = append(mock.waiting, ch)
	return waitTicket{ch}
}

func (mock *mockLeaderTracker) setLeader(c *gc.C, isLeader bool) {
	mock.mu.Lock()
	defer mock.mu.Unlock()
	if mock.isLeader == isLeader {
		return
	}
	if isLeader {
		claimer, err := mock.ctx.leaseManager.Claimer("application-leadership", mock.ctx.st.ModelUUID())
		c.Assert(err, jc.ErrorIsNil)
		err = claimer.Claim(
			mock.ctx.application.Name(), mock.ctx.unit.Name(), time.Minute,
		)
		c.Assert(err, jc.ErrorIsNil)
	} else {
		time.Sleep(coretesting.ShortWait)
	}
	mock.isLeader = isLeader
	for _, ch := range mock.waiting {
		close(ch)
	}
	mock.waiting = nil
}

type waitTicket struct {
	ch chan struct{}
}

func (t waitTicket) Ready() <-chan struct{} {
	return t.ch
}

func (t waitTicket) Wait() bool {
	return false
}

type fastTicket struct {
	value bool
}

func (fastTicket) Ready() <-chan struct{} {
	ch := make(chan struct{})
	close(ch)
	return ch
}

func (t fastTicket) Wait() bool {
	return t.value
}

type setLeaderSettings map[string]string

func (s setLeaderSettings) step(c *gc.C, ctx *testContext) {
	// We do this directly on State, not the API, so we don't have to worry
	// about getting an API conn for whatever unit's meant to be leader.
	err := ctx.application.UpdateLeaderSettings(successToken{}, s)
	c.Assert(err, jc.ErrorIsNil)
	ctx.s.BackingState.StartSync()
}

type successToken struct{}

func (successToken) Check(int, interface{}) error {
	return nil
}

type mockCharmDirGuard struct{}

// Unlock implements fortress.Guard.
func (*mockCharmDirGuard) Unlock() error { return nil }

// Lockdown implements fortress.Guard.
func (*mockCharmDirGuard) Lockdown(_ fortress.Abort) error { return nil }

type mockRotateSecretsWatcher struct{}

func (w *mockRotateSecretsWatcher) Kill() {}

func (*mockRotateSecretsWatcher) Wait() error { return nil }

type provisionStorage struct{}

func (s provisionStorage) step(c *gc.C, ctx *testContext) {
	sb, err := state.NewStorageBackend(ctx.st)
	c.Assert(err, jc.ErrorIsNil)
	storageAttachments, err := sb.UnitStorageAttachments(ctx.unit.UnitTag())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(storageAttachments, gc.HasLen, 1)

	filesystem, err := sb.StorageInstanceFilesystem(storageAttachments[0].StorageInstance())
	c.Assert(err, jc.ErrorIsNil)

	filesystemInfo := state.FilesystemInfo{
		Size:         1024,
		FilesystemId: "fs-id",
	}
	err = sb.SetFilesystemInfo(filesystem.FilesystemTag(), filesystemInfo)
	c.Assert(err, jc.ErrorIsNil)

	machineId, err := ctx.unit.AssignedMachineId()
	c.Assert(err, jc.ErrorIsNil)

	filesystemAttachmentInfo := state.FilesystemAttachmentInfo{
		MountPoint: "/srv/wordpress/content",
	}
	err = sb.SetFilesystemAttachmentInfo(
		names.NewMachineTag(machineId),
		filesystem.FilesystemTag(),
		filesystemAttachmentInfo,
	)
	c.Assert(err, jc.ErrorIsNil)
}

type destroyStorageAttachment struct{}

func (s destroyStorageAttachment) step(c *gc.C, ctx *testContext) {
	sb, err := state.NewStorageBackend(ctx.st)
	c.Assert(err, jc.ErrorIsNil)
	storageAttachments, err := sb.UnitStorageAttachments(ctx.unit.UnitTag())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(storageAttachments, gc.HasLen, 1)
	err = sb.DetachStorage(
		storageAttachments[0].StorageInstance(),
		ctx.unit.UnitTag(),
		false,
		time.Duration(0),
	)
	c.Assert(err, jc.ErrorIsNil)
}

type verifyStorageDetached struct{}

func (s verifyStorageDetached) step(c *gc.C, ctx *testContext) {
	sb, err := state.NewStorageBackend(ctx.st)
	c.Assert(err, jc.ErrorIsNil)
	storageAttachments, err := sb.UnitStorageAttachments(ctx.unit.UnitTag())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(storageAttachments, gc.HasLen, 0)
}

type createSecret struct {
	secretPath string
}

func (s createSecret) step(c *gc.C, ctx *testContext) {
	hr := time.Hour
	active := secrets.StatusActive
	_, err := ctx.secretsFacade.Create(&secrets.SecretConfig{
		Path:           s.secretPath,
		RotateInterval: &hr,
		Status:         &active,
	}, secrets.TypeBlob, secrets.NewSecretValue(map[string]string{"foo": "bar"}))
	c.Assert(err, jc.ErrorIsNil)
}

type rotateSecret struct {
	secretURL string
}

func (s rotateSecret) step(c *gc.C, ctx *testContext) {
	select {
	case ctx.secretsRotateCh <- []string{s.secretURL}:
	case <-time.After(coretesting.LongWait):
		c.Fatalf("sending rotate secret change for %q", s.secretURL)
	}
}

type expectError struct {
	err string
}

func (s expectError) step(_ *gc.C, ctx *testContext) {
	ctx.setExpectedError(s.err)
}

// manualTicker will be used to generate collect-metrics events
// in a time-independent manner for testing.
type manualTicker struct {
	c chan time.Time
}

// Tick sends a signal on the ticker channel.
func (t *manualTicker) Tick() error {
	select {
	case t.c <- time.Now():
	case <-time.After(worstCase):
		return fmt.Errorf("ticker channel blocked")
	}
	return nil
}

type dummyWaiter struct {
	c chan time.Time
}

func (w dummyWaiter) After() <-chan time.Time {
	return w.c
}

// ReturnTimer can be used to replace the update status signal generator.
func (t *manualTicker) ReturnTimer() remotestate.UpdateStatusTimerFunc {
	return func(_ time.Duration) remotestate.Waiter {
		return dummyWaiter{t.c}
	}
}

func newManualTicker() *manualTicker {
	return &manualTicker{
		c: make(chan time.Time),
	}
}

// Instead of having a machine level lock that we have real contention with,
// we instead fake it by creating a process lock. This will block callers within
// the same process. This is necessary due to the function above to return the
// machine lock. We create it once at process initialisation time and use it any
// time the function is asked for.
var processLock machinelock.Lock

func init() {
	processLock = &fakemachinelock{}
}

type fakemachinelock struct {
	machinelock.Lock
	mu sync.Mutex
}

func (f *fakemachinelock) Acquire(_ machinelock.Spec) (func(), error) {
	f.mu.Lock()
	return func() {
		f.mu.Unlock()
	}, nil
}

type activateTestContainer struct {
	containerName string
}

func (s activateTestContainer) step(c *gc.C, ctx *testContext) {
	ctx.pebbleClients[s.containerName].TriggerStart()
}

type injectTestContainer struct {
	containerName string
}

func (s injectTestContainer) step(c *gc.C, ctx *testContext) {
	c.Assert(ctx.uniter, gc.IsNil)
	ctx.containerNames = append(ctx.containerNames, s.containerName)
	if ctx.pebbleClients == nil {
		ctx.pebbleClients = make(map[string]*fakePebbleClient)
	}
	ctx.pebbleClients[s.containerName] = &fakePebbleClient{
		err: errors.BadRequestf("not ready yet"),
	}
}

type triggerShutdown struct {
}

func (t triggerShutdown) step(c *gc.C, ctx *testContext) {
	err := ctx.uniter.Terminate()
	c.Assert(err, jc.ErrorIsNil)
}
