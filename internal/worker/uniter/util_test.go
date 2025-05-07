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

	pebbleclient "github.com/canonical/pebble/client"
	"github.com/juju/clock/testclock"
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/mutex/v2"
	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"github.com/juju/utils/v4"
	"github.com/juju/worker/v4"
	"go.uber.org/mock/gomock"
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
	jujucharm "github.com/juju/juju/internal/charm"
	charmtesting "github.com/juju/juju/internal/charm/testing"
	"github.com/juju/juju/internal/downloader"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/testhelpers/filetesting"
	coretesting "github.com/juju/juju/internal/testing"
	jworker "github.com/juju/juju/internal/worker"
	"github.com/juju/juju/internal/worker/fortress"
	"github.com/juju/juju/internal/worker/uniter"
	uniterapi "github.com/juju/juju/internal/worker/uniter/api"
	"github.com/juju/juju/internal/worker/uniter/charm"
	"github.com/juju/juju/internal/worker/uniter/operation"
	"github.com/juju/juju/internal/worker/uniter/remotestate"
	"github.com/juju/juju/internal/worker/uniter/runner"
	runnercontext "github.com/juju/juju/internal/worker/uniter/runner/context"
	contextmocks "github.com/juju/juju/internal/worker/uniter/runner/context/mocks"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
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

	// API clients.
	api           *uniterapi.MockUniterClient
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
	unit  *unit
	app   *application
	charm *uniterapi.MockCharm

	relCounter atomic.Int32
	relation   *relation

	relUnitCounter atomic.Int32
	relUnit        *relationUnit

	relatedApplication *uniterapi.MockApplication

	subordRelation *relation

	// Data model aka "state".
	stateMu sync.Mutex

	machineProfiles []string
	storage         map[string]*storageAttachment
	relationUnits   map[int]relationUnitSettings
	actionCounter   atomic.Int32
	pendingActions  []*apiuniter.Action

	createdSecretURI *secrets.URI
	secretsRotateCh  chan []string
	secretsExpireCh  chan []string
	secretRevisions  map[string]int
	secretsClient    *uniterapi.MockSecretsClient
	secretBackends   *uniterapi.MockSecretsBackend

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

func (ctx *testContext) run(c *tc.C, steps []stepper) {
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
	for i, s := range steps {
		c.Logf("step %d:\n", i)
		step(c, ctx, s)
	}
}

func (ctx *testContext) waitFor(c *tc.C, ch chan bool, msg string) {
	select {
	case <-ch:
		return
	case <-time.After(coretesting.LongWait):
		c.Fatal(msg)
	}
}

func (ctx *testContext) sendUnitNotify(c *tc.C, msg string) {
	ctx.unitCh.Range(func(k, v any) bool {
		ctx.sendNotify(c, v.(chan struct{}), msg)
		return true
	})
}

func (ctx *testContext) sendNotify(c *tc.C, ch chan struct{}, msg string) {
	if !ctx.sendEvents || ctx.startError {
		return
	}
	select {
	case ch <- struct{}{}:
		c.Logf("sent: %s", msg)
		return
	case <-time.After(coretesting.LongWait):
		c.Fatalf("could not send: %s", msg)
		c.FailNow()
	}
}

func (ctx *testContext) sendStrings(c *tc.C, ch chan []string, msg string, s ...string) {
	if !ctx.sendEvents || ctx.startError {
		return
	}
	select {
	case ch <- s:
		c.Logf("sent: %s (%q)", msg, s)
		return
	case <-time.After(coretesting.LongWait):
		c.Fatalf("could not send: %s", msg)
		c.FailNow()
	}
}

func (ctx *testContext) sendRelationUnitChange(c *tc.C, msg string, ruc watcher.RelationUnitsChange) {
	select {
	case ctx.relationUnitCh <- ruc:
		c.Logf("sent: %s: %+v", msg, ruc)
	case <-time.After(coretesting.LongWait):
		c.Fatalf("could not send: %s", msg)
		c.FailNow()
	}
}

func (ctx *testContext) expectHookContext(c *tc.C) {
	ctx.api.EXPECT().APIAddresses(gomock.Any()).Return([]string{"10.6.6.6"}, nil).AnyTimes()
	ctx.api.EXPECT().CloudAPIVersion(gomock.Any()).Return("6.6.6", nil).AnyTimes()

	cfg := coretesting.ModelConfig(c)
	ctx.api.EXPECT().ModelConfig(gomock.Any()).Return(cfg, nil).AnyTimes()
	m, err := ctx.unit.AssignedMachine(context.Background())
	c.Assert(err, tc.ErrorIsNil)
	ctx.api.EXPECT().OpenedMachinePortRangesByEndpoint(gomock.Any(), m).Return(nil, nil).AnyTimes()
	ctx.secretsClient.EXPECT().SecretMetadata(gomock.Any()).Return(nil, nil).AnyTimes()
}

func (ctx *testContext) matchHooks(c *tc.C) (match, cannotMatch, overshoot bool) {
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
	step(c *tc.C, ctx *testContext)
}

func step(c *tc.C, ctx *testContext, s stepper) {
	c.Logf("%#v", s)
	s.step(c, ctx)
}

type createCharm struct {
	revision  int
	badHooks  []string
	customize func(*tc.C, *testContext, string)
}

func startupHooks(minion bool) []string {
	if minion {
		return []string{"install", "config-changed", "start"}
	}
	return []string{"install", "leader-elected", "config-changed", "start"}
}

func (s createCharm) step(c *tc.C, ctx *testContext) {
	base := testcharms.Repo.ClonedDirPath(c.MkDir(), "wordpress")
	if s.customize != nil {
		s.customize(c, ctx, base)
	}
	if len(s.badHooks) > 0 {
		ctx.runner.hooksWithErrors = set.NewStrings(s.badHooks...)
	}

	dir, err := charmtesting.ReadCharmDir(base)
	c.Assert(err, tc.ErrorIsNil)
	step(c, ctx, addCharm{dir, curl(s.revision), s.revision})
}

type addCharm struct {
	dir      *charmtesting.CharmDir
	curl     string
	revision int
}

func (s addCharm) step(c *tc.C, ctx *testContext) {
	var buf bytes.Buffer
	err := s.dir.ArchiveTo(&buf)
	c.Assert(err, tc.ErrorIsNil)
	body := buf.Bytes()
	hash, _, err := utils.ReadSHA256(&buf)
	c.Assert(err, tc.ErrorIsNil)

	storagePath := fmt.Sprintf("/charms/%s/%d", s.dir.Meta().Name, s.revision)
	ctx.charms[storagePath] = body
	ctx.charm = uniterapi.NewMockCharm(ctx.ctrl)
	ctx.charm.EXPECT().URL().Return(s.curl).AnyTimes()
	ctx.charm.EXPECT().ArchiveSha256(gomock.Any()).Return(hash, nil).AnyTimes()
	ctx.api.EXPECT().Charm(s.curl).Return(ctx.charm, nil).AnyTimes()
	ctx.charm.EXPECT().LXDProfileRequired(gomock.Any()).Return(s.dir.LXDProfile() != nil, nil).AnyTimes()
}

type serveCharm struct{}

func (s serveCharm) step(c *tc.C, ctx *testContext) {
	for storagePath, data := range ctx.charms {
		ctx.servedCharms[storagePath] = data
		delete(ctx.charms, storagePath)
	}
}

type addCharmProfileToMachine struct {
	profiles []string
}

func (acpm addCharmProfileToMachine) step(c *tc.C, ctx *testContext) {
	ctx.stateMu.Lock()
	ctx.machineProfiles = acpm.profiles
	ctx.stateMu.Unlock()
}

type createApplicationAndUnit struct {
	applicationName string
	storage         map[string]state.StorageConstraints
	container       bool
}

func (csau createApplicationAndUnit) step(c *tc.C, ctx *testContext) {
	if csau.applicationName == "" {
		csau.applicationName = "u"
	}
	unitTag := names.NewUnitTag(csau.applicationName + "/0")
	ctx.unit = ctx.makeUnit(c, unitTag, life.Alive)

	appTag := names.NewApplicationTag(csau.applicationName)
	ctx.app = ctx.makeApplication(appTag)

	ctx.storage = make(map[string]*storageAttachment)
	for si, info := range csau.storage {
		for n := 0; n < int(info.Count); n++ {
			tag := names.NewStorageTag(fmt.Sprintf("%s/%d", si, n))
			ctx.storage[tag.Id()] = &storageAttachment{
				eventCh: make(chan struct{}, 2),
			}
		}
	}

	// Assign the unit to a provisioned machine to match expected state.
	if csau.container {
		machineTag := names.NewMachineTag("0/lxd/0")
		ctx.unit.EXPECT().AssignedMachine(gomock.Any()).Return(machineTag, nil).AnyTimes()
	} else {
		machineTag := names.NewMachineTag("0")
		ctx.unit.EXPECT().AssignedMachine(gomock.Any()).Return(machineTag, nil).AnyTimes()
	}
	ctx.sendNotify(c, ctx.applicationCh, "application created event")
}

type deleteUnit struct{}

func (d deleteUnit) step(c *tc.C, ctx *testContext) {
	ctx.unit.mu.Lock()
	ctx.unit.life = life.Dead
	ctx.unit.mu.Unlock()
}

type createUniter struct {
	minion               bool
	startError           bool
	executorFunc         uniter.NewOperationExecutorFunc
	translateResolverErr func(error) error
}

func (s createUniter) step(c *tc.C, ctx *testContext) {
	ctx.startError = s.startError
	step(c, ctx, createApplicationAndUnit{})
	ctx.leaderTracker = newMockLeaderTracker(ctx, s.minion)
	if s.minion {
		step(c, ctx, forceMinion{})
	}
	step(c, ctx, startUniter{
		newExecutorFunc:      s.executorFunc,
		translateResolverErr: s.translateResolverErr,
		unit:                 ctx.unit.Name(),
	})
	step(c, ctx, waitAddresses{})
}

type waitAddresses struct{}

func (waitAddresses) step(c *tc.C, ctx *testContext) {
	timeout := time.After(coretesting.LongWait)
	for {
		select {
		case <-timeout:
			c.Fatalf("timed out waiting for unit addresses")
		case <-time.After(coretesting.ShortWait):
			private, _ := ctx.unit.PrivateAddress(context.Background())
			if private != dummyPrivateAddress.Value {
				continue
			}
			public, _ := ctx.unit.PublicAddress(context.Background())
			if public != dummyPublicAddress.Value {
				continue
			}
			return
		}
	}
}

type startUniter struct {
	unit                 string
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

type unitStateMatcher struct {
	c        *tc.C
	expected string
}

func (m unitStateMatcher) Matches(x interface{}) bool {
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

func (m uniterCharmUpgradeStateMatcher) Matches(x interface{}) bool {
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

func (m uniterRunHookStateMatcher) Matches(x interface{}) bool {
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

func (m uniterRunActionStateMatcher) Matches(x interface{}) bool {
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

func (m uniterContinueStateMatcher) Matches(x interface{}) bool {
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

func (m uniterSecretsStateMatcher) Matches(x interface{}) bool {
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

func (m uniterStorageStateMatcher) Matches(x interface{}) bool {
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

func (m uniterRelationStateMatcher) Matches(x interface{}) bool {
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

func (s *startUniter) expectRemoteStateWatchers(c *tc.C, ctx *testContext) {
	ctx.sendEvents = true
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
	}).AnyTimes()

	ctx.app.EXPECT().Watch(gomock.Any()).DoAndReturn(func(context.Context) (watcher.NotifyWatcher, error) {
		ctx.sendNotify(c, ctx.applicationCh, "initial application event")
		w := watchertest.NewMockNotifyWatcher(ctx.applicationCh)
		return w, nil
	}).AnyTimes()

	ctx.unit.EXPECT().WatchResolveMode(gomock.Any()).DoAndReturn(func(context.Context) (watcher.NotifyWatcher, error) {
		ctx.sendNotify(c, ctx.unitResolveCh, "initial resolve event")
		w := watchertest.NewMockNotifyWatcher(ctx.unitResolveCh)
		return w, nil
	}).AnyTimes()

	ctx.unit.EXPECT().WatchInstanceData(gomock.Any()).DoAndReturn(func(context.Context) (watcher.NotifyWatcher, error) {
		ch := make(chan struct{}, 1)
		ch <- struct{}{}
		w := watchertest.NewMockNotifyWatcher(ch)
		return w, nil
	}).AnyTimes()

	ctx.api.EXPECT().WatchUpdateStatusHookInterval(gomock.Any()).DoAndReturn(func(context.Context) (watcher.NotifyWatcher, error) {
		ch := make(chan struct{}, 1)
		ch <- struct{}{}
		w := watchertest.NewMockNotifyWatcher(ch)
		return w, nil
	}).AnyTimes()

	ctx.unit.EXPECT().WatchConfigSettingsHash(gomock.Any()).DoAndReturn(func(context.Context) (watcher.StringsWatcher, error) {
		ctx.sendStrings(c, ctx.configCh, "initial config event", ctx.app.configHash(nil))
		w := watchertest.NewMockStringsWatcher(ctx.configCh)
		return w, nil
	}).AnyTimes()

	ctx.unit.EXPECT().WatchTrustConfigSettingsHash(gomock.Any()).DoAndReturn(func(context.Context) (watcher.StringsWatcher, error) {
		ch := make(chan []string, 1)
		ch <- []string{"trust-hash"}
		w := watchertest.NewMockStringsWatcher(ch)
		return w, nil
	}).AnyTimes()

	ctx.unit.EXPECT().WatchAddressesHash(gomock.Any()).DoAndReturn(func(context.Context) (watcher.StringsWatcher, error) {
		ch := make(chan []string, 1)
		ch <- []string{"address-hash"}
		w := watchertest.NewMockStringsWatcher(ch)
		return w, nil
	}).AnyTimes()

	ctx.unit.EXPECT().WatchRelations(gomock.Any()).DoAndReturn(func(context.Context) (watcher.StringsWatcher, error) {
		var relations []string
		if ctx.relation != nil {
			relations = []string{ctx.relation.Tag().Id()}
		}
		ctx.sendStrings(c, ctx.relCh, "initial relation event", relations...)
		w := watchertest.NewMockStringsWatcher(ctx.relCh)
		return w, nil
	}).AnyTimes()

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
					OwnerTag:   ctx.unit.Tag().String(),
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
	}).AnyTimes()

	ctx.unit.EXPECT().WatchActionNotifications(gomock.Any()).DoAndReturn(func(context.Context) (watcher.StringsWatcher, error) {
		var actions []string
		for _, a := range ctx.pendingActions {
			actions = append(actions, a.ID())
		}
		ctx.sendStrings(c, ctx.actionsCh, "initial action event", actions...)
		w := watchertest.NewMockStringsWatcher(ctx.actionsCh)
		return w, nil
	}).AnyTimes()

	ctx.secretsClient.EXPECT().WatchConsumedSecretsChanges(gomock.Any(), ctx.unit.Name()).DoAndReturn(func(context.Context, string) (watcher.StringsWatcher, error) {
		ctx.sendStrings(c, ctx.consumedSecretsCh, "initial consumed secrets event")
		w := watchertest.NewMockStringsWatcher(ctx.consumedSecretsCh)
		return w, nil
	}).AnyTimes()

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
	}).AnyTimes()
}

func (s startUniter) setupUniter(c *tc.C, ctx *testContext) {
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
	}).AnyTimes()

	// Consumed secrets initial event.
	ctx.secretsClient.EXPECT().GetConsumerSecretsRevisionInfo(gomock.Any(), ctx.unit.Name(), []string(nil)).Return(nil, nil).AnyTimes()

	ctx.api.EXPECT().UpdateStatusHookInterval(gomock.Any()).Return(time.Minute, nil).AnyTimes()

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
	ctx.api.EXPECT().UnitStorageAttachments(gomock.Any(), tag).Return(attachments, nil).AnyTimes()
	ctx.api.EXPECT().Unit(gomock.Any(), tag).DoAndReturn(func(_ context.Context, tag names.UnitTag) (uniterapi.Unit, error) {
		if tag.Id() != ctx.unit.Tag().Id() {
			return nil, errors.New("permission denied")
		}
		return ctx.unit, nil
	}).AnyTimes()

	// Secrets init.
	ctx.secretsClient.EXPECT().SecretMetadata(gomock.Any()).Return(nil, nil).AnyTimes()
	ctx.secretsClient.EXPECT().SecretRotated(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, uri string, rev int) error {
		ctx.stateMu.Lock()
		ctx.secretRevisions[uri] = rev + 1
		ctx.stateMu.Unlock()
		return nil
	}).AnyTimes()

	// Context factory init.
	ctx.api.EXPECT().Model(gomock.Any()).Return(&types.Model{
		Name:      "test-model",
		UUID:      coretesting.ModelTag.Id(),
		ModelType: types.IAAS,
	}, nil).AnyTimes()

	// Set up the initial install op.
	data, err := yaml.Marshal(operation.State{
		CharmURL: ctx.charm.URL(),
		Kind:     "install",
		Step:     "pending",
	})
	c.Assert(err, tc.ErrorIsNil)
	st := string(data)
	ctx.unit.EXPECT().SetState(gomock.Any(), unitStateMatcher{c: c, expected: st}).Return(nil).MaxTimes(1)

	data, err = yaml.Marshal(operation.State{
		CharmURL: ctx.charm.URL(),
		Kind:     "install",
		Step:     "done",
	})
	c.Assert(err, tc.ErrorIsNil)
	st = string(data)
	ctx.unit.EXPECT().SetState(gomock.Any(), unitStateMatcher{c: c, expected: st}).Return(nil).MaxTimes(1)
}

func (s startUniter) setupUniterHookExec(c *tc.C, ctx *testContext) {
	ctx.api.EXPECT().Application(gomock.Any(), ctx.app.Tag()).Return(ctx.app, nil).AnyTimes()
	ctx.expectHookContext(c)

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
	ctx.unit.EXPECT().SetState(gomock.Any(), uniterCharmUpgradeStateMatcher{}).DoAndReturn(setState).AnyTimes()
	ctx.unit.EXPECT().SetState(gomock.Any(), uniterRunHookStateMatcher{}).DoAndReturn(setState).AnyTimes()
	ctx.unit.EXPECT().SetState(gomock.Any(), uniterRunActionStateMatcher{}).DoAndReturn(setState).AnyTimes()
	ctx.unit.EXPECT().SetState(gomock.Any(), uniterContinueStateMatcher{}).DoAndReturn(setState).AnyTimes()
	ctx.unit.EXPECT().SetState(gomock.Any(), uniterSecretsStateMatcher{}).DoAndReturn(setState).AnyTimes()
	ctx.unit.EXPECT().SetState(gomock.Any(), uniterStorageStateMatcher{}).DoAndReturn(setState).AnyTimes()
	ctx.unit.EXPECT().SetState(gomock.Any(), uniterRelationStateMatcher{}).DoAndReturn(setState).AnyTimes()
}

func (s startUniter) step(c *tc.C, ctx *testContext) {
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

	s.setupUniter(c, ctx)
	s.setupUniterHookExec(c, ctx)
	s.expectRemoteStateWatchers(c, ctx)

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
	var err error
	ctx.uniter, err = uniter.NewUniter(&uniterParams)
	c.Assert(err, tc.ErrorIsNil)
}

type waitUniterDead struct {
	err string
}

func (s waitUniterDead) step(c *tc.C, ctx *testContext) {
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
		step(c, ctx, startUniter{})
		err = s.waitDead(c, ctx)
	}
	c.Assert(err, tc.Equals, jworker.ErrTerminateAgent)
}

func (s waitUniterDead) waitDead(c *tc.C, ctx *testContext) error {
	u := ctx.uniter
	ctx.uniter = nil

	wait := make(chan error, 1)
	go func() {
		wait <- u.Wait()
	}()

	select {
	case err := <-wait:
		return err
	case <-time.After(coretesting.LongWait):
		u.Kill()
		c.Fatalf("uniter still alive")
	}
	panic("unreachable")
}

type stopUniter struct {
	err string
}

func (s stopUniter) step(c *tc.C, ctx *testContext) {
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

type verifyWaiting struct{}

func (s verifyWaiting) step(c *tc.C, ctx *testContext) {
	step(c, ctx, stopUniter{})
	step(c, ctx, startUniter{rebootQuerier: fakeRebootQuerier{rebootNotDetected}})
	step(c, ctx, waitHooks{})
}

type verifyRunning struct{}

func (s verifyRunning) step(c *tc.C, ctx *testContext) {
	step(c, ctx, stopUniter{})
	step(c, ctx, startUniter{rebootQuerier: fakeRebootQuerier{rebootNotDetected}})
	var hooks []string
	// We don't expect config-changed to always run on agent restart
	// anymore.
	step(c, ctx, waitHooks(hooks))
}

type startupError struct {
	badHook string
}

func (s startupError) step(c *tc.C, ctx *testContext) {
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

func (s verifyDeployed) step(c *tc.C, ctx *testContext) {
	c.Assert(ctx.deployer.staged, tc.DeepEquals, curl(0))
	c.Assert(ctx.deployer.deployed, tc.IsTrue)
}

type quickStart struct {
	minion bool
}

func (s quickStart) step(c *tc.C, ctx *testContext) {
	step(c, ctx, createCharm{})
	step(c, ctx, serveCharm{})
	step(c, ctx, createUniter{minion: s.minion})
	step(c, ctx, waitUnitAgent{status: status.Idle})
	step(c, ctx, waitHooks(startupHooks(s.minion)))
	step(c, ctx, verifyCharm{})
}

type quickStartRelation struct{}

func (s quickStartRelation) step(c *tc.C, ctx *testContext) {
	step(c, ctx, quickStart{})
	step(c, ctx, addRelation{})
	step(c, ctx, addRelationUnit{})
	step(c, ctx, waitHooks{"db-relation-joined mysql/0 db:0", "db-relation-changed mysql/0 db:0"})
	step(c, ctx, verifyRunning{})
}

type startupRelationError struct {
	badHook string
}

func (s startupRelationError) step(c *tc.C, ctx *testContext) {
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
	resolved params.ResolvedMode
}

func (s resolveError) step(c *tc.C, ctx *testContext) {
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
	data         map[string]interface{}
	charm        int
	resolved     params.ResolvedMode
}

func (s waitUnitAgent) step(c *tc.C, ctx *testContext) {
	if s.statusGetter == nil {
		s.statusGetter = agentStatusGetter
	}
	timeout := time.After(coretesting.LongWait)
	for {

		select {
		case <-time.After(coretesting.ShortWait):
			var (
				resolved params.ResolvedMode
				urlStr   *string
			)
			ctx.unit.mu.Lock()
			resolved = ctx.unit.resolved
			urlStr = ptr(ctx.unit.charmURL)
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
				c.Logf("want unit charm %q, got %q; still waiting", curl(s.charm), urlStr)
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
		case <-timeout:
			c.Fatalf("never reached desired status")
		}
	}
}

type waitHooks []string

func (s waitHooks) step(c *tc.C, ctx *testContext) {
	if len(s) == 0 {
		// Give unwanted hooks a moment to run...

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
		if len(s) > 0 {
			// only check for lock release if there were hooks
			// run; hooks *not* running may be due to the lock
			// being held.
			waitExecutionLockReleased()
		}
		return
	}
	timeout := time.After(coretesting.LongWait)
	for {
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

func (s waitActionInvocation) step(c *tc.C, ctx *testContext) {
	timeout := time.After(coretesting.LongWait)
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

func assertActionsMatch(c *tc.C, actualIn []actionData, expectIn []actionData) {
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

func (s fixHook) step(_ *tc.C, ctx *testContext) {
	if ctx.runner.hooksWithErrors != nil {
		ctx.runner.hooksWithErrors.Remove(s.name)
	}
}

type updateStatusHookTick struct{}

func (s updateStatusHookTick) step(c *tc.C, ctx *testContext) {
	err := ctx.updateStatusHookTicker.Tick()
	c.Assert(err, tc.ErrorIsNil)
}

type changeConfig map[string]interface{}

func (s changeConfig) step(c *tc.C, ctx *testContext) {
	ctx.sendStrings(c, ctx.configCh, "config change event", ctx.app.configHash(s))
}

type addAction struct {
	name   string
	params map[string]interface{}
}

func (s addAction) step(c *tc.C, ctx *testContext) {
	tag := names.NewActionTag(strconv.Itoa(int(ctx.actionCounter.Add(1))))
	a := apiuniter.NewAction(tag.Id(), s.name, s.params, false, "")
	ctx.pendingActions = append(ctx.pendingActions, a)
	ctx.api.EXPECT().Action(gomock.Any(), tag).Return(a, nil).AnyTimes()
	c.Logf("beginning action %s", tag)
	ctx.api.EXPECT().ActionBegin(gomock.Any(), tag).DoAndReturn(func(_ context.Context, tag names.ActionTag) error {
		ctx.actionsCh <- []string{tag.Id()}
		return nil
	}).MaxTimes(2)
	ctx.api.EXPECT().ActionStatus(gomock.Any(), tag).Return("completed", nil).AnyTimes()
	ctx.sendStrings(c, ctx.actionsCh, "action begin event", tag.Id())
}

type upgradeCharm struct {
	revision int
	forced   bool
}

func (s upgradeCharm) step(c *tc.C, ctx *testContext) {
	ctx.app.mu.Lock()
	defer ctx.app.mu.Unlock()
	ctx.app.charmURL = curl(s.revision)
	ctx.app.charmForced = s.forced
	ctx.app.charmModifiedVersion++
	// Make sure we upload the charm before changing it in the DB.
	serveCharm{}.step(c, ctx)
	ctx.sendNotify(c, ctx.applicationCh, "application charm upgrade event")
}

type verifyCharm struct {
	revision          int
	attemptedRevision int
	checkFiles        filetesting.Entries
}

func (s verifyCharm) step(c *tc.C, ctx *testContext) {
	s.checkFiles.Check(c, filepath.Join(ctx.path, "charm"))
	checkRevision := s.revision
	if s.attemptedRevision > checkRevision {
		checkRevision = s.attemptedRevision
	}
	ctx.unit.mu.Lock()
	defer ctx.unit.mu.Unlock()
	urlStr := ctx.unit.charmURL
	c.Assert(urlStr, tc.Equals, curl(checkRevision))
}

type pushResource struct{}

func (s pushResource) step(c *tc.C, ctx *testContext) {
	ctx.app.mu.Lock()
	ctx.app.charmModifiedVersion++
	ctx.app.mu.Unlock()
	ctx.sendNotify(c, ctx.applicationCh, "resource change event")
}

type startUpgradeError struct{}

func (s startUpgradeError) step(c *tc.C, ctx *testContext) {
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
			customize: func(c *tc.C, ctx *testContext, path string) {
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

func (s verifyWaitingUpgradeError) step(c *tc.C, ctx *testContext) {
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
		custom{func(c *tc.C, ctx *testContext) {
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
		startUniter{rebootQuerier: &fakeRebootQuerier{rebootNotDetected}},
	}
	allSteps := append(verifyCharmSteps, verifyWaitingSteps...)
	allSteps = append(allSteps, verifyCharmSteps...)
	for _, s_ := range allSteps {
		step(c, ctx, s_)
	}
}

type fixUpgradeError struct{}

func (s fixUpgradeError) step(_ *tc.C, ctx *testContext) {
	ctx.deployer.err = nil
}

type addRelation struct {
}

func (s addRelation) step(c *tc.C, ctx *testContext) {
	if ctx.relation != nil {
		panic("don't add two relations!")
	}
	if ctx.relatedApplication == nil {
		ctx.relatedApplication = uniterapi.NewMockApplication(ctx.ctrl)
		ctx.relatedApplication.EXPECT().Tag().Return(names.NewApplicationTag("mysql")).AnyTimes()
	}

	relTag := names.NewRelationTag("wordpress:db mysql:db")
	ctx.relation = ctx.makeRelation(c, relTag, life.Alive, "mysql")

	ctx.relUnit = ctx.makeRelationUnit(c, ctx.relation, ctx.unit)
	ctx.relation.EXPECT().Unit(gomock.Any(), ctx.unit.Tag()).Return(ctx.relUnit, nil).AnyTimes()

	ctx.api.EXPECT().WatchRelationUnits(gomock.Any(), relTag, ctx.unit.Tag()).DoAndReturn(func(_ context.Context, _ names.RelationTag, _ names.UnitTag) (watcher.RelationUnitsWatcher, error) {
		ctx.stateMu.Lock()
		defer ctx.stateMu.Unlock()

		changes := watcher.RelationUnitsChange{Changed: make(map[string]watcher.UnitSettings)}
		relUnits := ctx.relationUnits[ctx.relation.Id()]
		for u, vers := range relUnits {
			changes.Changed[u] = watcher.UnitSettings{Version: vers}
		}
		ctx.sendRelationUnitChange(c, "initial relation unit change", changes)
		w := newMockRelationUnitsWatcher(ctx.relationUnitCh)
		return w, nil
	}).AnyTimes()

	ctx.sendStrings(c, ctx.relCh, "relation event", relTag.Id())

	step(c, ctx, waitHooks{"db-relation-created mysql db:0"})
}

type addRelationUnit struct{}

func (s addRelationUnit) step(c *tc.C, ctx *testContext) {
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

func (s changeRelationUnit) step(c *tc.C, ctx *testContext) {
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

func (s removeRelationUnit) step(c *tc.C, ctx *testContext) {
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

func (s relationState) step(c *tc.C, ctx *testContext) {
	if s.removed {
		c.Assert(ctx.relation.Life(), tc.Equals, life.Dying)
		return
	}
	c.Assert(ctx.relation.Life(), tc.Equals, s.life)
}

type addSubordinateRelation struct {
	ifce string
}

func (s addSubordinateRelation) step(c *tc.C, ctx *testContext) {
	relKey := subordinateRelationKey(s.ifce)
	relTag := names.NewRelationTag(relKey)
	ctx.subordRelation = ctx.makeRelation(c, relTag, life.Alive, "logging")

	ru := ctx.makeRelationUnit(c, ctx.subordRelation, ctx.unit)
	ctx.subordRelation.EXPECT().Unit(gomock.Any(), ctx.unit.Tag()).Return(ru, nil).AnyTimes()

	ctx.api.EXPECT().WatchRelationUnits(gomock.Any(), relTag, ctx.unit.Tag()).DoAndReturn(func(_ context.Context, _ names.RelationTag, _ names.UnitTag) (watcher.RelationUnitsWatcher, error) {
		changes := watcher.RelationUnitsChange{Changed: make(map[string]watcher.UnitSettings)}
		changes.AppChanged = map[string]int64{"logging": 0}
		ctx.sendRelationUnitChange(c, "initial subordinate relation unit change", changes)
		w := newMockRelationUnitsWatcher(ctx.relationUnitCh)
		return w, nil
	}).AnyTimes()

	ctx.sendStrings(c, ctx.relCh, "add subordinate relation event", relTag.Id())
}

type removeSubordinateRelation struct {
	ifce string
}

func (s removeSubordinateRelation) step(c *tc.C, ctx *testContext) {
	ctx.subordRelation.mu.Lock()
	ctx.subordRelation.life = life.Dying
	ctx.subordRelation.mu.Unlock()
	ctx.sendStrings(c, ctx.relCh, "remove subordinate relation event", subordinateRelationKey(s.ifce))
}

type waitSubordinateExists struct {
	name string
}

func (s waitSubordinateExists) step(c *tc.C, ctx *testContext) {
	// First wait for the principal unit to enter scope.
	// If subordinate is not alive, test does not allow the
	// principal to enter scope.
	timeout := time.After(coretesting.LongWait)
	for {
		select {
		case <-timeout:
			c.Fatalf("unit is alive did not enter scope")
		case <-time.After(coretesting.ShortWait):
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

func (waitSubordinateDying) step(c *tc.C, ctx *testContext) {
	timeout := time.After(coretesting.LongWait)
	for {
		select {
		case <-timeout:
			c.Fatalf("subordinate was not made Dying")
		case <-time.After(coretesting.ShortWait):
			ctx.unit.mu.Lock()
			subordLife := ctx.unit.subordinate.Life()
			ctx.unit.mu.Unlock()
			if subordLife != life.Dying {
				c.Logf("subordinate life is %q, not %q", subordLife, life.Dying)
				continue
			}
		}
		break
	}
}

type removeSubordinate struct{}

func (removeSubordinate) step(c *tc.C, ctx *testContext) {
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

func (s writeFile) step(c *tc.C, ctx *testContext) {
	path := filepath.Join(ctx.path, s.path)
	dir := filepath.Dir(path)
	err := os.MkdirAll(dir, 0755)
	c.Assert(err, tc.ErrorIsNil)
	err = os.WriteFile(path, nil, s.mode)
	c.Assert(err, tc.ErrorIsNil)
}

type removeCharmDir struct{}

func (s removeCharmDir) step(c *tc.C, ctx *testContext) {
	path := filepath.Join(ctx.path, "charm")
	err := os.RemoveAll(path)
	c.Assert(err, tc.ErrorIsNil)
}

type custom struct {
	f func(*tc.C, *testContext)
}

func (s custom) step(c *tc.C, ctx *testContext) {
	s.f(c, ctx)
}

var relationDying = custom{func(c *tc.C, ctx *testContext) {
	ctx.relation.mu.Lock()
	ctx.relation.life = life.Dying
	ctx.relation.mu.Unlock()
	ctx.sendStrings(c, ctx.relCh, "relation dying event", ctx.relation.Tag().Id())
}}

var unitDying = custom{func(c *tc.C, ctx *testContext) {
	ctx.unit.mu.Lock()
	ctx.unit.life = life.Dying
	ctx.unit.mu.Unlock()
	ctx.api.EXPECT().DestroyUnitStorageAttachments(gomock.Any(), ctx.unit.Tag()).Return(nil)

	ctx.stateMu.Lock()
	for id := range ctx.storage {
		// Could be twice due to short circuit.
		ctx.api.EXPECT().RemoveStorageAttachment(gomock.Any(), names.NewStorageTag(id), ctx.unit.Tag()).DoAndReturn(func(_ context.Context, tag names.StorageTag, _ names.UnitTag) error {
			ctx.stateMu.Lock()
			delete(ctx.storage, id)
			ctx.stateMu.Unlock()
			return nil
		}).MaxTimes(3) // detaching hook, then short circuit remove called twice
	}
	ctx.stateMu.Unlock()
	ctx.sendUnitNotify(c, "send unit dying event")
}}

var unitDead = custom{func(c *tc.C, ctx *testContext) {
	ctx.unit.mu.Lock()
	ctx.unit.life = life.Dead
	ctx.unit.mu.Unlock()
	ctx.sendUnitNotify(c, "send unit dead event")
}}

var subordinateDying = custom{func(c *tc.C, ctx *testContext) {
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
	stepFunc func(*tc.C, *testContext)
}

func (h *hookStep) step(c *tc.C, ctx *testContext) {
	h.stepFunc(c, ctx)
}

func (h *hookLock) acquire() *hookStep {
	return &hookStep{stepFunc: func(c *tc.C, ctx *testContext) {
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
	return &hookStep{stepFunc: func(c *tc.C, ctx *testContext) {
		c.Assert(h.releaser, tc.NotNil)
		h.releaser()
		h.releaser = nil
	}}
}

type runCommands []string

func (cmds runCommands) step(c *tc.C, ctx *testContext) {
	commands := strings.Join(cmds, "\n")
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

func (forceMinion) step(c *tc.C, ctx *testContext) {
	ctx.leaderTracker.setLeader(c, false)
}

type forceLeader struct{}

func (forceLeader) step(c *tc.C, ctx *testContext) {
	ctx.leaderTracker.setLeader(c, true)
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

func (mock *mockLeaderTracker) Kill() {
	return
}

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

func (mock *mockLeaderTracker) setLeader(c *tc.C, isLeader bool) {
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
func (*mockCharmDirGuard) Unlock() error { return nil }

// Lockdown implements fortress.Guard.
func (*mockCharmDirGuard) Lockdown(_ fortress.Abort) error { return nil }

type provisionStorage struct{}

func (s provisionStorage) step(c *tc.C, ctx *testContext) {
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

func (s destroyStorageAttachment) step(c *tc.C, ctx *testContext) {
	ctx.stateMu.Lock()
	ctx.storage = make(map[string]*storageAttachment)
	ctx.stateMu.Unlock()
}

type verifyStorageDetached struct{}

func (s verifyStorageDetached) step(c *tc.C, ctx *testContext) {
	ctx.stateMu.Lock()
	defer ctx.stateMu.Unlock()
	c.Assert(ctx.storage, tc.HasLen, 0)
}

func ptr[T any](v T) *T {
	return &v
}

type createSecret struct {
	applicationName string
}

func (s createSecret) step(c *tc.C, ctx *testContext) {
	if s.applicationName == "" {
		s.applicationName = "u"
	}

	uri := secrets.NewURI()
	ctx.secretBackends.EXPECT().GetContent(gomock.Any(), uri, "foorbar", false, false).Return(
		secrets.NewSecretValue(map[string]string{"foo": "bar"}), nil).AnyTimes()
	ctx.createdSecretURI = uri
}

type changeSecret struct{}

func (s changeSecret) step(c *tc.C, ctx *testContext) {
	ctx.secretsClient.EXPECT().GetConsumerSecretsRevisionInfo(
		gomock.Any(),
		ctx.unit.Name(), []string{ctx.createdSecretURI.String()},
	).Return(map[string]secrets.SecretRevisionInfo{
		ctx.createdSecretURI.String(): {LatestRevision: 666},
	}, nil)
	ctx.sendStrings(c, ctx.consumedSecretsCh, "secret change", ctx.createdSecretURI.String())
	done := make(chan bool)
	go func() {
		for {
			ctx.stateMu.Lock()
			if strings.Contains(fmt.Sprintf("secret-revisions: %s: 666\n", ctx.createdSecretURI), ctx.secretsState) {
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

func (s getSecret) step(c *tc.C, ctx *testContext) {
	val, err := ctx.secretBackends.GetContent(context.Background(), ctx.createdSecretURI, "foorbar", false, false)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(val.EncodedValues(), tc.DeepEquals, map[string]string{"foo": "bar"})
}

type rotateSecret struct {
	rev int
}

func (s rotateSecret) step(c *tc.C, ctx *testContext) {
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

func (s expireSecret) step(c *tc.C, ctx *testContext) {
	ctx.sendStrings(c, ctx.secretsExpireCh, "expire secret change", ctx.createdSecretURI.String()+"/1")
}

type expectError struct {
	err string
}

func (s expectError) step(_ *tc.C, ctx *testContext) {
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
	case <-time.After(coretesting.LongWait):
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

func (s activateTestContainer) step(c *tc.C, ctx *testContext) {
	ctx.pebbleClients[s.containerName].TriggerStart()
}

type injectTestContainer struct {
	containerName string
}

func (s injectTestContainer) step(c *tc.C, ctx *testContext) {
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

func (t triggerShutdown) step(c *tc.C, ctx *testContext) {
	err := ctx.uniter.Terminate()
	c.Assert(err, tc.ErrorIsNil)
}
