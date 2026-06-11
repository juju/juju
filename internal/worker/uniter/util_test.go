// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/canonical/gomock/gomock"
	pebbleclient "github.com/canonical/pebble/client"
	"github.com/juju/clock/testclock"
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/mutex/v2"
	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"github.com/juju/utils/v4"
	"github.com/juju/worker/v5"
	"gopkg.in/yaml.v2"

	apiuniter "github.com/juju/juju/api/agent/uniter"
	"github.com/juju/juju/api/types"
	"github.com/juju/juju/core/leadership"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/machinelock"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/secrets"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/watchertest"
	jujucharm "github.com/juju/juju/domain/deployment/charm"
	charmtesting "github.com/juju/juju/domain/deployment/charm/testing"
	"github.com/juju/juju/internal/downloader"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/testhelpers/filetesting"
	coretesting "github.com/juju/juju/internal/testing"
	jworker "github.com/juju/juju/internal/worker"
	"github.com/juju/juju/internal/worker/uniter"
	uniterapi "github.com/juju/juju/internal/worker/uniter/api"
	apimocks "github.com/juju/juju/internal/worker/uniter/api/mocks"
	"github.com/juju/juju/internal/worker/uniter/charm"
	"github.com/juju/juju/internal/worker/uniter/operation"
	"github.com/juju/juju/internal/worker/uniter/remotestate"
	"github.com/juju/juju/internal/worker/uniter/runner"
	runnercontext "github.com/juju/juju/internal/worker/uniter/runner/context"
	contextmocks "github.com/juju/juju/internal/worker/uniter/runner/context/mocks"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/testcharms"
)

type relationUnitSettings map[string]int64

type storageAttachment struct {
	attached bool
	eventCh  chan struct{}
}

type testContext struct {
	ctrl    *gomock.Controller
	uuid    string
	path    string
	dataDir string
	s       *UniterSuite
	stepped *MockStepped

	// API clients.
	api           *apimocks.MockUniterClient
	resources     *contextmocks.MockOpenedResourceClient
	leaderTracker *mockLeaderTracker
	charmDirGuard *mockCharmDirGuard

	// Uniter artefacts.
	mu             sync.Mutex
	charms         map[string][]byte
	servedCharms   map[string][]byte
	hooks          []string
	hooksCompleted []string
	expectedError  string
	runner         *mockRunner
	deployer       *mockDeployer
	uniter         *uniter.Uniter

	// Remote watcher artefacts.
	startError           bool
	sendEvents           bool
	unitWatchCounter     atomic.Int32
	unitCh               sync.Map
	unitResolveCh        chan struct{}
	configCh             chan []string
	relCh                chan []string
	consumedSecretsCh    chan []string
	applicationCh        chan struct{}
	storageCh            chan []string
	leadershipSettingsCh chan struct{}
	actionsCh            chan []string
	relationUnitCh       chan watcher.RelationUnitsChange

	// Stateful domain entities.
	unit     *unit
	app      *application
	charm    *apimocks.MockCharm
	charmURL string // set in addCharm.prepare for use in setupUniter

	relCounter atomic.Int32
	relation   *relation

	relUnitCounter atomic.Int32
	relUnit        *relationUnit

	relatedApplication *apimocks.MockApplication

	subordRelation *relation

	// Data model aka "state".
	stateMu sync.Mutex

	storage        map[string]*storageAttachment
	relationUnits  map[int]relationUnitSettings
	actionCounter  atomic.Int32
	pendingActions []*apiuniter.Action

	createdSecretURI *secrets.URI
	secretsRotateCh  chan []string
	secretsExpireCh  chan []string
	secretRevisions  map[string]int
	secretsClient    *apimocks.MockSecretsClient
	secretBackends   *apimocks.MockSecretsBackend

	// Uniter state attributes (the ones we care about).
	uniterState   string
	secretsState  string
	relationState map[int]string

	// Running state.
	updateStatusHookTicker *manualTicker
	containerNames         []string
	pebbleClients          map[string]*fakePebbleClient
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
	ctx.expectedError = err
	ctx.mu.Unlock()
}

func (ctx *testContext) run(c tc.LikeC, steps []stepper) {
	defer func() {
		if ctx.uniter != nil {
			err := worker.Stop(ctx.uniter)
			if ctx.expectedError == "" {
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
				c.Assert(err, tc.ErrorIsNil)
			} else {
				c.Assert(err, tc.ErrorMatches, ctx.expectedError)
			}
		}
	}()
	for _, s := range steps {
		s.prepare(c, ctx)
	}
	for i, s := range steps {
		c.Logf("step %d:\n", i)
		step(c, ctx, s)
	}
}

func (ctx *testContext) waitFor(c tc.LikeC, ch chan bool, msg string) {
	c.Logf("waiting for %s", msg)
	<-ch
}

func (ctx *testContext) sendUnitNotify(c tc.LikeC, msg string) {
	ctx.unitCh.Range(func(k, v any) bool {
		ctx.sendNotify(c, v.(chan struct{}), msg)
		return true
	})
}

func (ctx *testContext) sendNotify(c tc.LikeC, ch chan struct{}, msg string) {
	if !ctx.sendEvents || ctx.startError {
		return
	}
	c.Logf("sending: %s", msg)
	ch <- struct{}{}
	c.Logf("sent: %s", msg)
}

func (ctx *testContext) sendStrings(c tc.LikeC, ch chan []string, msg string, s ...string) {
	if !ctx.sendEvents || ctx.startError {
		return
	}
	c.Logf("sending: %s (%q)", msg, s)
	ch <- s
	c.Logf("sent: %s (%q)", msg, s)
}

func (ctx *testContext) sendRelationUnitChange(c tc.LikeC, msg string, ruc watcher.RelationUnitsChange) {
	c.Logf("sending: %s: %+v", msg, ruc)
	ctx.relationUnitCh <- ruc
	c.Logf("sent: %s: %+v", msg, ruc)
}

func (ctx *testContext) expectHookContext(c tc.LikeC, stepped *MockSteppedSteppedCall) {
	ctx.api.EXPECT().GetUnitContext(gomock.Any(), gomock.Any()).Return(apiuniter.UnitContext{
		APIAddresses:    []string{"10.6.6.6"},
		CloudAPIVersion: "6.6.6",
	}, nil).AnyTimes().After(stepped)

	ctx.secretsClient.EXPECT().SecretMetadata(gomock.Any()).Return(nil, nil).AnyTimes().After(stepped)
}

func (ctx *testContext) matchHooks(c tc.LikeC) (match, cannotMatch, overshoot bool) {
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
	prepare(c tc.LikeC, ctx *testContext)
	step(c tc.LikeC, ctx *testContext)
}

func step(c tc.LikeC, ctx *testContext, s stepper) {
	c.Logf("%#v", s)
	s.step(c, ctx)
}

type createCharm struct {
	revision            int
	badHooks            []string
	customize           func(tc.LikeC, *testContext, string)
	charm               *addCharm
	deferredDeployerErr error // saved if customize changed ctx.deployer.err at prepare time
}

func startupHooks(minion bool) []string {
	if minion {
		return []string{"install", "config-changed", "start"}
	}
	return []string{"install", "leader-elected", "config-changed", "start"}
}

func (s *createCharm) prepare(c tc.LikeC, ctx *testContext) {
	base := testcharms.Repo.ClonedDirPath(c.MkDir(), "wordpress")
	if s.customize != nil {
		// Save and restore ctx.deployer.err: some customize functions set
		// ctx.deployer.err which is a runtime state that must only take
		// effect at step time (e.g. after the initial charm install completes).
		savedErr := ctx.deployer.err
		s.customize(c, ctx, base)
		if ctx.deployer.err != savedErr {
			s.deferredDeployerErr = ctx.deployer.err
			ctx.deployer.err = savedErr
		}
	}
	if len(s.badHooks) > 0 {
		ctx.runner.hooksWithErrors = set.NewStrings(s.badHooks...)
	}
	dir, err := charmtesting.ReadCharmDir(base)
	c.Assert(err, tc.ErrorIsNil)
	s.charm = &addCharm{dir: dir, curl: curl(s.revision), revision: s.revision}
	s.charm.prepare(c, ctx)
}

func (s *createCharm) step(c tc.LikeC, ctx *testContext) {
	if s.deferredDeployerErr != nil {
		// Re-apply the deployer error that was deferred from prepare time.
		ctx.deployer.err = s.deferredDeployerErr
	}
	step(c, ctx, s.charm)
}

type addCharm struct {
	dir      *charmtesting.CharmDir
	curl     string
	revision int
	body     []byte
}

func (s *addCharm) prepare(c tc.LikeC, ctx *testContext) {
	var buf bytes.Buffer
	err := s.dir.ArchiveTo(&buf)
	c.Assert(err, tc.ErrorIsNil)
	s.body = buf.Bytes()
	hash, _, err := utils.ReadSHA256(&buf)
	c.Assert(err, tc.ErrorIsNil)
	ctx.charm = apimocks.NewMockCharm(ctx.ctrl)
	ctx.charmURL = s.curl
	stepped := ctx.stepped.EXPECT().Stepped(s)
	ctx.charm.EXPECT().URL().Return(s.curl).AnyTimes().After(stepped)
	ctx.charm.EXPECT().ArchiveSha256(gomock.Any()).Return(hash, nil).AnyTimes().After(stepped)
	ctx.api.EXPECT().Charm(s.curl).Return(ctx.charm, nil).AnyTimes().After(stepped)
}

func (s *addCharm) step(_ tc.LikeC, ctx *testContext) {
	storagePath := fmt.Sprintf("/charms/%s/%d", s.dir.Meta().Name, s.revision)
	ctx.charms[storagePath] = s.body
	// Advance prepared mocks.
	ctx.stepped.Stepped(s)
}

type serveCharm struct{}

func (s *serveCharm) prepare(_ tc.LikeC, _ *testContext) {}

func (s *serveCharm) step(c tc.LikeC, ctx *testContext) {
	for storagePath, data := range ctx.charms {
		ctx.servedCharms[storagePath] = data
		delete(ctx.charms, storagePath)
	}
}

type createApplicationAndUnit struct {
	applicationName string
	storage         map[string]int
	container       bool
}

func (s *createApplicationAndUnit) prepare(c tc.LikeC, ctx *testContext) {
	if s.applicationName == "" {
		s.applicationName = "u"
	}
	unitTag := names.NewUnitTag(s.applicationName + "/0")
	ctx.unit = ctx.makeUnit(c, unitTag, life.Alive)

	appTag := names.NewApplicationTag(s.applicationName)
	ctx.app = ctx.makeApplication(appTag)

	ctx.storage = make(map[string]*storageAttachment)
	for si, count := range s.storage {
		for n := range count {
			tag := names.NewStorageTag(fmt.Sprintf("%s/%d", si, n))
			ctx.storage[tag.Id()] = &storageAttachment{
				eventCh: make(chan struct{}, 2),
			}
		}
	}

	stepped := ctx.stepped.EXPECT().Stepped(s)
	// Assign the unit to a provisioned machine to match expected state.
	if s.container {
		machineTag := names.NewMachineTag("0/lxd/0")
		ctx.unit.EXPECT().AssignedMachine(gomock.Any()).Return(machineTag, nil).AnyTimes().After(stepped)
	} else {
		machineTag := names.NewMachineTag("0")
		ctx.unit.EXPECT().AssignedMachine(gomock.Any()).Return(machineTag, nil).AnyTimes().After(stepped)
	}
}

func (s *createApplicationAndUnit) step(c tc.LikeC, ctx *testContext) {
	// Advance prepared mocks.
	ctx.stepped.Stepped(s)
	ctx.sendNotify(c, ctx.applicationCh, "application created event")
}

type deleteUnit struct{}

func (s *deleteUnit) prepare(_ tc.LikeC, _ *testContext) {}

func (s *deleteUnit) step(c tc.LikeC, ctx *testContext) {
	ctx.unit.mu.Lock()
	ctx.unit.life = life.Dead
	ctx.unit.mu.Unlock()
}

type createUniter struct {
	minion               bool
	startError           bool
	executorFunc         uniter.NewOperationExecutorFunc
	translateResolverErr func(error) error
	// sub-steppers
	createAppUnit     *createApplicationAndUnit
	forceMinionStep   *forceMinion
	startUniterStep   *startUniter
	waitAddressesStep *waitAddresses
}

func (s *createUniter) prepare(c tc.LikeC, ctx *testContext) {
	s.createAppUnit = &createApplicationAndUnit{}
	s.createAppUnit.prepare(c, ctx)

	ctx.leaderTracker = newMockLeaderTracker(ctx, s.minion)

	if s.minion {
		s.forceMinionStep = &forceMinion{}
		s.forceMinionStep.prepare(c, ctx)
	}

	s.startUniterStep = &startUniter{
		newExecutorFunc:      s.executorFunc,
		translateResolverErr: s.translateResolverErr,
		unit:                 ctx.unit.Name(),
	}
	s.startUniterStep.prepare(c, ctx)

	s.waitAddressesStep = &waitAddresses{}
	s.waitAddressesStep.prepare(c, ctx)
}

func (s *createUniter) step(c tc.LikeC, ctx *testContext) {
	ctx.startError = s.startError
	step(c, ctx, s.createAppUnit)
	if s.minion {
		step(c, ctx, s.forceMinionStep)
	}
	step(c, ctx, s.startUniterStep)
	step(c, ctx, s.waitAddressesStep)
}

type waitAddresses struct{}

func (s *waitAddresses) prepare(_ tc.LikeC, _ *testContext) {}

func (s *waitAddresses) step(c tc.LikeC, ctx *testContext) {
	c.Log("waiting for unit addresses")
	for {
		time.Sleep(coretesting.ShortWait)
		private, _ := ctx.unit.PrivateAddress(c.Context())
		if private != dummyPrivateAddress.Value {
			continue
		}
		public, _ := ctx.unit.PublicAddress(c.Context())
		if public != dummyPublicAddress.Value {
			continue
		}
		return
	}
}

type startUniter struct {
	unit                 string
	newExecutorFunc      uniter.NewOperationExecutorFunc
	translateResolverErr func(error) error
	rebootQuerier        uniter.RebootQuerier
	stepped              *MockSteppedSteppedCall
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

type unitStateMatcher struct {
	c        tc.LikeC
	expected string
}

func (m unitStateMatcher) Matches(x any) bool {
	obtained, ok := x.(params.SetUnitStateArg)
	if !ok || obtained.UniterState == nil {
		return false
	}

	found := *obtained.UniterState == m.expected
	if !found {
		m.c.Logf("Unit state mismatch\nGot: \n%s\nWant:\n%s", *obtained.UniterState, m.expected)
	}
	m.c.Assert(found, tc.IsTrue)
	return true
}

func (m unitStateMatcher) String() string {
	return "Match the contents of the UniterState pointer in params.SetUnitStateArg"
}

type uniterCharmUpgradeStateMatcher struct {
}

func (m uniterCharmUpgradeStateMatcher) Matches(x any) bool {
	obtained, ok := x.(params.SetUnitStateArg)
	if !ok || obtained.UniterState == nil {
		return false
	}
	return strings.Contains(*obtained.UniterState, "op: upgrade")
}

func (m uniterCharmUpgradeStateMatcher) String() string {
	return "match uniter upgrade charm state"
}

type uniterRunHookStateMatcher struct {
}

func (m uniterRunHookStateMatcher) Matches(x any) bool {
	obtained, ok := x.(params.SetUnitStateArg)
	if !ok || obtained.UniterState == nil {
		return false
	}
	return strings.Contains(*obtained.UniterState, "op: run-hook")
}

func (m uniterRunHookStateMatcher) String() string {
	return "match uniter run hook state"
}

type uniterRunActionStateMatcher struct {
}

func (m uniterRunActionStateMatcher) Matches(x any) bool {
	obtained, ok := x.(params.SetUnitStateArg)
	if !ok || obtained.UniterState == nil {
		return false
	}
	return strings.Contains(*obtained.UniterState, "op: run-action")
}

func (m uniterRunActionStateMatcher) String() string {
	return "match uniter run action state"
}

type uniterContinueStateMatcher struct {
}

func (m uniterContinueStateMatcher) Matches(x any) bool {
	obtained, ok := x.(params.SetUnitStateArg)
	if !ok || obtained.UniterState == nil {
		return false
	}
	return strings.Contains(*obtained.UniterState, "op: continue")
}

func (m uniterContinueStateMatcher) String() string {
	return "match uniter continue state"
}

type uniterSecretsStateMatcher struct {
}

func (m uniterSecretsStateMatcher) Matches(x any) bool {
	obtained, ok := x.(params.SetUnitStateArg)
	if !ok || obtained.SecretState == nil {
		return false
	}
	return strings.Contains(*obtained.SecretState, "secret-revisions:") ||
		strings.Contains(*obtained.SecretState, "secret-obsolete-revisions:") ||
		*obtained.SecretState == "{}\n"
}

func (m uniterSecretsStateMatcher) String() string {
	return "match uniter secrets state"
}

type uniterStorageStateMatcher struct {
}

func (m uniterStorageStateMatcher) Matches(x any) bool {
	obtained, ok := x.(params.SetUnitStateArg)
	if !ok || obtained.StorageState == nil {
		return false
	}
	// TODO(wallyworld) - get storage to match from the context
	return strings.Contains(*obtained.StorageState, "wp-content/0:") ||
		*obtained.StorageState == "{}\n"
}

func (m uniterStorageStateMatcher) String() string {
	return "match uniter storage state"
}

type uniterRelationStateMatcher struct {
}

func (m uniterRelationStateMatcher) Matches(x any) bool {
	obtained, ok := x.(params.SetUnitStateArg)
	if !ok || obtained.RelationState == nil {
		return false
	}
	return true
}

func (m uniterRelationStateMatcher) String() string {
	return "match uniter relation state"
}

type unitWatcher struct {
	*watchertest.MockNotifyWatcher
	ctx *testContext
	id  int
}

func (w *unitWatcher) Kill() {
	w.ctx.unitCh.Delete(w.id)
	w.MockNotifyWatcher.Kill()
}

func (s *startUniter) expectRemoteStateWatchers(c tc.LikeC, ctx *testContext) {
	ctx.unit.EXPECT().Watch(gomock.Any()).DoAndReturn(func(context.Context) (watcher.NotifyWatcher, error) {
		num := int(ctx.unitWatchCounter.Add(1))
		ch := make(chan struct{}, 3)
		ctx.unitCh.Store(num, ch)
		w := watchertest.NewMockNotifyWatcher(ch)
		defer ctx.sendNotify(c, ch, "initial unit event")
		return &unitWatcher{
			MockNotifyWatcher: w,
			ctx:               ctx,
			id:                num,
		}, nil
	}).AnyTimes().After(s.stepped)

	ctx.app.EXPECT().Watch(gomock.Any()).DoAndReturn(func(context.Context) (watcher.NotifyWatcher, error) {
		// Drain any stale event that arrived between startUniter draining
		// the channel and this goroutine being scheduled (e.g. an
		// upgradeCharm event). The RSW will refresh application state via
		// applicationChanged when it processes the initial event below.
		select {
		case <-ctx.applicationCh:
		default:
		}
		ctx.sendNotify(c, ctx.applicationCh, "initial application event")
		w := watchertest.NewMockNotifyWatcher(ctx.applicationCh)
		return w, nil
	}).AnyTimes().After(s.stepped)

	ctx.unit.EXPECT().WatchResolveMode(gomock.Any()).DoAndReturn(func(context.Context) (watcher.NotifyWatcher, error) {
		// Drain any stale event that arrived between startUniter draining
		// the channel and this goroutine being scheduled (e.g. a
		// resolveError event). The RSW reads the current resolved mode via
		// Resolved() when it processes the initial event below, so no
		// information is lost by discarding the stale entry.
		select {
		case <-ctx.unitResolveCh:
		default:
		}
		ctx.sendNotify(c, ctx.unitResolveCh, "initial resolve event")
		w := watchertest.NewMockNotifyWatcher(ctx.unitResolveCh)
		return w, nil
	}).AnyTimes().After(s.stepped)

	ctx.unit.EXPECT().WatchInstanceData(gomock.Any()).DoAndReturn(func(context.Context) (watcher.NotifyWatcher, error) {
		ch := make(chan struct{}, 1)
		ch <- struct{}{}
		w := watchertest.NewMockNotifyWatcher(ch)
		return w, nil
	}).AnyTimes().After(s.stepped)

	ctx.api.EXPECT().WatchUpdateStatusHookInterval(gomock.Any()).DoAndReturn(func(context.Context) (watcher.NotifyWatcher, error) {
		ch := make(chan struct{}, 1)
		ch <- struct{}{}
		w := watchertest.NewMockNotifyWatcher(ch)
		return w, nil
	}).AnyTimes().After(s.stepped)

	ctx.unit.EXPECT().WatchConfigSettingsHash(gomock.Any()).DoAndReturn(func(context.Context) (watcher.StringsWatcher, error) {
		ctx.sendStrings(c, ctx.configCh, "initial config event", ctx.app.configHash(nil))
		w := watchertest.NewMockStringsWatcher(ctx.configCh)
		return w, nil
	}).AnyTimes().After(s.stepped)

	ctx.unit.EXPECT().WatchTrustConfigSettingsHash(gomock.Any()).DoAndReturn(func(context.Context) (watcher.StringsWatcher, error) {
		ch := make(chan []string, 1)
		ch <- []string{"trust-hash"}
		w := watchertest.NewMockStringsWatcher(ch)
		return w, nil
	}).AnyTimes().After(s.stepped)

	ctx.unit.EXPECT().WatchAddressesHash(gomock.Any()).DoAndReturn(func(context.Context) (watcher.StringsWatcher, error) {
		ch := make(chan []string, 1)
		ch <- []string{"address-hash"}
		w := watchertest.NewMockStringsWatcher(ch)
		return w, nil
	}).AnyTimes().After(s.stepped)

	ctx.unit.EXPECT().WatchRelations(gomock.Any()).DoAndReturn(func(context.Context) (watcher.StringsWatcher, error) {
		var relations []string
		if ctx.relation != nil {
			relations = []string{ctx.relation.Tag().Id()}
		}
		ctx.sendStrings(c, ctx.relCh, "initial relation event", relations...)
		w := watchertest.NewMockStringsWatcher(ctx.relCh)
		return w, nil
	}).AnyTimes().After(s.stepped)

	ctx.unit.EXPECT().WatchStorage(gomock.Any()).DoAndReturn(func(context.Context) (watcher.StringsWatcher, error) {
		var storages []string
		for si, attachment := range ctx.storage {
			tag := names.NewStorageTag(si)
			storages = append(storages, tag.Id())
			storageW := watchertest.NewMockNotifyWatcher(attachment.eventCh)
			ctx.api.EXPECT().WatchStorageAttachment(gomock.Any(), tag, ctx.unit.Tag()).Return(storageW, nil)
			ctx.api.EXPECT().StorageAttachment(gomock.Any(), tag, ctx.unit.Tag()).DoAndReturn(func(_ context.Context, _ names.StorageTag, _ names.UnitTag) (params.StorageAttachment, error) {
				ctx.stateMu.Lock()
				defer ctx.stateMu.Unlock()
				if attachment, ok := ctx.storage[tag.Id()]; !attachment.attached || !ok {
					return params.StorageAttachment{}, errors.NotProvisioned
				}
				return params.StorageAttachment{
					StorageTag: tag.String(),
					UnitTag:    ctx.unit.Tag().String(),
					Kind:       params.StorageKindFilesystem,
					Location:   "/path/to/nowhere",
					Life:       "alive",
				}, nil
			}).AnyTimes()
			ctx.sendNotify(c, attachment.eventCh, "storage attach event")
		}
		ctx.sendStrings(c, ctx.storageCh, "initial storage event", storages...)

		w := watchertest.NewMockStringsWatcher(ctx.storageCh)
		return w, nil
	}).AnyTimes().After(s.stepped)

	ctx.unit.EXPECT().WatchActionNotifications(gomock.Any()).DoAndReturn(func(context.Context) (watcher.StringsWatcher, error) {
		// Reset the channel so any stale buffered event from a previous
		// watcher that was killed before draining it cannot block the send
		// below (the send happens in this goroutine, which is also the only
		// future receiver — a full buffer would deadlock).
		ctx.actionsCh = make(chan []string, 1)
		var actions []string
		for _, a := range ctx.pendingActions {
			actions = append(actions, a.ID())
		}
		ctx.sendStrings(c, ctx.actionsCh, "initial action event", actions...)
		w := watchertest.NewMockStringsWatcher(ctx.actionsCh)
		return w, nil
	}).AnyTimes().After(s.stepped)

	ctx.secretsClient.EXPECT().WatchConsumedSecretsChanges(gomock.Any(), ctx.unit.Name()).DoAndReturn(func(context.Context, string) (watcher.StringsWatcher, error) {
		// Reset the channel for the same reason as actionsCh above: the
		// buffer is 1, and a previous watcher killed before reading its
		// initial event leaves the buffer full, blocking this send.
		ctx.consumedSecretsCh = make(chan []string, 1)
		ctx.sendStrings(c, ctx.consumedSecretsCh, "initial consumed secrets event")
		w := watchertest.NewMockStringsWatcher(ctx.consumedSecretsCh)
		return w, nil
	}).AnyTimes().After(s.stepped)

	ctx.secretsClient.EXPECT().WatchObsolete(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, owners ...names.Tag) (watcher.StringsWatcher, error) {
		ownerNames := set.NewStrings()
		for _, o := range owners {
			ownerNames.Add(o.Id())
		}
		ownerNames.Remove(ctx.unit.Name())
		ownerNames.Remove(ctx.app.Tag().Id())
		if !ownerNames.IsEmpty() {
			c.Fatalf("unexpected watch obsolete secret owner(s): %q", ownerNames.Values())
		}
		ch := make(chan []string, 1)
		ch <- []string(nil)
		w := watchertest.NewMockStringsWatcher(ch)
		return w, nil
	}).AnyTimes().After(s.stepped)

	ctx.secretsClient.EXPECT().WatchDeleted(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, owners ...names.Tag) (watcher.StringsWatcher, error) {
		ownerNames := set.NewStrings()
		for _, o := range owners {
			ownerNames.Add(o.Id())
		}
		ownerNames.Remove(ctx.unit.Name())
		ownerNames.Remove(ctx.app.Tag().Id())
		if !ownerNames.IsEmpty() {
			c.Fatalf("unexpected watch deleted secret owner(s): %q", ownerNames.Values())
		}
		ch := make(chan []string, 1)
		ch <- []string(nil)
		w := watchertest.NewMockStringsWatcher(ch)
		return w, nil
	}).AnyTimes().After(s.stepped)
}

func (s *startUniter) setupUniter(c tc.LikeC, ctx *testContext) {
	ctx.api.EXPECT().StorageAttachmentLife(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, ids []params.StorageAttachmentId) ([]params.LifeResult, error) {
		ctx.stateMu.Lock()
		defer ctx.stateMu.Unlock()
		result := make([]params.LifeResult, len(ids))
		for i, id := range ids {
			if id.UnitTag != ctx.unit.Tag().String() {
				return nil, errors.Errorf("unexpected storage unit %q", id.UnitTag)
			}
			tag, err := names.ParseStorageTag(id.StorageTag)
			if err != nil {
				return nil, err
			}
			if _, ok := ctx.storage[tag.Id()]; ok {
				result[i] = params.LifeResult{
					Life: life.Alive,
				}
			} else {
				result[i] = params.LifeResult{
					Error: &params.Error{Code: params.CodeNotFound},
				}
			}
		}
		return result, nil
	}).AnyTimes().After(s.stepped)

	// Consumed secrets initial event.
	ctx.secretsClient.EXPECT().GetConsumerSecretsRevisionInfo(gomock.Any(), ctx.unit.Name(), []string(nil)).Return(nil, nil).AnyTimes().After(s.stepped)

	ctx.api.EXPECT().UpdateStatusHookInterval(gomock.Any()).Return(time.Minute, nil).AnyTimes().After(s.stepped)

	// Storage attachments init.
	var attachments []params.StorageAttachmentId
	ctx.stateMu.Lock()
	for si, attachment := range ctx.storage {
		if !attachment.attached {
			continue
		}
		attachments = append(attachments, params.StorageAttachmentId{
			StorageTag: names.NewStorageTag(si).String(),
			UnitTag:    ctx.unit.Tag().String(),
		})
	}
	ctx.stateMu.Unlock()
	tag := names.NewUnitTag(s.unit)
	ctx.api.EXPECT().UnitStorageAttachments(gomock.Any(), tag).Return(attachments, nil).AnyTimes().After(s.stepped)
	ctx.api.EXPECT().Unit(gomock.Any(), tag).DoAndReturn(func(_ context.Context, tag names.UnitTag) (uniterapi.Unit, error) {
		if tag.Id() != ctx.unit.Tag().Id() {
			return nil, errors.New("permission denied")
		}
		return ctx.unit, nil
	}).AnyTimes().After(s.stepped)

	// Secrets init.
	ctx.secretsClient.EXPECT().SecretMetadata(gomock.Any()).Return(nil, nil).AnyTimes().After(s.stepped)
	ctx.secretsClient.EXPECT().SecretRotated(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, uri string, rev int) error {
		ctx.stateMu.Lock()
		ctx.secretRevisions[uri] = rev + 1
		ctx.stateMu.Unlock()
		return nil
	}).AnyTimes().After(s.stepped)

	// Context factory init.
	ctx.api.EXPECT().Model(gomock.Any()).Return(&types.Model{
		Name:      "test-model",
		UUID:      coretesting.ModelTag.Id(),
		ModelType: types.IAAS,
	}, nil).AnyTimes().After(s.stepped)

	// Set up the initial install op.
	data, err := yaml.Marshal(operation.State{
		CharmURL: ctx.charmURL,
		Kind:     "install",
		Step:     "pending",
	})
	c.Assert(err, tc.ErrorIsNil)
	st := string(data)
	ctx.unit.EXPECT().SetState(gomock.Any(), unitStateMatcher{c: c, expected: st}).Return(nil).MaxTimes(1).After(s.stepped)

	data, err = yaml.Marshal(operation.State{
		CharmURL: ctx.charmURL,
		Kind:     "install",
		Step:     "done",
	})
	c.Assert(err, tc.ErrorIsNil)
	st = string(data)
	ctx.unit.EXPECT().SetState(gomock.Any(), unitStateMatcher{c: c, expected: st}).Return(nil).MaxTimes(1).After(s.stepped)

	// Termination.
	ctx.secretsClient.EXPECT().UnitOwnedSecretsAndRevisions(gomock.Any(), ctx.unit.Tag()).Return(nil, nil).AnyTimes().After(s.stepped)
}

func (s *startUniter) setupUniterHookExec(c tc.LikeC, ctx *testContext) {
	ctx.api.EXPECT().Application(gomock.Any(), ctx.app.Tag()).Return(ctx.app, nil).AnyTimes().After(s.stepped)
	ctx.expectHookContext(c, s.stepped)

	setState := func(_ context.Context, unitState params.SetUnitStateArg) error {
		ctx.stateMu.Lock()
		defer ctx.stateMu.Unlock()
		if unitState.UniterState != nil {
			ctx.uniterState = *unitState.UniterState
		}
		if unitState.SecretState != nil {
			ctx.secretsState = *unitState.SecretState
		}
		if unitState.RelationState != nil {
			ctx.relationState = *unitState.RelationState
		}
		return nil
	}
	ctx.unit.EXPECT().SetState(gomock.Any(), uniterCharmUpgradeStateMatcher{}).DoAndReturn(setState).AnyTimes().After(s.stepped)
	ctx.unit.EXPECT().SetState(gomock.Any(), uniterRunHookStateMatcher{}).DoAndReturn(setState).AnyTimes().After(s.stepped)
	ctx.unit.EXPECT().SetState(gomock.Any(), uniterRunActionStateMatcher{}).DoAndReturn(setState).AnyTimes().After(s.stepped)
	ctx.unit.EXPECT().SetState(gomock.Any(), uniterContinueStateMatcher{}).DoAndReturn(setState).AnyTimes().After(s.stepped)
	ctx.unit.EXPECT().SetState(gomock.Any(), uniterSecretsStateMatcher{}).DoAndReturn(setState).AnyTimes().After(s.stepped)
	ctx.unit.EXPECT().SetState(gomock.Any(), uniterStorageStateMatcher{}).DoAndReturn(setState).AnyTimes().After(s.stepped)
	ctx.unit.EXPECT().SetState(gomock.Any(), uniterRelationStateMatcher{}).DoAndReturn(setState).AnyTimes().After(s.stepped)
}

func (s *startUniter) prepare(c tc.LikeC, ctx *testContext) {
	if s.unit == "" {
		s.unit = "u/0"
	}
	s.stepped = ctx.stepped.EXPECT().Stepped(s)
	s.setupUniter(c, ctx)
	s.setupUniterHookExec(c, ctx)
	s.expectRemoteStateWatchers(c, ctx)
}

func (s *startUniter) step(c tc.LikeC, ctx *testContext) {
	if s.unit == "" {
		s.unit = "u/0"
	}
	if ctx.uniter != nil {
		panic("don't start two uniters!")
	}
	if ctx.api == nil {
		panic("API connection not established")
	}
	if ctx.resources == nil {
		panic("resources API connection not established")
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
	dlr := &downloader.Downloader{
		OpenBlob: func(req downloader.Request) (io.ReadCloser, error) {
			ctx.app.mu.Lock()
			defer ctx.app.mu.Unlock()
			curl := jujucharm.MustParseURL(ctx.app.charmURL)
			storagePath := fmt.Sprintf("/charms/%s/%d", curl.Name, curl.Revision)
			blob, ok := ctx.servedCharms[storagePath]
			if !ok {
				return nil, errors.NotFoundf(ctx.app.charmURL)
			}
			return io.NopCloser(bytes.NewReader(blob)), nil
		},
	}
	operationExecutor := operation.NewExecutor
	if s.newExecutorFunc != nil {
		operationExecutor = s.newExecutorFunc
	}

	// Drain stale events from shared channels. When a previous RSW is
	// stopped (e.g. after ErrRestart or stopUniter), its setup goroutine
	// may have sent an initial event before the RSW's select loop started
	// reading. That event stays buffered and would block the next RSW's
	// setup DoAndReturn, causing a flaky hang.
	drainNotify := func(ch chan struct{}) {
		for {
			select {
			case <-ch:
			default:
				return
			}
		}
	}
	drainNotify(ctx.unitResolveCh)
	drainNotify(ctx.applicationCh)
	ctx.sendEvents = true

	if ctx.leaderTracker == nil {
		ctx.leaderTracker = newMockLeaderTracker(ctx, false)
	}

	tag := names.NewUnitTag(s.unit)
	uniterParams := uniter.UniterParams{
		UniterClient: ctx.api,
		UnitTag:      tag,
		ModelType:    model.IAAS,
		LeadershipTrackerFunc: func(_ names.UnitTag) leadership.Tracker {
			return ctx.leaderTracker
		},
		ResourcesClient:      ctx.resources,
		CharmDirGuard:        ctx.charmDirGuard,
		DataDir:              ctx.dataDir,
		Downloader:           dlr,
		MachineLock:          processLock,
		UpdateStatusSignal:   ctx.updateStatusHookTicker.ReturnTimer(),
		NewOperationExecutor: operationExecutor,
		NewProcessRunner: func(context runnercontext.Context, paths runnercontext.Paths, options ...runner.Option) runner.Runner {
			ctx.runner.stdContext = context
			return ctx.runner
		},
		NewDeployer: func(charmPath, dataPath string, bundles charm.BundleReader, logger logger.Logger) (charm.Deployer, error) {
			ctx.deployer.charmPath = charmPath
			ctx.deployer.dataPath = dataPath
			ctx.deployer.bundles = bundles
			return ctx.deployer, nil
		},
		TranslateResolverErr: s.translateResolverErr,
		Observer:             ctx,
		Clock:                testclock.NewDilatedWallClock(coretesting.ShortWait),
		RebootQuerier:        s.rebootQuerier,
		Logger:               loggertesting.WrapCheckLog(c),
		ContainerNames:       ctx.containerNames,
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
		SecretRotateWatcherFunc: func(u names.UnitTag, isLeader bool, secretsChanged chan []string) (worker.Worker, error) {
			c.Assert(u.String(), tc.Equals, tag.String())
			ctx.secretsRotateCh = secretsChanged
			return watchertest.NewMockStringsWatcher(ctx.secretsRotateCh), nil
		},
		SecretExpiryWatcherFunc: func(u names.UnitTag, isLeader bool, secretsChanged chan []string) (worker.Worker, error) {
			c.Assert(u.String(), tc.Equals, tag.String())
			ctx.secretsExpireCh = secretsChanged
			return watchertest.NewMockStringsWatcher(ctx.secretsExpireCh), nil
		},
		SecretsClient: ctx.secretsClient,
		SecretsBackendGetter: func() (uniterapi.SecretsBackend, error) {
			return ctx.secretBackends, nil
		},
	}
	// Advance prepared mocks.
	ctx.stepped.Stepped(s)
	var err error
	ctx.uniter, err = uniter.NewUniter(&uniterParams)
	c.Assert(err, tc.ErrorIsNil)
}

type waitUniterDead struct {
	err string
}

func (s *waitUniterDead) prepare(_ tc.LikeC, _ *testContext) {}

func (s *waitUniterDead) step(c tc.LikeC, ctx *testContext) {
	if s.err != "" {
		err := s.waitDead(c, ctx)
		c.Log(errors.ErrorStack(err))
		c.Assert(err, tc.ErrorMatches, s.err)
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
		su := &startUniter{}
		su.prepare(c, ctx)
		step(c, ctx, su)
		err = s.waitDead(c, ctx)
	}
	c.Assert(err, tc.Equals, jworker.ErrTerminateAgent)
}

func (s waitUniterDead) waitDead(c tc.LikeC, ctx *testContext) error {
	u := ctx.uniter
	ctx.uniter = nil

	c.Log("waiting for uniter to stop")
	return u.Wait()
}

type stopUniter struct {
	err string
}

func (s *stopUniter) prepare(_ tc.LikeC, _ *testContext) {}

func (s *stopUniter) step(c tc.LikeC, ctx *testContext) {
	u := ctx.uniter
	if u == nil {
		c.Logf("uniter not started, skipping stopUniter{}")
		return
	}
	ctx.uniter = nil
	err := worker.Stop(u)
	if s.err == "" {
		c.Assert(err, tc.ErrorIsNil)
	} else {
		c.Assert(err, tc.ErrorMatches, s.err)
	}
	ctx.unitCh = sync.Map{}
}

type verifyWaiting struct {
	// sub-steppers
	stopUniterStep  *stopUniter
	startUniterStep *startUniter
	waitHooksStep   *waitHooks
}

func (s *verifyWaiting) prepare(c tc.LikeC, ctx *testContext) {
	s.stopUniterStep = &stopUniter{}
	s.stopUniterStep.prepare(c, ctx)

	s.startUniterStep = &startUniter{rebootQuerier: fakeRebootQuerier{rebootNotDetected}}
	s.startUniterStep.prepare(c, ctx)

	wh := waitHooks(nil)
	s.waitHooksStep = &wh
	s.waitHooksStep.prepare(c, ctx)
}

func (s *verifyWaiting) step(c tc.LikeC, ctx *testContext) {
	step(c, ctx, s.stopUniterStep)
	step(c, ctx, s.startUniterStep)
	step(c, ctx, s.waitHooksStep)
}

type verifyRunning struct {
	// sub-steppers
	stopUniterStep  *stopUniter
	startUniterStep *startUniter
	waitHooksStep   *waitHooks
}

func (s *verifyRunning) prepare(c tc.LikeC, ctx *testContext) {
	s.stopUniterStep = &stopUniter{}
	s.stopUniterStep.prepare(c, ctx)

	s.startUniterStep = &startUniter{rebootQuerier: fakeRebootQuerier{rebootNotDetected}}
	s.startUniterStep.prepare(c, ctx)

	wh := waitHooks(nil)
	s.waitHooksStep = &wh
	s.waitHooksStep.prepare(c, ctx)
}

func (s *verifyRunning) step(c tc.LikeC, ctx *testContext) {
	step(c, ctx, s.stopUniterStep)
	step(c, ctx, s.startUniterStep)
	step(c, ctx, s.waitHooksStep)
}

type startupError struct {
	badHook string
	// sub-steppers
	createCharmStep   *createCharm
	serveCharmStep    *serveCharm
	createUniterStep  *createUniter
	waitUnitAgentStep *waitUnitAgent
	hooksBefore       []*waitHooks
	failHook          *waitHooks
	verifyCharmStep   *verifyCharm
}

func (s *startupError) prepare(c tc.LikeC, ctx *testContext) {
	s.createCharmStep = &createCharm{badHooks: []string{s.badHook}}
	s.createCharmStep.prepare(c, ctx)

	s.serveCharmStep = &serveCharm{}
	s.serveCharmStep.prepare(c, ctx)

	s.createUniterStep = &createUniter{}
	s.createUniterStep.prepare(c, ctx)

	s.waitUnitAgentStep = &waitUnitAgent{
		statusGetter: unitStatusGetter,
		status:       status.Error,
		info:         fmt.Sprintf(`hook failed: %q`, s.badHook),
	}
	s.waitUnitAgentStep.prepare(c, ctx)

	for _, hook := range startupHooks(false) {
		if hook == s.badHook {
			wh := waitHooks{"fail-" + hook}
			s.failHook = &wh
			s.failHook.prepare(c, ctx)
			break
		}
		wh := waitHooks{hook}
		whp := &wh
		whp.prepare(c, ctx)
		s.hooksBefore = append(s.hooksBefore, whp)
	}

	s.verifyCharmStep = &verifyCharm{}
	s.verifyCharmStep.prepare(c, ctx)
}

func (s *startupError) step(c tc.LikeC, ctx *testContext) {
	step(c, ctx, s.createCharmStep)
	step(c, ctx, s.serveCharmStep)
	step(c, ctx, s.createUniterStep)
	step(c, ctx, s.waitUnitAgentStep)
	for _, wh := range s.hooksBefore {
		step(c, ctx, wh)
	}
	step(c, ctx, s.failHook)
	step(c, ctx, s.verifyCharmStep)
}

type verifyDeployed struct{}

func (s *verifyDeployed) prepare(_ tc.LikeC, _ *testContext) {}

func (s *verifyDeployed) step(c tc.LikeC, ctx *testContext) {
	c.Assert(ctx.deployer.staged, tc.DeepEquals, curl(0))
	c.Assert(ctx.deployer.deployed, tc.IsTrue)
}

type quickStart struct {
	minion bool
	// sub-steppers
	createCharmStep   *createCharm
	serveCharmStep    *serveCharm
	createUniterStep  *createUniter
	waitUnitAgentStep *waitUnitAgent
	waitHooksStep     *waitHooks
	verifyCharmStep   *verifyCharm
}

func (s *quickStart) prepare(c tc.LikeC, ctx *testContext) {
	s.createCharmStep = &createCharm{}
	s.createCharmStep.prepare(c, ctx)

	s.serveCharmStep = &serveCharm{}
	s.serveCharmStep.prepare(c, ctx)

	s.createUniterStep = &createUniter{minion: s.minion}
	s.createUniterStep.prepare(c, ctx)

	s.waitUnitAgentStep = &waitUnitAgent{status: status.Idle}
	s.waitUnitAgentStep.prepare(c, ctx)

	wh := waitHooks(startupHooks(s.minion))
	s.waitHooksStep = &wh
	s.waitHooksStep.prepare(c, ctx)

	s.verifyCharmStep = &verifyCharm{}
	s.verifyCharmStep.prepare(c, ctx)
}

func (s *quickStart) step(c tc.LikeC, ctx *testContext) {
	step(c, ctx, s.createCharmStep)
	step(c, ctx, s.serveCharmStep)
	step(c, ctx, s.createUniterStep)
	step(c, ctx, s.waitUnitAgentStep)
	step(c, ctx, s.waitHooksStep)
	step(c, ctx, s.verifyCharmStep)
}

type quickStartRelation struct {
	// sub-steppers
	quickStartStep    *quickStart
	addRelationStep   *addRelation
	addRelUnitStep    *addRelationUnit
	waitHooksStep     *waitHooks
	verifyRunningStep *verifyRunning
}

func (s *quickStartRelation) prepare(c tc.LikeC, ctx *testContext) {
	s.quickStartStep = &quickStart{}
	s.quickStartStep.prepare(c, ctx)

	s.addRelationStep = &addRelation{}
	s.addRelationStep.prepare(c, ctx)

	s.addRelUnitStep = &addRelationUnit{}
	s.addRelUnitStep.prepare(c, ctx)

	wh := waitHooks{"db-relation-joined mysql/0 db:0", "db-relation-changed mysql/0 db:0"}
	s.waitHooksStep = &wh
	s.waitHooksStep.prepare(c, ctx)

	s.verifyRunningStep = &verifyRunning{}
	s.verifyRunningStep.prepare(c, ctx)
}

func (s *quickStartRelation) step(c tc.LikeC, ctx *testContext) {
	step(c, ctx, s.quickStartStep)
	step(c, ctx, s.addRelationStep)
	step(c, ctx, s.addRelUnitStep)
	step(c, ctx, s.waitHooksStep)
	step(c, ctx, s.verifyRunningStep)
}

type startupRelationError struct {
	badHook string
	// sub-steppers
	createCharmStep   *createCharm
	serveCharmStep    *serveCharm
	createUniterStep  *createUniter
	waitUnitAgentStep *waitUnitAgent
	waitHooksStep     *waitHooks
	verifyCharmStep   *verifyCharm
	addRelationStep   *addRelation
	addRelUnitStep    *addRelationUnit
}

func (s *startupRelationError) prepare(c tc.LikeC, ctx *testContext) {
	s.createCharmStep = &createCharm{badHooks: []string{s.badHook}}
	s.createCharmStep.prepare(c, ctx)

	s.serveCharmStep = &serveCharm{}
	s.serveCharmStep.prepare(c, ctx)

	s.createUniterStep = &createUniter{}
	s.createUniterStep.prepare(c, ctx)

	s.waitUnitAgentStep = &waitUnitAgent{status: status.Idle}
	s.waitUnitAgentStep.prepare(c, ctx)

	wh := waitHooks(startupHooks(false))
	s.waitHooksStep = &wh
	s.waitHooksStep.prepare(c, ctx)

	s.verifyCharmStep = &verifyCharm{}
	s.verifyCharmStep.prepare(c, ctx)

	s.addRelationStep = &addRelation{}
	s.addRelationStep.prepare(c, ctx)

	s.addRelUnitStep = &addRelationUnit{}
	s.addRelUnitStep.prepare(c, ctx)
}

func (s *startupRelationError) step(c tc.LikeC, ctx *testContext) {
	step(c, ctx, s.createCharmStep)
	step(c, ctx, s.serveCharmStep)
	step(c, ctx, s.createUniterStep)
	step(c, ctx, s.waitUnitAgentStep)
	step(c, ctx, s.waitHooksStep)
	step(c, ctx, s.verifyCharmStep)
	step(c, ctx, s.addRelationStep)
	step(c, ctx, s.addRelUnitStep)
}

type resolveError struct {
	resolved params.ResolvedMode
}

func (s *resolveError) prepare(_ tc.LikeC, _ *testContext) {}

func (s *resolveError) step(c tc.LikeC, ctx *testContext) {
	ctx.unit.mu.Lock()
	ctx.unit.resolved = s.resolved
	ctx.unit.mu.Unlock()
	ctx.sendNotify(c, ctx.unitResolveCh, "resolved event")
}

type statusfunc func() (status.StatusInfo, error)

var unitStatusGetter = func(ctx *testContext) statusfunc {
	return func() (status.StatusInfo, error) {
		ctx.unit.mu.Lock()
		defer ctx.unit.mu.Unlock()
		if ctx.unit.agentStatus.Status == status.Error {
			return ctx.unit.agentStatus, nil
		}
		return ctx.unit.unitStatus, nil
	}
}

var agentStatusGetter = func(ctx *testContext) statusfunc {
	return func() (status.StatusInfo, error) {
		ctx.unit.mu.Lock()
		defer ctx.unit.mu.Unlock()
		return ctx.unit.agentStatus, nil
	}
}

type waitUnitAgent struct {
	statusGetter func(ctx *testContext) statusfunc
	status       status.Status
	info         string
	data         map[string]any
	charm        int
	resolved     params.ResolvedMode
}

func (s *waitUnitAgent) prepare(_ tc.LikeC, _ *testContext) {}

func (s *waitUnitAgent) step(c tc.LikeC, ctx *testContext) {
	if s.statusGetter == nil {
		s.statusGetter = agentStatusGetter
	}
	c.Logf("waiting for desired status: %v")
	for {
		time.Sleep(coretesting.ShortWait)
		var (
			resolved params.ResolvedMode
			urlStr   *string
		)
		ctx.unit.mu.Lock()
		resolved = ctx.unit.resolved
		urlStr = new(ctx.unit.charmURL)
		ctx.unit.mu.Unlock()

		if resolved != s.resolved {
			c.Logf("want resolved mode %q, got %q; still waiting", s.resolved, resolved)
			continue
		}
		if urlStr == nil {
			c.Logf("want unit charm %q, got nil; still waiting", curl(s.charm))
			continue
		}
		if *urlStr != curl(s.charm) {
			c.Logf("want unit charm %q, got %q; still waiting", curl(s.charm), *urlStr)
			continue
		}
		statusInfo, err := s.statusGetter(ctx)()
		c.Assert(err, tc.ErrorIsNil)
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
	}
}

type waitHooks []string

func (s *waitHooks) prepare(_ tc.LikeC, _ *testContext) {}

func (s *waitHooks) step(c tc.LikeC, ctx *testContext) {
	if len(*s) == 0 {
		// Give unwanted hooks a moment to run...
		time.Sleep(coretesting.ShortWait)
	}
	ctx.hooks = append(ctx.hooks, *s...)
	c.Logf("waiting for hooks: %#v", ctx.hooks)
	match, cannotMatch, overshoot := ctx.matchHooks(c)
	if overshoot && len(*s) == 0 {
		c.Fatalf("ran more hooks than expected")
	}
	if cannotMatch {
		c.Fatalf("hooks did not match expected")
	}
	waitExecutionLockReleased := func() {
		timeout := make(chan struct{})
		go func() {
			<-time.After(coretesting.LongWait)
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
		if len(*s) > 0 {
			// only check for lock release if there were hooks
			// run; hooks *not* running may be due to the lock
			// being held.
			waitExecutionLockReleased()
		}
		return
	}
	c.Log("waiting for expected hooks")
	for {
		time.Sleep(coretesting.ShortWait)
		if match, cannotMatch, _ = ctx.matchHooks(c); match {
			waitExecutionLockReleased()
			return
		} else if cannotMatch {
			c.Fatalf("unexpected hook triggered")
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

func (s *waitActionInvocation) prepare(_ tc.LikeC, _ *testContext) {}

func (s *waitActionInvocation) step(c tc.LikeC, ctx *testContext) {
	c.Log("waiting for action invocation")
	for {
		time.Sleep(coretesting.ShortWait)
		ranActions := ctx.runner.ranActions()
		if len(ranActions) != len(s.expectedActions) {
			continue
		}
		assertActionsMatch(c, ranActions, s.expectedActions)
		return
	}
}

func assertActionsMatch(c tc.LikeC, actualIn []actionData, expectIn []actionData) {
	matches := 0
	desiredMatches := len(actualIn)
	c.Assert(len(actualIn), tc.Equals, len(expectIn))
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
		c.Assert(actualIn, tc.DeepEquals, expectIn)
	}
	c.Assert(matches, tc.Equals, desiredMatches)
}

type fixHook struct {
	name string
}

func (s *fixHook) prepare(_ tc.LikeC, _ *testContext) {}

func (s *fixHook) step(_ tc.LikeC, ctx *testContext) {
	if ctx.runner.hooksWithErrors != nil {
		ctx.runner.hooksWithErrors.Remove(s.name)
	}
}

type updateStatusHookTick struct{}

func (s *updateStatusHookTick) prepare(_ tc.LikeC, _ *testContext) {}

func (s *updateStatusHookTick) step(c tc.LikeC, ctx *testContext) {
	err := ctx.updateStatusHookTicker.Tick()
	c.Assert(err, tc.ErrorIsNil)
}

type changeConfig map[string]any

func (s *changeConfig) prepare(_ tc.LikeC, _ *testContext) {}

func (s *changeConfig) step(c tc.LikeC, ctx *testContext) {
	ctx.sendStrings(c, ctx.configCh, "config change event", ctx.app.configHash(*s))
}

type addAction struct {
	name   string
	params map[string]any
	tag    names.ActionTag
	action *apiuniter.Action
}

func (s *addAction) prepare(_ tc.LikeC, ctx *testContext) {
	s.tag = names.NewActionTag(strconv.Itoa(int(ctx.actionCounter.Add(1))))
	s.action = apiuniter.NewAction(s.tag.Id(), s.name, s.params, false, "")
	stepped := ctx.stepped.EXPECT().Stepped(s)
	ctx.api.EXPECT().Action(gomock.Any(), s.tag).Return(s.action, nil).AnyTimes().After(stepped)
	ctx.api.EXPECT().ActionBegin(gomock.Any(), s.tag).DoAndReturn(func(_ context.Context, tag names.ActionTag) error {
		ctx.actionsCh <- []string{tag.Id()}
		return nil
	}).MaxTimes(2).After(stepped)
	ctx.api.EXPECT().ActionStatus(gomock.Any(), s.tag).Return("completed", nil).AnyTimes().After(stepped)
}

func (s *addAction) step(c tc.LikeC, ctx *testContext) {
	// Add to pendingActions in step (not prepare) so it's not included in the
	// WatchActionNotifications initial event before the action gate is released.
	ctx.pendingActions = append(ctx.pendingActions, s.action)
	c.Logf("beginning action %s", s.tag)
	// Advance prepared mocks.
	ctx.stepped.Stepped(s)
	ctx.sendStrings(c, ctx.actionsCh, "action begin event", s.tag.Id())
}

type upgradeCharm struct {
	revision int
	forced   bool
}

func (s *upgradeCharm) prepare(_ tc.LikeC, _ *testContext) {}

func (s *upgradeCharm) step(c tc.LikeC, ctx *testContext) {
	ctx.app.mu.Lock()
	defer ctx.app.mu.Unlock()
	ctx.app.charmURL = curl(s.revision)
	ctx.app.charmForced = s.forced
	ctx.app.charmModifiedVersion++
	// Make sure we upload the charm before changing it in the DB.
	(&serveCharm{}).step(c, ctx)
	ctx.sendNotify(c, ctx.applicationCh, "application charm upgrade event")
}

type verifyCharm struct {
	revision          int
	attemptedRevision int
	checkFiles        filetesting.Entries
}

func (s *verifyCharm) prepare(_ tc.LikeC, _ *testContext) {}

func (s *verifyCharm) step(c tc.LikeC, ctx *testContext) {
	s.checkFiles.Check(c, filepath.Join(ctx.path, "charm"))
	checkRevision := max(s.attemptedRevision, s.revision)
	ctx.unit.mu.Lock()
	defer ctx.unit.mu.Unlock()
	urlStr := ctx.unit.charmURL
	c.Assert(urlStr, tc.Equals, curl(checkRevision))
}

type pushResource struct{}

func (s *pushResource) prepare(_ tc.LikeC, _ *testContext) {}

func (s *pushResource) step(c tc.LikeC, ctx *testContext) {
	ctx.app.mu.Lock()
	ctx.app.charmModifiedVersion++
	ctx.app.mu.Unlock()
	ctx.sendNotify(c, ctx.applicationCh, "resource change event")
}

type startUpgradeError struct {
	steps []stepper
}

func (s *startUpgradeError) prepare(c tc.LikeC, ctx *testContext) {
	wh := waitHooks(startupHooks(false))
	s.steps = []stepper{
		&createCharm{},
		&serveCharm{},
		&createUniter{},
		&waitUnitAgent{
			status: status.Idle,
		},
		&wh,
		&verifyCharm{},

		&createCharm{
			revision: 1,
			customize: func(c tc.LikeC, ctx *testContext, path string) {
				ctx.deployer.err = charm.ErrConflict
			},
		},
		&serveCharm{},
		&upgradeCharm{revision: 1},
		&waitUnitAgent{
			statusGetter: unitStatusGetter,
			status:       status.Error,
			info:         "upgrade failed",
			charm:        1,
		},
		&verifyWaiting{},
		&verifyCharm{attemptedRevision: 1},
	}
	for _, s_ := range s.steps {
		s_.prepare(c, ctx)
	}
}

func (s *startUpgradeError) step(c tc.LikeC, ctx *testContext) {
	for _, s_ := range s.steps {
		step(c, ctx, s_)
	}
}

type verifyWaitingUpgradeError struct {
	revision int
}

func (s *verifyWaitingUpgradeError) prepare(_ tc.LikeC, _ *testContext) {}

func (s *verifyWaitingUpgradeError) step(c tc.LikeC, ctx *testContext) {
	verifyCharmSteps := []stepper{
		&waitUnitAgent{
			statusGetter: unitStatusGetter,
			status:       status.Error,
			info:         "upgrade failed",
			charm:        s.revision,
		},
		&verifyCharm{attemptedRevision: s.revision},
	}
	verifyWaitingSteps := []stepper{
		&stopUniter{},
		&custom{f: func(c tc.LikeC, ctx *testContext) {
			// By setting status to Idle, and waiting for the restarted uniter
			// to reset the error status, we can avoid a race in which a subsequent
			// fixUpgradeError lands just before the restarting uniter retries the
			// upgrade; and thus puts us in an unexpected state for future steps.
			ctx.unit.mu.Lock()
			ctx.unit.agentStatus = status.StatusInfo{
				Status: status.Idle,
			}
			ctx.unit.mu.Unlock()
		}},
		&startUniter{rebootQuerier: &fakeRebootQuerier{rebootNotDetected}},
	}
	allSteps := append(verifyCharmSteps, verifyWaitingSteps...)
	allSteps = append(allSteps, verifyCharmSteps...)
	for _, s_ := range allSteps {
		s_.prepare(c, ctx)
	}
	for _, s_ := range allSteps {
		step(c, ctx, s_)
	}
}

type fixUpgradeError struct{}

func (s *fixUpgradeError) prepare(_ tc.LikeC, _ *testContext) {}

func (s *fixUpgradeError) step(_ tc.LikeC, ctx *testContext) {
	ctx.deployer.err = nil
}

type addRelation struct {
	waitHooksStep *waitHooks
	relation      *relation     // set in prepare, assigned to ctx.relation in step
	relUnit       *relationUnit // set in prepare, assigned to ctx.relUnit in step
}

func (s *addRelation) prepare(c tc.LikeC, ctx *testContext) {
	if ctx.relation != nil {
		panic("don't add two relations!")
	}
	if ctx.relatedApplication == nil {
		ctx.relatedApplication = apimocks.NewMockApplication(ctx.ctrl)
		ctx.relatedApplication.EXPECT().Tag().Return(names.NewApplicationTag("mysql")).AnyTimes()
	}

	relTag := names.NewRelationTag("wordpress:db mysql:db")
	// Store in s.relation (not ctx.relation) so WatchRelations initial event
	// does not include this relation before the gate is released.
	s.relation = ctx.makeRelation(c, relTag, life.Alive, "mysql")
	s.relUnit = ctx.makeRelationUnit(c, s.relation, ctx.unit)

	stepped := ctx.stepped.EXPECT().Stepped(s)
	s.relation.EXPECT().Unit(gomock.Any(), ctx.unit.Tag()).Return(s.relUnit, nil).AnyTimes().After(stepped)

	rel := s.relation // capture for closure
	ctx.api.EXPECT().WatchRelationUnits(gomock.Any(), relTag, ctx.unit.Tag()).DoAndReturn(func(_ context.Context, _ names.RelationTag, _ names.UnitTag) (watcher.RelationUnitsWatcher, error) {
		ctx.stateMu.Lock()
		defer ctx.stateMu.Unlock()

		changes := watcher.RelationUnitsChange{Changed: make(map[string]watcher.UnitSettings)}
		relUnits := ctx.relationUnits[rel.Id()]
		for u, vers := range relUnits {
			changes.Changed[u] = watcher.UnitSettings{Version: vers}
		}
		ctx.sendRelationUnitChange(c, "initial relation unit change", changes)
		w := newMockRelationUnitsWatcher(ctx.relationUnitCh)
		return w, nil
	}).AnyTimes().After(stepped)

	wh := waitHooks{"db-relation-created mysql db:0"}
	s.waitHooksStep = &wh
	s.waitHooksStep.prepare(c, ctx)
}

func (s *addRelation) step(c tc.LikeC, ctx *testContext) {
	// Assign to ctx now that the uniter is running and the initial
	// WatchRelations event has already been sent (with no relation).
	ctx.relation = s.relation
	ctx.relUnit = s.relUnit
	// Advance prepared mocks BEFORE sending the relation event. The
	// RemoteStateWatcher processes relCh asynchronously and may call
	// WatchRelationUnits before Stepped(s) is called, which would
	// cause a gomock unexpected-call failure via Goexit, permanently
	// deadlocking the watcher's catacomb (Kill is never called).
	ctx.stepped.Stepped(s)
	ctx.sendStrings(c, ctx.relCh, "relation event", ctx.relation.Tag().Id())
	step(c, ctx, s.waitHooksStep)
}

type addRelationUnit struct{}

func (s *addRelationUnit) prepare(_ tc.LikeC, _ *testContext) {}

func (s *addRelationUnit) step(c tc.LikeC, ctx *testContext) {
	related := fmt.Sprintf("%s/%d", ctx.relatedApplication.Tag().Id(), ctx.relUnitCounter.Add(1))
	ctx.stateMu.Lock()
	defer ctx.stateMu.Unlock()

	relUnitData, ok := ctx.relationUnits[ctx.relation.Id()]
	if !ok {
		relUnitData = make(relationUnitSettings)
		ctx.relationUnits[ctx.relation.Id()] = relUnitData
	}
	relUnitData[related] = 123
	changes := watcher.RelationUnitsChange{Changed: make(map[string]watcher.UnitSettings)}
	for u, vers := range relUnitData {
		changes.Changed[u] = watcher.UnitSettings{Version: vers}
	}
	ctx.sendRelationUnitChange(c, "relation unit add event", changes)
}

type changeRelationUnit struct {
	name string
}

func (s *changeRelationUnit) prepare(_ tc.LikeC, _ *testContext) {}

func (s *changeRelationUnit) step(c tc.LikeC, ctx *testContext) {
	ctx.stateMu.Lock()
	defer ctx.stateMu.Unlock()

	relUnitData, ok := ctx.relationUnits[ctx.relation.Id()]
	if !ok {
		relUnitData = make(relationUnitSettings)
		ctx.relationUnits[ctx.relation.Id()] = relUnitData
	}
	vers := relUnitData[s.name] + 1
	relUnitData[s.name] = vers
	changes := watcher.RelationUnitsChange{Changed: map[string]watcher.UnitSettings{
		s.name: {Version: vers},
	}}

	ctx.sendRelationUnitChange(c, "relation unit change event", changes)
}

type removeRelationUnit struct {
	name string
}

func (s *removeRelationUnit) prepare(_ tc.LikeC, _ *testContext) {}

func (s *removeRelationUnit) step(c tc.LikeC, ctx *testContext) {
	ctx.stateMu.Lock()
	defer ctx.stateMu.Unlock()

	relUnitData, ok := ctx.relationUnits[ctx.relation.Id()]
	if ok {
		delete(relUnitData, s.name)
	}
	changes := watcher.RelationUnitsChange{}
	changes.Departed = []string{s.name}

	ctx.sendRelationUnitChange(c, "relation unit depart event", changes)
}

type relationState struct {
	removed bool
	life    life.Value
}

func (s *relationState) prepare(_ tc.LikeC, _ *testContext) {}

func (s *relationState) step(c tc.LikeC, ctx *testContext) {
	if s.removed {
		c.Assert(ctx.relation.Life(), tc.Equals, life.Dying)
		return
	}
	c.Assert(ctx.relation.Life(), tc.Equals, s.life)
}

type addSubordinateRelation struct {
	ifce      string
	subordRel *relation // set in prepare, assigned to ctx.subordRelation in step
}

func (s *addSubordinateRelation) prepare(c tc.LikeC, ctx *testContext) {
	relKey := subordinateRelationKey(s.ifce)
	relTag := names.NewRelationTag(relKey)
	// Store in s.subordRel (not ctx.subordRelation) so WatchRelations initial
	// event does not include this relation before the gate is released.
	s.subordRel = ctx.makeRelation(c, relTag, life.Alive, "logging")

	ru := ctx.makeRelationUnit(c, s.subordRel, ctx.unit)
	stepped := ctx.stepped.EXPECT().Stepped(s)
	s.subordRel.EXPECT().Unit(gomock.Any(), ctx.unit.Tag()).Return(ru, nil).AnyTimes().After(stepped)

	ctx.api.EXPECT().WatchRelationUnits(gomock.Any(), relTag, ctx.unit.Tag()).DoAndReturn(func(_ context.Context, _ names.RelationTag, _ names.UnitTag) (watcher.RelationUnitsWatcher, error) {
		changes := watcher.RelationUnitsChange{Changed: make(map[string]watcher.UnitSettings)}
		changes.AppChanged = map[string]int64{"logging": 0}
		ctx.sendRelationUnitChange(c, "initial subordinate relation unit change", changes)
		w := newMockRelationUnitsWatcher(ctx.relationUnitCh)
		return w, nil
	}).AnyTimes().After(stepped)
}

func (s *addSubordinateRelation) step(c tc.LikeC, ctx *testContext) {
	// Assign to ctx now that the uniter is running.
	ctx.subordRelation = s.subordRel
	relKey := subordinateRelationKey(s.ifce)
	relTag := names.NewRelationTag(relKey)
	// Advance prepared mocks BEFORE sending the relation event (same
	// reasoning as addRelation.step).
	ctx.stepped.Stepped(s)
	ctx.sendStrings(c, ctx.relCh, "add subordinate relation event", relTag.Id())
}

type removeSubordinateRelation struct {
	ifce string
}

func (s *removeSubordinateRelation) prepare(_ tc.LikeC, _ *testContext) {}

func (s *removeSubordinateRelation) step(c tc.LikeC, ctx *testContext) {
	ctx.subordRelation.mu.Lock()
	ctx.subordRelation.life = life.Dying
	ctx.subordRelation.mu.Unlock()
	ctx.sendStrings(c, ctx.relCh, "remove subordinate relation event", subordinateRelationKey(s.ifce))
}

type waitSubordinateExists struct {
	name string
}

func (s *waitSubordinateExists) prepare(_ tc.LikeC, _ *testContext) {}

func (s *waitSubordinateExists) step(c tc.LikeC, ctx *testContext) {
	// First wait for the principal unit to enter scope.
	// If subordinate is not alive, test does not allow the
	// principal to enter scope.
	c.Log("waiting for subordinate unit to enter scope")
	for {
		time.Sleep(coretesting.ShortWait)
		subordLife := life.Dying
		ctx.unit.mu.Lock()
		inScope := ctx.unit.inScope
		if ctx.unit.subordinate != nil {
			subordLife = ctx.unit.subordinate.Life()
		}
		ctx.unit.mu.Unlock()
		if subordLife == life.Alive && !inScope {
			c.Logf("unit is alive and not yet in scope")
			continue
		}
		break
	}

	subordTag := names.NewUnitTag("logging/0")
	ctx.unit.mu.Lock()
	ctx.unit.subordinate = ctx.makeUnit(c, subordTag, life.Alive)
	ctx.unit.mu.Unlock()
	ctx.sendUnitNotify(c, "subordinate exists")

	changes := watcher.RelationUnitsChange{Changed: make(map[string]watcher.UnitSettings)}
	changes.Changed = map[string]watcher.UnitSettings{
		s.name: {Version: 666},
	}
	ctx.sendRelationUnitChange(c, "subordinate relation unit change", changes)
}

type waitSubordinateDying struct{}

func (s *waitSubordinateDying) prepare(_ tc.LikeC, _ *testContext) {}

func (s *waitSubordinateDying) step(c tc.LikeC, ctx *testContext) {
	c.Log("waiting for subordinate to be made Dying")
	for {
		time.Sleep(coretesting.ShortWait)
		ctx.unit.mu.Lock()
		subordLife := ctx.unit.subordinate.Life()
		ctx.unit.mu.Unlock()
		if subordLife != life.Dying {
			c.Logf("subordinate life is %q, not %q", subordLife, life.Dying)
			continue
		}
		break
	}
}

type removeSubordinate struct{}

func (s *removeSubordinate) prepare(_ tc.LikeC, _ *testContext) {}

func (s *removeSubordinate) step(c tc.LikeC, ctx *testContext) {
	ctx.unit.mu.Lock()
	ctx.unit.subordinate = nil
	ctx.unit.mu.Unlock()
	changes := watcher.RelationUnitsChange{Changed: make(map[string]watcher.UnitSettings)}
	changes.Departed = []string{"logging/0"}
	ctx.sendRelationUnitChange(c, "remove subordinate relation unit change", changes)
	ctx.sendUnitNotify(c, "subordinate removed event")
}

type writeFile struct {
	path string
	mode os.FileMode
}

func (s *writeFile) prepare(_ tc.LikeC, _ *testContext) {}

func (s *writeFile) step(c tc.LikeC, ctx *testContext) {
	path := filepath.Join(ctx.path, s.path)
	dir := filepath.Dir(path)
	err := os.MkdirAll(dir, 0755)
	c.Assert(err, tc.ErrorIsNil)
	err = os.WriteFile(path, nil, s.mode)
	c.Assert(err, tc.ErrorIsNil)
}

type removeCharmDir struct{}

func (s *removeCharmDir) prepare(_ tc.LikeC, _ *testContext) {}

func (s *removeCharmDir) step(c tc.LikeC, ctx *testContext) {
	path := filepath.Join(ctx.path, "charm")
	err := os.RemoveAll(path)
	c.Assert(err, tc.ErrorIsNil)
}

type custom struct {
	prepareFn func(tc.LikeC, *testContext)
	f         func(tc.LikeC, *testContext)
}

func (s *custom) prepare(c tc.LikeC, ctx *testContext) {
	if s.prepareFn != nil {
		s.prepareFn(c, ctx)
	}
}

func (s *custom) step(c tc.LikeC, ctx *testContext) {
	s.f(c, ctx)
}

var relationDying = &custom{f: func(c tc.LikeC, ctx *testContext) {
	ctx.relation.mu.Lock()
	ctx.relation.life = life.Dying
	ctx.relation.mu.Unlock()
	ctx.sendStrings(c, ctx.relCh, "relation dying event", ctx.relation.Tag().Id())
}}

type unitDying struct {
	stepped *MockSteppedSteppedCall
}

// prepare registers mock expectations for the unit dying scenario.
//
// All expectations are gated on the stepped call to prevent a race: the outer
// resolver loop can restart via ErrRestart, calling restartWatcher and
// unit.Watch, which sends an initial unit event. If that event is processed
// after ctx.unit.life is set to Dying but before the gate is opened, the
// storage resolver would call DestroyUnitStorageAttachments with no active
// expectation; gomock calls FailNow → runtime.Goexit in the uniter goroutine,
// permanently deadlocking the catacomb WaitGroup. Gating on stepped ensures
// the expectations are active before life is set. Each resolver restart
// creates a fresh storageResolver (s.dying = false), so calls can occur any
// number of times; AnyTimes() handles all cases.
func (s *unitDying) prepare(_ tc.LikeC, ctx *testContext) {
	s.stepped = ctx.stepped.EXPECT().Stepped(s)
	ctx.api.EXPECT().DestroyUnitStorageAttachments(gomock.Any(), ctx.unit.Tag()).Return(nil).AnyTimes().After(s.stepped)
	ctx.api.EXPECT().RemoveStorageAttachment(gomock.Any(), gomock.Any(), ctx.unit.Tag()).DoAndReturn(
		func(_ context.Context, tag names.StorageTag, _ names.UnitTag) error {
			ctx.stateMu.Lock()
			delete(ctx.storage, tag.Id())
			ctx.stateMu.Unlock()
			return nil
		}).AnyTimes().After(s.stepped)
}

func (s *unitDying) step(c tc.LikeC, ctx *testContext) {
	ctx.stepped.Stepped(s)
	ctx.unit.mu.Lock()
	ctx.unit.life = life.Dying
	ctx.unit.mu.Unlock()
	ctx.sendUnitNotify(c, "send unit dying event")
}

var unitDead = &custom{f: func(c tc.LikeC, ctx *testContext) {
	ctx.unit.mu.Lock()
	ctx.unit.life = life.Dead
	ctx.unit.mu.Unlock()
	ctx.sendUnitNotify(c, "send unit dead event")
}}

var subordinateDying = &custom{f: func(c tc.LikeC, ctx *testContext) {
	ctx.unit.mu.Lock()
	ctx.unit.subordinate.mu.Lock()
	ctx.unit.subordinate.life = life.Dying
	ctx.unit.subordinate.mu.Unlock()
	ctx.unit.mu.Unlock()
	ctx.sendStrings(c, ctx.relCh, "subord relation dying change", ctx.subordRelation.Tag().Id())
}}

func curl(revision int) string {
	// This functionality is highly depended on by the local
	// defaultCharmOrigin function. Any changes must be made
	// in both locations.
	return jujucharm.MustParseURL("ch:quantal/wordpress").WithRevision(revision).String()
}

type hookLock struct {
	releaser func()
}

type hookStep struct {
	stepFunc func(tc.LikeC, *testContext)
}

func (h *hookStep) prepare(_ tc.LikeC, _ *testContext) {}

func (h *hookStep) step(c tc.LikeC, ctx *testContext) {
	h.stepFunc(c, ctx)
}

func (h *hookLock) acquire() *hookStep {
	return &hookStep{stepFunc: func(c tc.LikeC, ctx *testContext) {
		releaser, err := processLock.Acquire(machinelock.Spec{
			Worker:  "uniter-test",
			Comment: "hookLock",
			Cancel:  make(chan struct{}), // clearly suboptimal
		})
		c.Assert(err, tc.ErrorIsNil)
		h.releaser = releaser
	}}
}

func (h *hookLock) release() *hookStep {
	return &hookStep{stepFunc: func(c tc.LikeC, ctx *testContext) {
		c.Assert(h.releaser, tc.NotNil)
		h.releaser()
		h.releaser = nil
	}}
}

type runCommands []string

func (s *runCommands) prepare(_ tc.LikeC, _ *testContext) {}

func (s *runCommands) step(c tc.LikeC, ctx *testContext) {
	commands := strings.Join(*s, "\n")
	args := uniter.RunCommandsArgs{
		Commands:       commands,
		RelationId:     -1,
		RemoteUnitName: "",
		UnitName:       "u/0",
	}
	result, err := ctx.uniter.RunCommands(args)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(result.Code, tc.Equals, 0)
	c.Check(string(result.Stdout), tc.Equals, "test")
	c.Check(string(result.Stderr), tc.Equals, "")
}

type forceMinion struct{}

func (s *forceMinion) prepare(_ tc.LikeC, _ *testContext) {}

func (s *forceMinion) step(c tc.LikeC, ctx *testContext) {
	ctx.leaderTracker.setLeader(false)
}

type forceLeader struct{}

func (s *forceLeader) prepare(_ tc.LikeC, _ *testContext) {}

func (s *forceLeader) step(c tc.LikeC, ctx *testContext) {
	ctx.leaderTracker.setLeader(true)
}

func newMockLeaderTracker(ctx *testContext, minion bool) *mockLeaderTracker {
	return &mockLeaderTracker{
		ctx:      ctx,
		isLeader: !minion,
	}
}

type mockLeaderTracker struct {
	mu       sync.Mutex
	ctx      *testContext
	isLeader bool
	waiting  []chan struct{}
}

func (mock *mockLeaderTracker) Kill() {}

func (mock *mockLeaderTracker) Wait() error {
	return nil
}

func (mock *mockLeaderTracker) ApplicationName() string {
	return mock.ctx.app.Tag().Id()
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

func (mock *mockLeaderTracker) setLeader(isLeader bool) {
	mock.mu.Lock()
	defer mock.mu.Unlock()
	if mock.isLeader == isLeader {
		return
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

type mockCharmDirGuard struct{}

// Unlock implements fortress.Guard.
func (*mockCharmDirGuard) Unlock(context.Context) error { return nil }

// Lockdown implements fortress.Guard.
func (*mockCharmDirGuard) Lockdown(context.Context) error { return nil }

type provisionStorage struct{}

func (s *provisionStorage) prepare(_ tc.LikeC, _ *testContext) {}

func (s *provisionStorage) step(c tc.LikeC, ctx *testContext) {
	ctx.stateMu.Lock()
	defer ctx.stateMu.Unlock()
	for si := range ctx.storage {
		ctx.storage[si].attached = true
	}
	var ids []string
	for si := range ctx.storage {
		ids = append(ids, si)
	}
	ctx.sendStrings(c, ctx.storageCh, "storage event", ids...)
	for _, attachment := range ctx.storage {
		ctx.sendNotify(c, attachment.eventCh, "storage attach event")
	}
}

type destroyStorageAttachment struct{}

func (s *destroyStorageAttachment) prepare(_ tc.LikeC, _ *testContext) {}

func (s *destroyStorageAttachment) step(c tc.LikeC, ctx *testContext) {
	ctx.stateMu.Lock()
	ctx.storage = make(map[string]*storageAttachment)
	ctx.stateMu.Unlock()
}

type verifyStorageDetached struct{}

func (s *verifyStorageDetached) prepare(_ tc.LikeC, _ *testContext) {}

func (s *verifyStorageDetached) step(c tc.LikeC, ctx *testContext) {
	ctx.stateMu.Lock()
	defer ctx.stateMu.Unlock()
	c.Assert(ctx.storage, tc.HasLen, 0)
}

type createSecret struct {
	uri *secrets.URI
}

func (s *createSecret) prepare(_ tc.LikeC, ctx *testContext) {
	s.uri = secrets.NewURI()
	ctx.createdSecretURI = s.uri
	stepped := ctx.stepped.EXPECT().Stepped(s)
	ctx.secretBackends.EXPECT().GetContent(gomock.Any(), s.uri, "foorbar", false, false).Return(
		secrets.NewSecretValue(map[string]string{"foo": "bar"}), nil).AnyTimes().After(stepped)
}

func (s *createSecret) step(_ tc.LikeC, ctx *testContext) {
	// Advance prepared mocks.
	ctx.stepped.Stepped(s)
}

type changeSecret struct{}

func (s *changeSecret) prepare(_ tc.LikeC, ctx *testContext) {
	stepped := ctx.stepped.EXPECT().Stepped(s)
	ctx.secretsClient.EXPECT().GetConsumerSecretsRevisionInfo(
		gomock.Any(),
		ctx.unit.Name(), []string{ctx.createdSecretURI.String()},
	).Return(map[string]secrets.SecretRevisionInfo{
		ctx.createdSecretURI.String(): {LatestRevision: 666},
	}, nil).After(stepped)
}

func (s *changeSecret) step(c tc.LikeC, ctx *testContext) {
	// Advance prepared mocks.
	ctx.stepped.Stepped(s)
	ctx.sendStrings(c, ctx.consumedSecretsCh, "secret change", ctx.createdSecretURI.String())
	done := make(chan bool)
	go func() {
		for {
			ctx.stateMu.Lock()
			if strings.Contains(ctx.secretsState, fmt.Sprintf("  %s: 666\n", ctx.createdSecretURI)) {
				ctx.stateMu.Unlock()
				close(done)
				return
			}
			ctx.stateMu.Unlock()
			time.Sleep(coretesting.ShortWait)
		}
	}()
	ctx.waitFor(c, done, "waiting for secret state to be updated")
}

type getSecret struct{}

func (s *getSecret) prepare(_ tc.LikeC, _ *testContext) {}

func (s *getSecret) step(c tc.LikeC, ctx *testContext) {
	val, err := ctx.secretBackends.GetContent(c.Context(), ctx.createdSecretURI, "foorbar", false, false)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(val.EncodedValues(), tc.DeepEquals, map[string]string{"foo": "bar"})
}

type rotateSecret struct {
	rev int
}

func (s *rotateSecret) prepare(_ tc.LikeC, _ *testContext) {}

func (s *rotateSecret) step(c tc.LikeC, ctx *testContext) {
	ctx.sendStrings(c, ctx.secretsRotateCh, "rotate secret change", ctx.createdSecretURI.String())
	done := make(chan bool)
	go func() {
		for {
			ctx.stateMu.Lock()
			rev := ctx.secretRevisions[ctx.createdSecretURI.String()]
			if rev == s.rev {
				ctx.stateMu.Unlock()
				close(done)
				return
			}
			ctx.stateMu.Unlock()
			time.Sleep(coretesting.ShortWait)
		}
	}()
	ctx.waitFor(c, done, "waiting for secret to be updated")
}

type expireSecret struct{}

func (s *expireSecret) prepare(_ tc.LikeC, _ *testContext) {}

func (s *expireSecret) step(c tc.LikeC, ctx *testContext) {
	ctx.sendStrings(c, ctx.secretsExpireCh, "expire secret change", ctx.createdSecretURI.String()+"/1")
}

type expectError struct {
	err string
}

func (s *expectError) prepare(_ tc.LikeC, _ *testContext) {}

func (s *expectError) step(_ tc.LikeC, ctx *testContext) {
	ctx.setExpectedError(s.err)
}

// manualTicker will be used to generate collect-metrics events
// in a time-independent manner for testing.
type manualTicker struct {
	c chan time.Time
}

// Tick sends a signal on the ticker channel.
func (t *manualTicker) Tick() error {
	t.c <- time.Now()
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

func (s *activateTestContainer) prepare(_ tc.LikeC, _ *testContext) {}

func (s *activateTestContainer) step(c tc.LikeC, ctx *testContext) {
	ctx.pebbleClients[s.containerName].TriggerStart()
}

type injectTestContainer struct {
	containerName string
}

func (s *injectTestContainer) prepare(_ tc.LikeC, _ *testContext) {}

func (s *injectTestContainer) step(c tc.LikeC, ctx *testContext) {
	c.Assert(ctx.uniter, tc.IsNil)
	ctx.containerNames = append(ctx.containerNames, s.containerName)
	if ctx.pebbleClients == nil {
		ctx.pebbleClients = make(map[string]*fakePebbleClient)
	}
	ctx.pebbleClients[s.containerName] = &fakePebbleClient{
		err:   errors.BadRequestf("not ready yet"),
		clock: testclock.NewClock(time.Time{}),
	}
}

type triggerShutdown struct {
}

func (s *triggerShutdown) prepare(_ tc.LikeC, _ *testContext) {}

func (s *triggerShutdown) step(c tc.LikeC, ctx *testContext) {
	err := ctx.uniter.Terminate()
	c.Assert(err, tc.ErrorIsNil)
}
