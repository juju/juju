// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package context_test

import (
	"encoding/hex"
	"os"
	"strings"
	"time"

	"github.com/juju/charm/v12/hooks"
	"github.com/juju/clock/testclock"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names/v5"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/v3"
	"github.com/juju/utils/v3/fs"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/agent/uniter"
	"github.com/juju/juju/caas/kubernetes/provider"
	k8stesting "github.com/juju/juju/caas/kubernetes/provider/testing"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/leadership"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/environs"
	environscontext "github.com/juju/juju/environs/context"
	"github.com/juju/juju/feature"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
	statetesting "github.com/juju/juju/state/testing"
	"github.com/juju/juju/storage"
	"github.com/juju/juju/testcharms"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/testing/factory"
	"github.com/juju/juju/worker/uniter/hook"
	"github.com/juju/juju/worker/uniter/runner/context"
	runnertesting "github.com/juju/juju/worker/uniter/runner/testing"
)

type ContextFactorySuite struct {
	HookContextSuite
	paths      runnertesting.RealPaths
	factory    context.ContextFactory
	membership map[int][]string
}

var _ = gc.Suite(&ContextFactorySuite{})

func (s *ContextFactorySuite) SetUpTest(c *gc.C) {
	s.ControllerConfigAttrs = map[string]interface{}{
		controller.Features: []string{feature.RawK8sSpec},
	}

	s.HookContextSuite.SetUpTest(c)
	s.paths = runnertesting.NewRealPaths(c)
	s.membership = map[int][]string{
		0: {"r/0"},
		1: {"r/1"},
	}

	contextFactory, err := context.NewContextFactory(context.FactoryConfig{
		State:            s.uniter,
		Unit:             s.apiUnit,
		Tracker:          &runnertesting.FakeTracker{},
		GetRelationInfos: s.getRelationInfos,
		SecretsClient:    s.secrets,
		Payloads:         s.payloads,
		Paths:            s.paths,
		Clock:            testclock.NewClock(time.Time{}),
		Logger:           loggo.GetLogger("test"),
	})
	c.Assert(err, jc.ErrorIsNil)
	s.factory = contextFactory
	s.PatchValue(&provider.NewK8sClients, k8stesting.NoopFakeK8sClients)
}

func (s *ContextFactorySuite) setUpCacheMethods(c *gc.C) {
	// The factory's caches are created lazily, so it doesn't have any at all to
	// begin with. Creating and discarding a context lets us call updateCache
	// without panicking. (IMO this is less invasive that making updateCache
	// responsible for creating missing caches etc.)
	_, err := s.factory.HookContext(hook.Info{Kind: hooks.Install})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *ContextFactorySuite) Model(c *gc.C) *state.Model {
	m, err := s.State.Model()
	c.Assert(err, jc.ErrorIsNil)
	return m
}

func (s *ContextFactorySuite) updateCache(relId int, unitName string, settings params.Settings) {
	context.UpdateCachedSettings(s.factory, relId, unitName, settings)
}

func (s *ContextFactorySuite) updateAppCache(relId int, unitName string, settings params.Settings) {
	context.UpdateCachedAppSettings(s.factory, relId, unitName, settings)
}

func (s *ContextFactorySuite) getCache(relId int, unitName string) (params.Settings, bool) {
	return context.CachedSettings(s.factory, relId, unitName)
}

func (s *ContextFactorySuite) getAppCache(relId int, appName string) (params.Settings, bool) {
	return context.CachedAppSettings(s.factory, relId, appName)
}

func (s *ContextFactorySuite) SetCharm(c *gc.C, name string) {
	err := os.RemoveAll(s.paths.GetCharmDir())
	c.Assert(err, jc.ErrorIsNil)
	err = fs.Copy(testcharms.Repo.CharmDirPath(name), s.paths.GetCharmDir())
	c.Assert(err, jc.ErrorIsNil)
}

func (s *ContextFactorySuite) getRelationInfos() map[int]*context.RelationInfo {
	info := map[int]*context.RelationInfo{}
	for relId, relUnit := range s.apiRelunits {
		info[relId] = &context.RelationInfo{
			RelationUnit: &relUnitShim{relUnit},
			MemberNames:  s.membership[relId],
		}
	}
	return info
}

func (s *ContextFactorySuite) testLeadershipContextWiring(c *gc.C, createContext func() *context.HookContext) {
	var stub testing.Stub
	stub.SetErrors(errors.New("bam"))
	restore := context.PatchNewLeadershipContext(
		func(accessor context.LeadershipSettingsAccessor, tracker leadership.Tracker, unitName string) context.LeadershipContext {
			stub.AddCall("NewLeadershipContext", accessor, tracker, unitName)
			return &StubLeadershipContext{Stub: &stub}
		},
	)
	defer restore()

	ctx := createContext()
	isLeader, err := ctx.IsLeader()
	c.Check(err, gc.ErrorMatches, "bam")
	c.Check(isLeader, jc.IsFalse)

	stub.CheckCalls(c, []testing.StubCall{{
		FuncName: "NewLeadershipContext",
		Args:     []interface{}{s.uniter.LeadershipSettings, &runnertesting.FakeTracker{}, "u/0"},
	}, {
		FuncName: "IsLeader",
	}})

}

func (s *ContextFactorySuite) TestNewHookContextRetrievesSLALevel(c *gc.C) {
	err := s.State.SetSLA("essential", "bob", []byte("creds"))
	c.Assert(err, jc.ErrorIsNil)

	ctx, err := s.factory.HookContext(hook.Info{Kind: hooks.ConfigChanged})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ctx.SLALevel(), gc.Equals, "essential")
}

func (s *ContextFactorySuite) TestNewHookContextLeadershipContext(c *gc.C) {
	s.testLeadershipContextWiring(c, func() *context.HookContext {
		ctx, err := s.factory.HookContext(hook.Info{Kind: hooks.ConfigChanged})
		c.Assert(err, jc.ErrorIsNil)
		return ctx
	})
}

func (s *ContextFactorySuite) TestNewCommandContextLeadershipContext(c *gc.C) {
	s.testLeadershipContextWiring(c, func() *context.HookContext {
		ctx, err := s.factory.CommandContext(context.CommandInfo{RelationId: -1})
		c.Assert(err, jc.ErrorIsNil)
		return ctx
	})
}

func (s *ContextFactorySuite) TestNewActionContextLeadershipContext(c *gc.C) {
	s.testLeadershipContextWiring(c, func() *context.HookContext {
		s.SetCharm(c, "dummy")
		operationID, err := s.Model(c).EnqueueOperation("a test", 1)
		c.Assert(err, jc.ErrorIsNil)
		action, err := s.Model(c).EnqueueAction(operationID, s.unit.Tag(), "snapshot", nil, true, "group", nil)
		c.Assert(err, jc.ErrorIsNil)

		actionData := &context.ActionData{
			Name:       action.Name(),
			Tag:        names.NewActionTag(action.Id()),
			Params:     action.Parameters(),
			ResultsMap: map[string]interface{}{},
		}

		ctx, err := s.factory.ActionContext(actionData)
		c.Assert(err, jc.ErrorIsNil)
		return ctx
	})
}

func (s *ContextFactorySuite) TestHookContextID(c *gc.C) {
	hi := hook.Info{
		Kind: hooks.Install,
	}
	ctx, err := s.factory.HookContext(hi)
	c.Assert(err, jc.ErrorIsNil)

	v := strings.Split(ctx.Id(), "-")
	c.Assert(v, gc.HasLen, 3)

	randomComponent, err := hex.DecodeString(v[2])
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(randomComponent, gc.HasLen, 16)
}

func (s *ContextFactorySuite) TestRelationHookContext(c *gc.C) {
	hi := hook.Info{
		Kind:       hooks.RelationBroken,
		RelationId: 1,
	}
	ctx, err := s.factory.HookContext(hi)
	c.Assert(err, jc.ErrorIsNil)
	s.AssertCoreContext(c, ctx)
	s.AssertNotActionContext(c, ctx)
	s.AssertRelationContext(c, ctx, 1, "", "")
	s.AssertNotStorageContext(c, ctx)
	s.AssertNotWorkloadContext(c, ctx)
	s.AssertNotSecretContext(c, ctx)
}

func (s *ContextFactorySuite) TestRelationBrokenHookContext(c *gc.C) {
	delete(s.membership, 1)
	rel, err := s.State.Relation(1)
	c.Assert(err, jc.ErrorIsNil)
	err = rel.SetSuspended(true, "")
	c.Assert(err, jc.ErrorIsNil)
	err = s.apiRelunits[1].Relation().Refresh()
	c.Assert(err, jc.ErrorIsNil)
	hi := hook.Info{
		Kind:       hooks.RelationBroken,
		RelationId: 1,
	}
	ctx, err := s.factory.HookContext(hi)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(context.RelationBroken(ctx, 0), jc.IsFalse)
	c.Assert(context.RelationBroken(ctx, 1), jc.IsTrue)
}

func (s *ContextFactorySuite) TestRelationIsPeerHookContext(c *gc.C) {
	relCh := s.AddTestingCharm(c, "riak")
	app := s.AddTestingApplication(c, "riak", relCh)
	u, err := app.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	password, err := utils.RandomPassword()
	c.Assert(err, jc.ErrorIsNil)
	err = u.SetPassword(password)
	c.Assert(err, jc.ErrorIsNil)
	st := s.OpenAPIAs(c, u.Tag(), password)
	uniterAPI, err := uniter.NewFromConnection(st)
	c.Assert(err, jc.ErrorIsNil)

	rels, err := app.Relations()
	c.Assert(err, jc.ErrorIsNil)
	var rel *state.Relation
	for _, r := range rels {
		if len(r.Endpoints()) == 1 {
			rel = r
			break
		}
	}
	c.Assert(rel, gc.NotNil)
	ru, err := rel.Unit(u)
	c.Assert(err, jc.ErrorIsNil)
	err = ru.EnterScope(map[string]interface{}{"relation-name": "riak"})
	c.Assert(err, jc.ErrorIsNil)
	apiRel, err := uniterAPI.Relation(rel.Tag().(names.RelationTag))
	c.Assert(err, jc.ErrorIsNil)
	apiRelUnit, err := apiRel.Unit(u.UnitTag())
	c.Assert(err, jc.ErrorIsNil)
	s.apiRelunits[rel.Id()] = apiRelUnit

	hi := hook.Info{
		Kind:       hooks.RelationBroken,
		RelationId: rel.Id(),
	}
	ctx, err := s.factory.HookContext(hi)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(context.RelationBroken(ctx, rel.Id()), jc.IsFalse)
}

// TestWorkloadHookContext verifies that each of the types of workload hook
// generate the correct event context.
func (s *ContextFactorySuite) TestWorkloadHookContext(c *gc.C) {
	infos := []hook.Info{
		{
			Kind:         hooks.PebbleReady,
			WorkloadName: "test",
		},
		{
			Kind:         hooks.PebbleCustomNotice,
			WorkloadName: "test",
			NoticeID:     "123",
			NoticeType:   "custom",
			NoticeKey:    "example.com/bar",
		},
		{
			Kind:         hooks.PebbleCheckFailed,
			WorkloadName: "test",
			CheckName:    "http-check",
		},
		{
			Kind:         hooks.PebbleCheckRecovered,
			WorkloadName: "test",
			CheckName:    "http-check",
		},
	}
	for _, hi := range infos {
		ctx, err := s.factory.HookContext(hi)
		c.Assert(err, jc.ErrorIsNil)
		s.AssertCoreContext(c, ctx)
		s.AssertWorkloadContext(c, ctx, "test")
		s.AssertNotActionContext(c, ctx)
		s.AssertNotRelationContext(c, ctx)
		s.AssertNotStorageContext(c, ctx)
		s.AssertNotSecretContext(c, ctx)
		switch hi.Kind {
		case hooks.PebbleCustomNotice:
			actualNoticeKey, _ := ctx.WorkloadNoticeKey()
			c.Assert(actualNoticeKey, gc.Equals, "example.com/bar")
			actualNoticeType, _ := ctx.WorkloadNoticeType()
			c.Assert(actualNoticeType, gc.Equals, "custom")
		case hooks.PebbleCheckFailed, hooks.PebbleCheckRecovered:
			actualCheckName, _ := ctx.WorkloadCheckName()
			c.Assert(actualCheckName, gc.Equals, "http-check")
		}
	}
}

func (s *ContextFactorySuite) TestNewHookContextWithStorage(c *gc.C) {
	// We need to set up a unit that has storage metadata defined.
	ch := s.AddTestingCharm(c, "storage-block")
	sCons := map[string]state.StorageConstraints{
		"data": {Pool: "", Size: 1024, Count: 1},
	}
	application := s.AddTestingApplicationWithStorage(c, "storage-block", ch, sCons)
	s.machine = nil // allocate a new machine
	unit := s.AddUnit(c, application)

	sb, err := state.NewStorageBackend(s.State)
	c.Assert(err, jc.ErrorIsNil)
	storageAttachments, err := sb.UnitStorageAttachments(unit.UnitTag())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(storageAttachments, gc.HasLen, 1)
	storageTag := storageAttachments[0].StorageInstance()

	volume, err := sb.StorageInstanceVolume(storageTag)
	c.Assert(err, jc.ErrorIsNil)
	volumeTag := volume.VolumeTag()
	machineTag := s.machine.MachineTag()

	err = sb.SetVolumeInfo(
		volumeTag, state.VolumeInfo{
			VolumeId: "vol-123",
			Size:     456,
		},
	)
	c.Assert(err, jc.ErrorIsNil)
	err = sb.SetVolumeAttachmentInfo(
		machineTag, volumeTag, state.VolumeAttachmentInfo{
			DeviceName: "sdb",
		},
	)
	c.Assert(err, jc.ErrorIsNil)

	err = sb.CreateVolumeAttachmentPlan(machineTag, volumeTag, state.VolumeAttachmentPlanInfo{
		DeviceType:       storage.DeviceTypeLocal,
		DeviceAttributes: nil,
	})
	c.Assert(err, jc.ErrorIsNil)

	err = sb.SetVolumeAttachmentPlanBlockInfo(machineTag, volumeTag, state.BlockDeviceInfo{
		DeviceName: "sdb",
	})
	c.Assert(err, jc.ErrorIsNil)

	err = s.machine.SetMachineBlockDevices(state.BlockDeviceInfo{
		DeviceName: "sdb",
	})
	c.Assert(err, jc.ErrorIsNil)

	password, err := utils.RandomPassword()
	c.Assert(err, jc.ErrorIsNil)
	err = unit.SetPassword(password)
	c.Assert(err, jc.ErrorIsNil)
	st := s.OpenAPIAs(c, unit.Tag(), password)
	uniter, err := uniter.NewFromConnection(st)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(uniter, gc.NotNil)
	apiUnit, err := uniter.Unit(unit.Tag().(names.UnitTag))
	c.Assert(err, jc.ErrorIsNil)

	contextFactory, err := context.NewContextFactory(context.FactoryConfig{
		State:            uniter,
		Unit:             apiUnit,
		Tracker:          &runnertesting.FakeTracker{},
		GetRelationInfos: s.getRelationInfos,
		SecretsClient:    s.secrets,
		Payloads:         s.payloads,
		Paths:            s.paths,
		Clock:            testclock.NewClock(time.Time{}),
		Logger:           loggo.GetLogger("test"),
	})
	c.Assert(err, jc.ErrorIsNil)
	ctx, err := contextFactory.HookContext(hook.Info{
		Kind:      hooks.StorageAttached,
		StorageId: "data/0",
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ctx.UnitName(), gc.Equals, "storage-block/0")
	c.Assert(ctx.ModelType(), gc.Equals, model.IAAS)
	s.AssertStorageContext(c, ctx, "data/0", storage.StorageAttachmentInfo{
		Kind:     storage.StorageKindBlock,
		Location: "/dev/sdb",
	})
	s.AssertNotActionContext(c, ctx)
	s.AssertNotRelationContext(c, ctx)
	s.AssertNotSecretContext(c, ctx)
}

func (s *ContextFactorySuite) TestSecretHookContext(c *gc.C) {
	hi := hook.Info{
		// Kind can be any secret hook kind.
		// Whatever attributes are set below will
		// be added to the context.
		Kind:           hooks.SecretExpired,
		SecretURI:      "secret:9m4e2mr0ui3e8a215n4g",
		SecretLabel:    "label",
		SecretRevision: 666,
	}
	ctx, err := s.factory.HookContext(hi)
	c.Assert(err, jc.ErrorIsNil)
	s.AssertCoreContext(c, ctx)
	s.AssertSecretContext(c, ctx, hi.SecretURI, hi.SecretLabel, hi.SecretRevision)
	s.AssertNotWorkloadContext(c, ctx)
	s.AssertNotActionContext(c, ctx)
	s.AssertNotRelationContext(c, ctx)
	s.AssertNotStorageContext(c, ctx)
}

var podSpec = `
containers:
  - name: gitlab
    image: gitlab/latest
    ports:
    - containerPort: 80
      protocol: TCP
    - containerPort: 443
    config:
      attr: foo=bar; fred=blogs
      foo: bar
`[1:]

func (s *ContextFactorySuite) setupPodSpec(c *gc.C) (*state.State, context.ContextFactory, string) {
	st := s.Factory.MakeCAASModel(c, nil)
	f := factory.NewFactory(st, s.StatePool)
	ch := f.MakeCharm(c, &factory.CharmParams{Name: "gitlab", Series: "kubernetes"})
	app := f.MakeApplication(c, &factory.ApplicationParams{Name: "gitlab", Charm: ch})
	unit, err := app.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)

	// We are using the lease.Manager from the apiserver and not st.LeadershipClaimer
	// so unfortunately, we need to hack the acquisition of a lease because of the
	// way this test is set up.
	claimer, err := s.LeaseManager.Claimer("application-leadership", st.ModelUUID())
	c.Assert(err, jc.ErrorIsNil)
	err = claimer.Claim(app.Tag().Id(), unit.Tag().Id(), time.Minute)
	c.Assert(err, jc.ErrorIsNil)

	password, err := utils.RandomPassword()
	c.Assert(err, jc.ErrorIsNil)
	err = unit.SetPassword(password)
	c.Assert(err, jc.ErrorIsNil)
	apiInfo, err := environs.APIInfo(
		environscontext.NewEmptyCloudCallContext(),
		s.ControllerConfig.ControllerUUID(), st.ModelUUID(), coretesting.CACert, s.ControllerConfig.APIPort(), s.Environ)
	c.Assert(err, jc.ErrorIsNil)
	apiInfo.Tag = unit.Tag()
	apiInfo.Password = password
	apiState, err := api.Open(apiInfo, api.DialOpts{})
	c.Assert(err, jc.ErrorIsNil)
	uniter, err := uniter.NewFromConnection(apiState)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(uniter, gc.NotNil)
	apiUnit, err := uniter.Unit(unit.Tag().(names.UnitTag))
	c.Assert(err, jc.ErrorIsNil)

	contextFactory, err := context.NewContextFactory(context.FactoryConfig{
		State: uniter,
		Unit:  apiUnit,
		Tracker: &runnertesting.FakeTracker{
			AllowClaimLeader: true,
		},
		GetRelationInfos: s.getRelationInfos,
		SecretsClient:    s.secrets,
		Payloads:         s.payloads,
		Paths:            s.paths,
		Clock:            testclock.NewClock(time.Time{}),
		Logger:           loggo.GetLogger("test"),
	})
	c.Assert(err, jc.ErrorIsNil)
	return st, contextFactory, unit.ApplicationName()
}

func (s *ContextFactorySuite) TestHookContextCAASDeferredSetPodSpec(c *gc.C) {
	st, cf, appName := s.setupPodSpec(c)
	defer st.Close()
	appTag := names.NewApplicationTag(appName)

	ctx, err := cf.HookContext(hook.Info{
		Kind: hooks.ConfigChanged,
	})
	c.Assert(err, jc.ErrorIsNil)

	sm, err := st.Model()
	c.Assert(err, jc.ErrorIsNil)
	cm, err := sm.CAASModel()
	c.Assert(err, jc.ErrorIsNil)

	err = ctx.SetPodSpec(podSpec)
	c.Assert(err, jc.ErrorIsNil)

	_, err = cm.PodSpec(appTag)
	c.Assert(err, gc.ErrorMatches, "k8s spec for application gitlab not found")
	_, err = cm.RawK8sSpec(appTag)
	c.Assert(err, gc.ErrorMatches, "k8s spec for application gitlab not found")

	err = ctx.Flush("", nil)
	c.Assert(err, jc.ErrorIsNil)

	ps, err := cm.PodSpec(appTag)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ps, gc.Equals, podSpec)

	rps, err := cm.RawK8sSpec(appTag)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(rps, gc.Equals, "")
}

var rawK8sSpec = `
apiVersion: apps/v1
kind: Deployment
metadata:
  name: nginx-deployment
  labels:
    app: nginx
spec:
  replicas: 3
  selector:
    matchLabels:
      app: nginx
  template:
    metadata:
      labels:
        app: nginx
    spec:
      containers:
      - name: nginx
        image: nginx:1.14.2
        ports:
        - containerPort: 80
`[1:]

func (s *ContextFactorySuite) TestHookContextCAASDeferredSetRawK8sSpec(c *gc.C) {
	st, cf, appName := s.setupPodSpec(c)
	defer st.Close()
	appTag := names.NewApplicationTag(appName)

	ctx, err := cf.HookContext(hook.Info{
		Kind: hooks.ConfigChanged,
	})
	c.Assert(err, jc.ErrorIsNil)

	sm, err := st.Model()
	c.Assert(err, jc.ErrorIsNil)
	cm, err := sm.CAASModel()
	c.Assert(err, jc.ErrorIsNil)

	err = ctx.SetRawK8sSpec(rawK8sSpec)
	c.Assert(err, jc.ErrorIsNil)

	_, err = cm.PodSpec(appTag)
	c.Assert(err, gc.ErrorMatches, "k8s spec for application gitlab not found")
	_, err = cm.RawK8sSpec(appTag)
	c.Assert(err, gc.ErrorMatches, "k8s spec for application gitlab not found")

	err = ctx.Flush("", nil)
	c.Assert(err, jc.ErrorIsNil)

	rps, err := cm.RawK8sSpec(appTag)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(rps, gc.Equals, rawK8sSpec)

	ps, err := cm.PodSpec(appTag)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ps, gc.Equals, "")
}

func (s *ContextFactorySuite) TestHookContextCAASDeferredSetPodSpecSetRawK8sSpecNotAllowed(c *gc.C) {
	st, cf, appName := s.setupPodSpec(c)
	defer st.Close()
	appTag := names.NewApplicationTag(appName)

	ctx, err := cf.HookContext(hook.Info{
		Kind: hooks.ConfigChanged,
	})
	c.Assert(err, jc.ErrorIsNil)

	sm, err := st.Model()
	c.Assert(err, jc.ErrorIsNil)
	cm, err := sm.CAASModel()
	c.Assert(err, jc.ErrorIsNil)

	err = ctx.SetPodSpec(podSpec)
	c.Assert(err, jc.ErrorIsNil)
	_, err = cm.PodSpec(appTag)
	c.Assert(err, gc.ErrorMatches, "k8s spec for application gitlab not found")

	err = ctx.SetRawK8sSpec(rawK8sSpec)
	c.Assert(err, jc.ErrorIsNil)
	_, err = cm.RawK8sSpec(appTag)
	c.Assert(err, gc.ErrorMatches, "k8s spec for application gitlab not found")

	err = ctx.Flush("", nil)
	c.Assert(err, gc.ErrorMatches, `either k8s-spec-set or k8s-raw-set can be run for each application, but not both`)
}

func (s *ContextFactorySuite) TestHookContextCAASNilPodSpecNilRawPodSpecButUpgradeCharmHookRan(c *gc.C) {
	st, cf, appName := s.setupPodSpec(c)
	defer st.Close()

	ctx, err := cf.HookContext(hook.Info{
		Kind: hooks.UpgradeCharm,
	})
	c.Assert(err, jc.ErrorIsNil)

	sm, err := st.Model()
	c.Assert(err, jc.ErrorIsNil)
	cm, err := sm.CAASModel()
	c.Assert(err, jc.ErrorIsNil)

	appTag := names.NewApplicationTag(appName)
	w, err := cm.WatchPodSpec(appTag)
	c.Assert(err, jc.ErrorIsNil)
	wc := statetesting.NewNotifyWatcherC(c, w)
	wc.AssertOneChange() // initial event.

	// No change for non upgrade-hook.
	err = ctx.Flush("", nil)
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertNoChange()

	err = ctx.Flush("upgrade-charm", nil)
	c.Assert(err, jc.ErrorIsNil)
	// both k8s spec and raw k8s spec are nil, but "upgrade-charm" hook will trigger a change to update "upgrade-counter".
	wc.AssertOneChange()

	ps, err := cm.PodSpec(appTag)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ps, gc.Equals, "")

	rps, err := cm.RawK8sSpec(appTag)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(rps, gc.Equals, "")

	statetesting.AssertStop(c, w)
	wc.AssertClosed()
}

func (s *ContextFactorySuite) TestNewHookContextCAASModel(c *gc.C) {
	st := s.Factory.MakeCAASModel(c, nil)
	defer st.Close()
	f := factory.NewFactory(st, s.StatePool)
	ch := f.MakeCharm(c, &factory.CharmParams{Name: "gitlab", Series: "kubernetes"})
	app := f.MakeApplication(c, &factory.ApplicationParams{Name: "gitlab", Charm: ch})
	unit, err := app.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)

	password, err := utils.RandomPassword()
	c.Assert(err, jc.ErrorIsNil)
	err = unit.SetPassword(password)
	c.Assert(err, jc.ErrorIsNil)
	apiInfo, err := environs.APIInfo(
		environscontext.NewEmptyCloudCallContext(),
		s.ControllerConfig.ControllerUUID(), st.ModelUUID(), coretesting.CACert, s.ControllerConfig.APIPort(), s.Environ)
	c.Assert(err, jc.ErrorIsNil)
	apiInfo.Tag = unit.Tag()
	apiInfo.Password = password
	apiState, err := api.Open(apiInfo, api.DialOpts{})
	c.Assert(err, jc.ErrorIsNil)
	uniter, err := uniter.NewFromConnection(apiState)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(uniter, gc.NotNil)
	apiUnit, err := uniter.Unit(unit.Tag().(names.UnitTag))
	c.Assert(err, jc.ErrorIsNil)

	contextFactory, err := context.NewContextFactory(context.FactoryConfig{
		State: uniter,
		Unit:  apiUnit,
		Tracker: &runnertesting.FakeTracker{
			AllowClaimLeader: true,
		},
		GetRelationInfos: s.getRelationInfos,
		SecretsClient:    s.secrets,
		Payloads:         s.payloads,
		Paths:            s.paths,
		Clock:            testclock.NewClock(time.Time{}),
		Logger:           loggo.GetLogger("test"),
	})
	c.Assert(err, jc.ErrorIsNil)
	ctx, err := contextFactory.HookContext(hook.Info{
		Kind: hooks.ConfigChanged,
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ctx.UnitName(), gc.Equals, unit.Name())
	c.Assert(ctx.ModelType(), gc.Equals, model.CAAS)
	s.AssertNotActionContext(c, ctx)
	s.AssertNotRelationContext(c, ctx)
	s.AssertNotStorageContext(c, ctx)
	s.AssertNotWorkloadContext(c, ctx)
}

func (s *ContextFactorySuite) TestActionContext(c *gc.C) {
	s.SetCharm(c, "dummy")
	operationID, err := s.Model(c).EnqueueOperation("a test", 1)
	c.Assert(err, jc.ErrorIsNil)
	action, err := s.Model(c).EnqueueAction(operationID, s.unit.Tag(), "snapshot", nil, true, "group", nil)
	c.Assert(err, jc.ErrorIsNil)

	actionData := &context.ActionData{
		Name:       action.Name(),
		Tag:        names.NewActionTag(action.Id()),
		Params:     action.Parameters(),
		ResultsMap: map[string]interface{}{},
	}

	ctx, err := s.factory.ActionContext(actionData)
	c.Assert(err, jc.ErrorIsNil)

	s.AssertCoreContext(c, ctx)
	s.AssertActionContext(c, ctx)
	s.AssertNotRelationContext(c, ctx)
	s.AssertNotStorageContext(c, ctx)
	s.AssertNotWorkloadContext(c, ctx)
}

func (s *ContextFactorySuite) TestCommandContext(c *gc.C) {
	ctx, err := s.factory.CommandContext(context.CommandInfo{RelationId: -1})
	c.Assert(err, jc.ErrorIsNil)

	s.AssertCoreContext(c, ctx)
	s.AssertNotActionContext(c, ctx)
	s.AssertNotRelationContext(c, ctx)
	s.AssertNotStorageContext(c, ctx)
	s.AssertNotWorkloadContext(c, ctx)
}

func (s *ContextFactorySuite) TestCommandContextNoRelation(c *gc.C) {
	ctx, err := s.factory.CommandContext(context.CommandInfo{RelationId: -1})
	c.Assert(err, jc.ErrorIsNil)
	s.AssertCoreContext(c, ctx)
	s.AssertNotActionContext(c, ctx)
	s.AssertNotRelationContext(c, ctx)
	s.AssertNotStorageContext(c, ctx)
	s.AssertNotWorkloadContext(c, ctx)
}

func (s *ContextFactorySuite) TestNewCommandContextForceNoRemoteUnit(c *gc.C) {
	ctx, err := s.factory.CommandContext(context.CommandInfo{
		RelationId: 0, ForceRemoteUnit: true,
	})
	c.Assert(err, jc.ErrorIsNil)
	s.AssertCoreContext(c, ctx)
	s.AssertNotActionContext(c, ctx)
	s.AssertRelationContext(c, ctx, 0, "", "")
	s.AssertNotStorageContext(c, ctx)
	s.AssertNotWorkloadContext(c, ctx)
}

func (s *ContextFactorySuite) TestNewCommandContextForceRemoteUnitMissing(c *gc.C) {
	ctx, err := s.factory.CommandContext(context.CommandInfo{
		// TODO(jam): 2019-10-23 Add RemoteApplicationName
		RelationId: 0, RemoteUnitName: "blah/123", ForceRemoteUnit: true,
	})
	c.Assert(err, gc.IsNil)
	s.AssertCoreContext(c, ctx)
	s.AssertNotActionContext(c, ctx)
	s.AssertRelationContext(c, ctx, 0, "blah/123", "")
	s.AssertNotStorageContext(c, ctx)
	s.AssertNotWorkloadContext(c, ctx)
}

func (s *ContextFactorySuite) TestNewCommandContextInferRemoteUnit(c *gc.C) {
	// TODO(jam): 2019-10-23 Add RemoteApplicationName
	s.membership[0] = []string{"foo/2"}
	ctx, err := s.factory.CommandContext(context.CommandInfo{RelationId: 0})
	c.Assert(err, jc.ErrorIsNil)
	s.AssertCoreContext(c, ctx)
	s.AssertNotActionContext(c, ctx)
	s.AssertRelationContext(c, ctx, 0, "foo/2", "")
	s.AssertNotStorageContext(c, ctx)
	s.AssertNotWorkloadContext(c, ctx)
}

func (s *ContextFactorySuite) TestNewHookContextPrunesNonMemberCaches(c *gc.C) {

	// Write cached member settings for a member and a non-member.
	s.setUpCacheMethods(c)
	s.membership[0] = []string{"rel0/0"}
	s.updateCache(0, "rel0/0", params.Settings{"keep": "me"})
	s.updateCache(0, "rel0/1", params.Settings{"drop": "me"})

	ctx, err := s.factory.HookContext(hook.Info{Kind: hooks.Install})
	c.Assert(err, jc.ErrorIsNil)

	settings0, found := s.getCache(0, "rel0/0")
	c.Assert(found, jc.IsTrue)
	c.Assert(settings0, jc.DeepEquals, params.Settings{"keep": "me"})

	settings1, found := s.getCache(0, "rel0/1")
	c.Assert(found, jc.IsFalse)
	c.Assert(settings1, gc.IsNil)

	// Check the caches are being used by the context relations.
	relCtx, err := ctx.Relation(0)
	c.Assert(err, jc.ErrorIsNil)

	// Verify that the settings really were cached by trying to look them up.
	// Nothing's really in scope, so the call would fail if they weren't.
	settings0, err = relCtx.ReadSettings("rel0/0")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(settings0, jc.DeepEquals, params.Settings{"keep": "me"})

	// Verify that the non-member settings were purged by looking them up and
	// checking for the expected error.
	settings1, err = relCtx.ReadSettings("rel0/1")
	c.Assert(settings1, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, "permission denied")
}

func (s *ContextFactorySuite) TestNewHookContextRelationJoinedUpdatesRelationContextAndCaches(c *gc.C) {
	// Write some cached settings for r/0, so we can verify the cache gets cleared.
	s.setUpCacheMethods(c)
	s.membership[1] = []string{"r/0"}
	s.updateCache(1, "r/0", params.Settings{"foo": "bar"})

	ctx, err := s.factory.HookContext(hook.Info{
		Kind:              hooks.RelationJoined,
		RelationId:        1,
		RemoteUnit:        "r/0",
		RemoteApplication: "r",
	})
	c.Assert(err, jc.ErrorIsNil)
	s.AssertCoreContext(c, ctx)
	s.AssertNotActionContext(c, ctx)
	s.AssertNotStorageContext(c, ctx)
	s.AssertNotWorkloadContext(c, ctx)
	rel := s.AssertRelationContext(c, ctx, 1, "r/0", "r")
	c.Assert(rel.UnitNames(), jc.DeepEquals, []string{"r/0"})
	cached0, member := s.getCache(1, "r/0")
	c.Assert(cached0, gc.IsNil)
	c.Assert(member, jc.IsTrue)
}

func (s *ContextFactorySuite) TestNewHookContextRelationChangedUpdatesRelationContextAndCaches(c *gc.C) {
	// Update member settings to have actual values, so we can check that
	// the change for r/4 clears its cache but leaves r/0's alone.
	s.setUpCacheMethods(c)
	s.membership[1] = []string{"r/0", "r/4"}
	s.updateCache(1, "r/0", params.Settings{"foo": "bar"})
	s.updateCache(1, "r/4", params.Settings{"baz": "qux"})
	s.updateAppCache(1, "r", params.Settings{"frob": "nizzle"})

	ctx, err := s.factory.HookContext(hook.Info{
		Kind:              hooks.RelationChanged,
		RelationId:        1,
		RemoteUnit:        "r/4",
		RemoteApplication: "r",
	})
	c.Assert(err, jc.ErrorIsNil)
	s.AssertCoreContext(c, ctx)
	s.AssertNotActionContext(c, ctx)
	s.AssertNotStorageContext(c, ctx)
	s.AssertNotWorkloadContext(c, ctx)
	rel := s.AssertRelationContext(c, ctx, 1, "r/4", "r")
	c.Assert(rel.UnitNames(), jc.DeepEquals, []string{"r/0", "r/4"})
	cached0, member := s.getCache(1, "r/0")
	c.Assert(cached0, jc.DeepEquals, params.Settings{"foo": "bar"})
	c.Assert(member, jc.IsTrue)
	cached4, member := s.getCache(1, "r/4")
	c.Assert(cached4, gc.IsNil)
	c.Assert(member, jc.IsTrue)
	wrongCache, member := s.getCache(1, "r")
	c.Assert(wrongCache, gc.IsNil)
	c.Assert(member, jc.IsFalse)
	cachedApp, found := s.getAppCache(1, "r")
	// TODO(jam): 2019-10-23 This is currently wrong. We are currently pruning
	//  all application settings on every hook invocation. We should only
	//  invalidate it when we run a relation-changed hook for the app
	c.ExpectFailure("application settings should be properly cached")
	c.Assert(cachedApp, jc.DeepEquals, params.Settings{"frob": "bar"})
	c.Assert(found, jc.IsTrue)
}

func (s *ContextFactorySuite) TestNewHookContextRelationChangedUpdatesRelationContextAndCachesApplication(c *gc.C) {
	// Set values for r/0 and r make sure we don't see r/0 change but we *do* see r wiped.
	s.setUpCacheMethods(c)
	s.membership[1] = []string{"r/0"}
	s.updateCache(1, "r/0", params.Settings{"foo": "bar"})
	s.updateAppCache(1, "r", params.Settings{"baz": "quux"})
	cachedApp, found := s.getAppCache(1, "r")
	c.Assert(cachedApp, jc.DeepEquals, params.Settings{"baz": "quux"})
	c.Assert(found, jc.IsTrue)

	ctx, err := s.factory.HookContext(hook.Info{
		Kind:              hooks.RelationChanged,
		RelationId:        1,
		RemoteApplication: "r",
	})
	c.Assert(err, jc.ErrorIsNil)
	s.AssertCoreContext(c, ctx)
	s.AssertNotActionContext(c, ctx)
	s.AssertNotStorageContext(c, ctx)
	s.AssertNotWorkloadContext(c, ctx)
	rel := s.AssertRelationContext(c, ctx, 1, "", "r")
	c.Assert(rel.UnitNames(), jc.DeepEquals, []string{"r/0"})
	cached0, member := s.getCache(1, "r/0")
	c.Assert(cached0, jc.DeepEquals, params.Settings{"foo": "bar"})
	c.Assert(member, jc.IsTrue)
	// It should not be found in the normal cache
	wrongCache, member := s.getCache(1, "r")
	c.Assert(wrongCache, gc.IsNil)
	c.Assert(member, jc.IsFalse)
	cachedApp, found = s.getAppCache(1, "r")
	c.Assert(cachedApp, gc.IsNil)
	c.Assert(found, jc.IsFalse)
}

func (s *ContextFactorySuite) TestNewHookContextRelationDepartedUpdatesRelationContextAndCaches(c *gc.C) {
	// Update member settings to have actual values, so we can check that
	// the depart for r/0 leaves r/4's cache alone (while discarding r/0's).
	s.setUpCacheMethods(c)
	s.membership[1] = []string{"r/0", "r/4"}
	s.updateCache(1, "r/0", params.Settings{"foo": "bar"})
	s.updateCache(1, "r/4", params.Settings{"baz": "qux"})

	ctx, err := s.factory.HookContext(hook.Info{
		Kind:          hooks.RelationDeparted,
		RelationId:    1,
		RemoteUnit:    "r/0",
		DepartingUnit: "r/0",
	})
	c.Assert(err, jc.ErrorIsNil)
	s.AssertCoreContext(c, ctx)
	s.AssertNotActionContext(c, ctx)
	s.AssertNotStorageContext(c, ctx)
	s.AssertNotWorkloadContext(c, ctx)
	rel := s.AssertRelationContext(c, ctx, 1, "r/0", "")
	c.Assert(rel.UnitNames(), jc.DeepEquals, []string{"r/4"})
	cached0, member := s.getCache(1, "r/0")
	c.Assert(cached0, gc.IsNil)
	c.Assert(member, jc.IsFalse)
	cached4, member := s.getCache(1, "r/4")
	c.Assert(cached4, jc.DeepEquals, params.Settings{"baz": "qux"})
	c.Assert(member, jc.IsTrue)
}

func (s *ContextFactorySuite) TestNewHookContextRelationBrokenRetainsCaches(c *gc.C) {
	// Note that this is bizarre and unrealistic, because we would never usually
	// run relation-broken on a non-empty relation. But verfying that the settings
	// stick around allows us to verify that there's no special handling for that
	// hook -- as there should not be, because the relation caches will be discarded
	// for the *next* hook, which will be constructed with the current set of known
	// relations and ignore everything else.
	s.setUpCacheMethods(c)
	s.membership[1] = []string{"r/0", "r/4"}
	s.updateCache(1, "r/0", params.Settings{"foo": "bar"})
	s.updateCache(1, "r/4", params.Settings{"baz": "qux"})

	ctx, err := s.factory.HookContext(hook.Info{
		Kind:       hooks.RelationBroken,
		RelationId: 1,
	})
	c.Assert(err, jc.ErrorIsNil)
	rel := s.AssertRelationContext(c, ctx, 1, "", "")
	c.Assert(rel.UnitNames(), jc.DeepEquals, []string{"r/0", "r/4"})
	cached0, member := s.getCache(1, "r/0")
	c.Assert(cached0, jc.DeepEquals, params.Settings{"foo": "bar"})
	c.Assert(member, jc.IsTrue)
	cached4, member := s.getCache(1, "r/4")
	c.Assert(cached4, jc.DeepEquals, params.Settings{"baz": "qux"})
	c.Assert(member, jc.IsTrue)
}

func (s *ContextFactorySuite) TestReadApplicationSettings(c *gc.C) {
	s.setUpCacheMethods(c)
	// First, try to read the ApplicationSettings but not as the leader, ensure we get an error
	// Make sure this unit is the leader
	ctx, err := s.factory.HookContext(hook.Info{Kind: hooks.Install})
	c.Assert(err, jc.ErrorIsNil)
	s.membership[0] = []string{"r/0"}
	rel, err := ctx.Relation(0)
	c.Assert(err, jc.ErrorIsNil)
	_, err = rel.ApplicationSettings()
	c.Assert(err, gc.ErrorMatches, "permission denied.*")
	// Now claim leadership and try again
	claimer, err := s.LeaseManager.Claimer("application-leadership", s.State.ModelUUID())
	c.Assert(err, jc.ErrorIsNil)
	err = claimer.Claim(s.unit.ApplicationName(), s.unit.Name(), time.Minute)
	c.Assert(err, jc.ErrorIsNil)
	settings, err := rel.ApplicationSettings()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(settings.Map(), jc.DeepEquals, params.Settings{})
}

type StubLeadershipContext struct {
	context.LeadershipContext
	*testing.Stub
	isLeader bool
}

func (stub *StubLeadershipContext) IsLeader() (bool, error) {
	stub.MethodCall(stub, "IsLeader")
	return stub.isLeader, stub.NextErr()
}
