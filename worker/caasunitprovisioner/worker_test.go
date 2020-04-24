// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasunitprovisioner_test

import (
	"time"

	"github.com/golang/mock/gomock"
	"github.com/juju/clock/testclock"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names/v4"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	"github.com/juju/worker/v2"
	"github.com/juju/worker/v2/workertest"
	gc "gopkg.in/check.v1"

	apicaasunitprovisioner "github.com/juju/juju/api/caasunitprovisioner"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/caas"
	"github.com/juju/juju/caas/specs"
	"github.com/juju/juju/core/application"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/core/watcher/watchertest"
	"github.com/juju/juju/storage"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker/caasunitprovisioner"
)

type WorkerSuite struct {
	testing.IsolationSuite

	config             caasunitprovisioner.Config
	applicationGetter  mockApplicationGetter
	applicationUpdater mockApplicationUpdater
	serviceBroker      mockServiceBroker
	containerBroker    mockContainerBroker
	podSpecGetter      mockProvisioningInfoGetterGetter
	lifeGetter         mockLifeGetter
	unitUpdater        mockUnitUpdater
	statusSetter       *caasunitprovisioner.MockProvisioningStatusSetter

	applicationChanges      chan []string
	applicationScaleChanges chan struct{}
	caasUnitsChanges        chan struct{}
	caasServiceChanges      chan struct{}
	caasOperatorChanges     chan struct{}
	containerSpecChanges    chan struct{}
	serviceDeleted          chan struct{}
	serviceEnsured          chan struct{}
	serviceUpdated          chan struct{}
	resourcesCleared        chan struct{}
	clock                   *testclock.Clock
}

var _ = gc.Suite(&WorkerSuite{})

var (
	containerSpec = `
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
)

func getParsedSpec() *specs.PodSpec {
	parsedSpec := &specs.PodSpec{}
	parsedSpec.Version = specs.CurrentVersion
	parsedSpec.Containers = []specs.ContainerSpec{
		{
			Name:  "gitlab",
			Image: "gitlab/latest",
			Ports: []specs.ContainerPort{
				{ContainerPort: 80, Protocol: "TCP"},
				{ContainerPort: 443},
			},
			EnvConfig: map[string]interface{}{
				"attr": "foo=bar; fred=blogs",
				"foo":  "bar",
			},
		},
	}
	return parsedSpec
}

func getExpectedServiceParams() *caas.ServiceParams {
	parsedSpec := getParsedSpec()
	return &caas.ServiceParams{
		PodSpec:      parsedSpec,
		ResourceTags: map[string]string{"foo": "bar"},
		Constraints:  constraints.MustParse("mem=4G"),
		Deployment: caas.DeploymentParams{
			DeploymentType: caas.DeploymentStateful,
			ServiceType:    caas.ServiceLoadBalancer,
		},
		Filesystems: []storage.KubernetesFilesystemParams{{
			StorageName: "database",
			Size:        100,
		}},
	}
}

func (s *WorkerSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)

	s.applicationChanges = make(chan []string)
	s.applicationScaleChanges = make(chan struct{})
	s.caasUnitsChanges = make(chan struct{})
	s.caasServiceChanges = make(chan struct{})
	s.caasOperatorChanges = make(chan struct{})
	s.containerSpecChanges = make(chan struct{}, 1)
	s.serviceDeleted = make(chan struct{})
	s.serviceEnsured = make(chan struct{})
	s.serviceUpdated = make(chan struct{})
	s.resourcesCleared = make(chan struct{})

	s.applicationGetter = mockApplicationGetter{
		watcher:        watchertest.NewMockStringsWatcher(s.applicationChanges),
		scaleWatcher:   watchertest.NewMockNotifyWatcher(s.applicationScaleChanges),
		deploymentMode: caas.ModeWorkload,
	}
	s.applicationUpdater = mockApplicationUpdater{
		updated: s.serviceUpdated,
		cleared: s.resourcesCleared,
	}

	s.podSpecGetter = mockProvisioningInfoGetterGetter{
		watcher: watchertest.NewMockNotifyWatcher(s.containerSpecChanges),
	}
	s.podSpecGetter.setProvisioningInfo(apicaasunitprovisioner.ProvisioningInfo{
		PodSpec:     containerSpec,
		Tags:        map[string]string{"foo": "bar"},
		Constraints: constraints.MustParse("mem=4G"),
		DeploymentInfo: apicaasunitprovisioner.DeploymentInfo{
			DeploymentType: "stateful",
			ServiceType:    "loadbalancer",
		},
		Filesystems: []storage.KubernetesFilesystemParams{{
			StorageName: "database",
			Size:        100,
		}},
	})

	s.unitUpdater = mockUnitUpdater{}

	s.containerBroker = mockContainerBroker{
		unitsWatcher:    watchertest.NewMockNotifyWatcher(s.caasUnitsChanges),
		operatorWatcher: watchertest.NewMockNotifyWatcher(s.caasOperatorChanges),
		units: []caas.Unit{
			{
				Id:       "u1",
				Address:  "10.0.0.1",
				Stateful: true,
				FilesystemInfo: []caas.FilesystemInfo{
					{MountPoint: "/path-to-here", ReadOnly: true, StorageName: "database",
						Size: 100, FilesystemId: "fs-id",
						Status: status.StatusInfo{Status: status.Attaching, Message: "not ready"},
						Volume: caas.VolumeInfo{VolumeId: "vol-id", Size: 200, Persistent: true,
							Status: status.StatusInfo{Status: status.Error, Message: "vol not ready"}},
					},
				},
			},
		},
	}
	s.lifeGetter = mockLifeGetter{}
	s.lifeGetter.setLife(life.Alive)
	s.serviceBroker = mockServiceBroker{
		ensured:        s.serviceEnsured,
		deleted:        s.serviceDeleted,
		serviceWatcher: watchertest.NewMockNotifyWatcher(s.caasServiceChanges),
	}
}

func (s *WorkerSuite) sendContainerSpecChange(c *gc.C) {
	select {
	case s.containerSpecChanges <- struct{}{}:
	case <-time.After(coretesting.LongWait):
		c.Fatal("timed out sending pod spec change")
	}
}

func (s *WorkerSuite) TestValidateConfig(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.testValidateConfig(c, func(config *caasunitprovisioner.Config) {
		config.ApplicationGetter = nil
	}, `missing ApplicationGetter not valid`)

	s.testValidateConfig(c, func(config *caasunitprovisioner.Config) {
		config.ApplicationUpdater = nil
	}, `missing ApplicationUpdater not valid`)

	s.testValidateConfig(c, func(config *caasunitprovisioner.Config) {
		config.ServiceBroker = nil
	}, `missing ServiceBroker not valid`)

	s.testValidateConfig(c, func(config *caasunitprovisioner.Config) {
		config.ContainerBroker = nil
	}, `missing ContainerBroker not valid`)

	s.testValidateConfig(c, func(config *caasunitprovisioner.Config) {
		config.ProvisioningInfoGetter = nil
	}, `missing ProvisioningInfoGetter not valid`)

	s.testValidateConfig(c, func(config *caasunitprovisioner.Config) {
		config.LifeGetter = nil
	}, `missing LifeGetter not valid`)

	s.testValidateConfig(c, func(config *caasunitprovisioner.Config) {
		config.ProvisioningStatusSetter = nil
	}, `missing ProvisioningStatusSetter not valid`)

	s.testValidateConfig(c, func(config *caasunitprovisioner.Config) {
		config.Logger = nil
	}, `missing Logger not valid`)
}

func (s *WorkerSuite) testValidateConfig(c *gc.C, f func(*caasunitprovisioner.Config), expect string) {
	config := s.config
	f(&config)
	w, err := caasunitprovisioner.NewWorker(config)
	if err == nil {
		workertest.DirtyKill(c, w)
	}
	c.Check(err, gc.ErrorMatches, expect)
}

func (s *WorkerSuite) TestStartStop(c *gc.C) {
	defer s.setupMocks(c).Finish()

	w, err := caasunitprovisioner.NewWorker(s.config)
	c.Assert(err, jc.ErrorIsNil)
	workertest.CheckAlive(c, w)
	workertest.CleanKill(c, w)
}

func (s *WorkerSuite) setupNewUnitScenario(c *gc.C) worker.Worker {
	s.statusSetter.EXPECT().SetOperatorStatus(
		"gitlab", status.Waiting, "ensuring", map[string]interface{}{"foo": "bar"}).MinTimes(1)

	w, err := caasunitprovisioner.NewWorker(s.config)
	c.Assert(err, jc.ErrorIsNil)

	s.podSpecGetter.SetErrors(nil, errors.NotFoundf("spec"))

	select {
	case s.applicationChanges <- []string{"gitlab"}:
	case <-time.After(coretesting.LongWait):
		c.Fatal("timed out sending applications change")
	}

	s.applicationGetter.scale = 1
	select {
	case s.applicationScaleChanges <- struct{}{}:
	case <-time.After(coretesting.LongWait):
		c.Fatal("timed out sending scale change")
	}

	// We seed a "not found" error above to indicate that
	// there is not yet a pod spec; the broker should
	// not be invoked.
	s.sendContainerSpecChange(c)
	select {
	case <-s.serviceEnsured:
		c.Fatal("service ensured unexpectedly")
	case <-time.After(coretesting.ShortWait):
	}

	s.sendContainerSpecChange(c)
	s.podSpecGetter.assertSpecRetrieved(c)
	select {
	case <-s.serviceEnsured:
	case <-time.After(coretesting.LongWait):
		c.Fatal("timed out waiting for service to be ensured")
	}

	select {
	case <-s.serviceUpdated:
	case <-time.After(coretesting.LongWait):
		c.Fatal("timed out waiting for service to be updated")
	}
	return w
}

func (s *WorkerSuite) TestScaleChangedInJuju(c *gc.C) {
	defer s.setupMocks(c).Finish()

	w := s.setupNewUnitScenario(c)
	defer workertest.CleanKill(c, w)

	s.applicationGetter.CheckCallNames(c, "WatchApplications", "DeploymentMode", "WatchApplicationScale", "ApplicationScale", "ApplicationConfig")
	s.podSpecGetter.CheckCallNames(c, "WatchPodSpec", "ProvisioningInfo", "ProvisioningInfo")
	s.podSpecGetter.CheckCall(c, 0, "WatchPodSpec", "gitlab")
	s.podSpecGetter.CheckCall(c, 1, "ProvisioningInfo", "gitlab") // not found
	s.podSpecGetter.CheckCall(c, 2, "ProvisioningInfo", "gitlab")
	s.lifeGetter.CheckCallNames(c, "Life")
	s.lifeGetter.CheckCall(c, 0, "Life", "gitlab")
	s.serviceBroker.CheckCallNames(c, "WatchService", "EnsureService", "GetService")
	s.serviceBroker.CheckCall(c, 1, "EnsureService",
		"gitlab", getExpectedServiceParams(), 1, application.ConfigAttributes{"juju-external-hostname": "exthost"})
	s.serviceBroker.CheckCall(c, 2, "GetService", "gitlab", caas.ModeWorkload)

	s.serviceBroker.ResetCalls()
	// Add another unit.
	s.applicationGetter.scale = 2
	select {
	case s.applicationScaleChanges <- struct{}{}:
	case <-time.After(coretesting.LongWait):
		c.Fatal("timed out sending scale change")
	}

	select {
	case <-s.serviceEnsured:
	case <-time.After(coretesting.LongWait):
		c.Fatal("timed out waiting for service to be ensured")
	}

	newExpectedParams := getExpectedServiceParams()
	s.serviceBroker.CheckCallNames(c, "EnsureService")
	s.serviceBroker.CheckCall(c, 0, "EnsureService",
		"gitlab", newExpectedParams, 2, application.ConfigAttributes{"juju-external-hostname": "exthost"})

	s.serviceBroker.ResetCalls()
	// Delete a unit.
	s.applicationGetter.scale = 1
	select {
	case s.applicationScaleChanges <- struct{}{}:
	case <-time.After(coretesting.LongWait):
		c.Fatal("timed out sending scale change")
	}

	select {
	case <-s.serviceEnsured:
	case <-time.After(coretesting.LongWait):
		c.Fatal("timed out waiting for service to be ensured")
	}

	s.serviceBroker.CheckCallNames(c, "EnsureService")
	s.serviceBroker.CheckCall(c, 0, "EnsureService",
		"gitlab", newExpectedParams, 1, application.ConfigAttributes{"juju-external-hostname": "exthost"})
}

func intPtr(i int) *int {
	return &i
}

func (s *WorkerSuite) TestScaleChangedInCluster(c *gc.C) {
	defer s.setupMocks(c).Finish()

	w := s.setupNewUnitScenario(c)
	defer workertest.CleanKill(c, w)

	s.containerBroker.ResetCalls()
	s.applicationUpdater.ResetCalls()
	s.serviceBroker.ResetCalls()
	s.serviceBroker.serviceStatus = status.StatusInfo{
		Status:  status.Active,
		Message: "working",
	}

	s.unitUpdater.unitsInfo = &params.UpdateApplicationUnitsInfo{
		Units: []params.ApplicationUnitInfo{
			{ProviderId: "u1", UnitTag: "unit-gitlab-0"},
		},
	}

	select {
	case s.caasServiceChanges <- struct{}{}:
	case <-time.After(coretesting.LongWait):
		c.Fatal("timed out sending service change")
	}

	for a := coretesting.LongAttempt.Start(); a.Next(); {
		if len(s.serviceBroker.Calls()) > 0 {
			break
		}
	}
	s.serviceBroker.CheckCallNames(c, "GetService")
	c.Assert(s.serviceBroker.Calls()[0].Args, jc.DeepEquals, []interface{}{"gitlab", caas.ModeWorkload})

	select {
	case <-s.serviceUpdated:
		s.applicationUpdater.CheckCallNames(c, "UpdateApplicationService")
		c.Assert(s.applicationUpdater.Calls()[0].Args, jc.DeepEquals, []interface{}{
			params.UpdateApplicationServiceArg{
				ApplicationTag: names.NewApplicationTag("gitlab").String(),
				ProviderId:     "id",
				Addresses:      params.FromProviderAddresses(network.NewProviderAddresses("10.0.0.1")...),
				Scale:          intPtr(4),
			},
		})
	case <-time.After(coretesting.LongWait):
		c.Fatal("timed out waiting for service to be updated")
	}

	for a := coretesting.LongAttempt.Start(); a.Next(); {
		if len(s.containerBroker.Calls()) >= 2 {
			break
		}
	}
	if !s.containerBroker.CheckCallNames(c, "Units", "AnnotateUnit") {
		return
	}
	c.Assert(s.containerBroker.Calls()[0].Args, jc.DeepEquals, []interface{}{"gitlab", caas.ModeWorkload})
	c.Assert(s.containerBroker.Calls()[1].Args, jc.DeepEquals, []interface{}{"gitlab", caas.ModeWorkload, "u1", names.NewUnitTag("gitlab/0")})

	for a := coretesting.LongAttempt.Start(); a.Next(); {
		if len(s.unitUpdater.Calls()) > 0 {
			break
		}
	}
	s.unitUpdater.CheckCallNames(c, "UpdateUnits")
	scale := 4
	c.Assert(s.unitUpdater.Calls()[0].Args, jc.DeepEquals, []interface{}{
		params.UpdateApplicationUnits{
			ApplicationTag: names.NewApplicationTag("gitlab").String(),
			Scale:          &scale,
			Status: params.EntityStatus{
				Status: status.Active,
				Info:   "working",
			},
			Units: []params.ApplicationUnitParams{
				{ProviderId: "u1", Address: "10.0.0.1", Ports: []string(nil),
					Stateful: true,
					FilesystemInfo: []params.KubernetesFilesystemInfo{
						{StorageName: "database", MountPoint: "/path-to-here", ReadOnly: true,
							FilesystemId: "fs-id", Size: 100, Pool: "",
							Volume: params.KubernetesVolumeInfo{
								VolumeId: "vol-id", Size: 200,
								Persistent: true, Status: "error", Info: "vol not ready"},
							Status: "attaching", Info: "not ready"},
					}},
			},
		},
	})
}

func (s *WorkerSuite) TestNewPodSpecChange(c *gc.C) {
	defer s.setupMocks(c).Finish()

	w := s.setupNewUnitScenario(c)
	defer workertest.CleanKill(c, w)

	s.serviceBroker.ResetCalls()

	// Same spec, nothing happens.
	s.sendContainerSpecChange(c)
	s.podSpecGetter.assertSpecRetrieved(c)
	select {
	case <-s.serviceEnsured:
		c.Fatal("service/unit ensured unexpectedly")
	case <-time.After(coretesting.ShortWait):
	}

	var (
		anotherSpec = `
containers:
  - name: gitlab
    image: gitlab/latest
`[1:]
	)
	anotherParsedSpec := &specs.PodSpec{}
	anotherParsedSpec.Version = specs.CurrentVersion
	anotherParsedSpec.Containers = []specs.ContainerSpec{{
		Name:  "gitlab",
		Image: "gitlab/latest",
	}}

	s.podSpecGetter.setProvisioningInfo(apicaasunitprovisioner.ProvisioningInfo{
		PodSpec:     anotherSpec,
		Tags:        map[string]string{"foo": "bar"},
		Constraints: constraints.MustParse("mem=4G"),
		DeploymentInfo: apicaasunitprovisioner.DeploymentInfo{
			DeploymentType: "stateful",
			ServiceType:    "loadbalancer",
		},
		Filesystems: []storage.KubernetesFilesystemParams{{
			StorageName: "database",
			Size:        100,
		}},
	})
	s.sendContainerSpecChange(c)
	s.podSpecGetter.assertSpecRetrieved(c)

	select {
	case <-s.serviceEnsured:
	case <-time.After(coretesting.LongWait):
		c.Fatal("timed out waiting for service to be ensured")
	}

	expectedParams := &caas.ServiceParams{
		PodSpec:      anotherParsedSpec,
		ResourceTags: map[string]string{"foo": "bar"},
		Constraints:  constraints.MustParse("mem=4G"),
		Deployment: caas.DeploymentParams{
			DeploymentType: "stateful",
			ServiceType:    "loadbalancer",
		},
		Filesystems: []storage.KubernetesFilesystemParams{{
			StorageName: "database",
			Size:        100,
		}},
	}
	s.serviceBroker.CheckCallNames(c, "EnsureService")
	s.serviceBroker.CheckCall(c, 0, "EnsureService",
		"gitlab", expectedParams, 1, application.ConfigAttributes{"juju-external-hostname": "exthost"})
}

func (s *WorkerSuite) TestInvalidDeploymentChange(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.statusSetter.EXPECT().SetOperatorStatus(
		"gitlab", status.Error, "k8s does not support updating storage", map[string]interface{}(nil))

	w := s.setupNewUnitScenario(c)
	defer workertest.CleanKill(c, w)

	s.serviceBroker.ResetCalls()

	// Same spec, nothing happens.
	s.sendContainerSpecChange(c)
	s.podSpecGetter.assertSpecRetrieved(c)
	select {
	case <-s.serviceEnsured:
		c.Fatal("service/unit ensured unexpectedly")
	case <-time.After(coretesting.ShortWait):
	}

	var (
		anotherSpec = `
containers:
  - name: gitlab
    image: gitlab/latest
`[1:]
	)
	anotherParsedSpec := &specs.PodSpec{}
	anotherParsedSpec.Version = specs.CurrentVersion
	anotherParsedSpec.Containers = []specs.ContainerSpec{{
		Name:  "gitlab",
		Image: "gitlab/latest",
	}}

	s.podSpecGetter.setProvisioningInfo(apicaasunitprovisioner.ProvisioningInfo{
		PodSpec:     anotherSpec,
		Tags:        map[string]string{"foo": "bar"},
		Constraints: constraints.MustParse("mem=4G"),
		DeploymentInfo: apicaasunitprovisioner.DeploymentInfo{
			DeploymentType: "stateful",
			ServiceType:    "loadbalancer",
		},
		Filesystems: []storage.KubernetesFilesystemParams{{
			StorageName: "database",
			Size:        100,
		}, {
			StorageName: "logs",
			Size:        100,
		}},
	})
	s.sendContainerSpecChange(c)
	s.podSpecGetter.assertSpecRetrieved(c)

	c.Assert(s.serviceBroker.Calls(), gc.HasLen, 0)
}

func (s *WorkerSuite) TestScaleZero(c *gc.C) {
	defer s.setupMocks(c).Finish()

	w := s.setupNewUnitScenario(c)
	defer workertest.CleanKill(c, w)

	s.serviceBroker.ResetCalls()
	// Add another unit.
	s.applicationGetter.scale = 2
	select {
	case s.applicationScaleChanges <- struct{}{}:
	case <-time.After(coretesting.LongWait):
		c.Fatal("timed out sending scale change")
	}

	select {
	case <-s.serviceEnsured:
	case <-time.After(coretesting.LongWait):
		c.Fatal("timed out waiting for service to be ensured")
	}
	s.serviceBroker.ResetCalls()

	// Now the scale down to 0.
	s.applicationGetter.scale = 0
	select {
	case s.applicationScaleChanges <- struct{}{}:
	case <-time.After(coretesting.LongWait):
		c.Fatal("timed out sending scale change")
	}

	select {
	case <-s.serviceEnsured:
	case <-time.After(coretesting.LongWait):
		c.Fatal("timed out waiting for service to be ensured")
	}
	s.serviceBroker.CheckCallNames(c, "EnsureService")
	s.serviceBroker.CheckCall(c, 0, "EnsureService",
		"gitlab", &caas.ServiceParams{}, 0, application.ConfigAttributes(nil))
}

func (s *WorkerSuite) TestApplicationDeadRemovesService(c *gc.C) {
	defer s.setupMocks(c).Finish()

	w := s.setupNewUnitScenario(c)
	defer workertest.CleanKill(c, w)

	s.serviceBroker.ResetCalls()
	s.containerBroker.ResetCalls()

	s.lifeGetter.SetErrors(errors.NotFoundf("application"))
	select {
	case s.applicationChanges <- []string{"gitlab"}:
	case <-time.After(coretesting.LongWait):
		c.Fatal("timed out sending application change")
	}

	select {
	case <-s.serviceDeleted:
	case <-time.After(coretesting.LongWait):
		c.Fatal("timed out waiting for service to be deleted")
	}

	s.serviceBroker.CheckCallNames(c, "UnexposeService", "DeleteService")
	s.serviceBroker.CheckCall(c, 0, "UnexposeService", "gitlab")
	s.serviceBroker.CheckCall(c, 1, "DeleteService", "gitlab")
}

func (s *WorkerSuite) TestWatchApplicationDead(c *gc.C) {
	defer s.setupMocks(c).Finish()

	w, err := caasunitprovisioner.NewWorker(s.config)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.CleanKill(c, w)

	s.lifeGetter.setLife(life.Dead)
	select {
	case s.applicationChanges <- []string{"gitlab"}:
	case <-time.After(coretesting.LongWait):
		c.Fatal("timed out sending applications change")
	}

	select {
	case <-s.serviceDeleted:
	case <-time.After(coretesting.LongWait):
		c.Fatal("timed out waiting for service to be deleted")
	}

	select {
	case s.applicationScaleChanges <- struct{}{}:
		c.Fatal("unexpected watch for application scale")
	case <-time.After(coretesting.ShortWait):
	}

	workertest.CleanKill(c, w)
	// There should just be the initial watch call, no subsequent calls to watch/get scale etc.
	s.applicationGetter.CheckCallNames(c, "WatchApplications", "DeploymentMode")
}

func (s *WorkerSuite) TestRemoveApplicationStopsWatchingApplicationScale(c *gc.C) {
	defer s.setupMocks(c).Finish()

	w, err := caasunitprovisioner.NewWorker(s.config)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.CleanKill(c, w)

	select {
	case s.applicationChanges <- []string{"gitlab"}:
	case <-time.After(coretesting.LongWait):
		c.Fatal("timed out sending applications change")
	}

	// Check that the gitlab worker is running or not;
	// given it time to startup.
	shortAttempt := &utils.AttemptStrategy{
		Total: coretesting.LongWait,
		Delay: 10 * time.Millisecond,
	}
	running := false
	for a := shortAttempt.Start(); a.Next(); {
		_, running = caasunitprovisioner.AppWorker(w, "gitlab")
		if running {
			break
		}
	}
	c.Assert(running, jc.IsTrue)

	// Add an additional app worker so we can check that the correct one is accessed.
	caasunitprovisioner.NewAppWorker(w, "mysql")

	s.lifeGetter.SetErrors(errors.NotFoundf("application"))
	select {
	case s.applicationChanges <- []string{"gitlab"}:
	case <-time.After(coretesting.LongWait):
		c.Fatal("timed out sending applications change")
	}

	select {
	case <-s.serviceDeleted:
	case <-time.After(coretesting.LongWait):
		c.Fatal("timed out waiting for service to be deleted")
	}

	// The mysql worker should still be running.
	_, ok := caasunitprovisioner.AppWorker(w, "mysql")
	c.Assert(ok, jc.IsTrue)

	// Check that the gitlab worker is running or not;
	// given it time to shutdown.
	for a := shortAttempt.Start(); a.Next(); {
		_, running = caasunitprovisioner.AppWorker(w, "gitlab")
		if !running {
			break
		}
	}
	c.Assert(running, jc.IsFalse)
	workertest.CheckKilled(c, s.applicationGetter.scaleWatcher)
}

func (s *WorkerSuite) TestRemoveWorkloadApplicationWaitsForResources(c *gc.C) {
	defer s.setupMocks(c).Finish()

	w, err := caasunitprovisioner.NewWorker(s.config)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.CleanKill(c, w)

	select {
	case s.applicationChanges <- []string{"gitlab"}:
	case <-time.After(coretesting.LongWait):
		c.Fatal("timed out sending applications change")
	}

	// Check that the gitlab worker is running or not;
	// given it time to startup.
	shortAttempt := &utils.AttemptStrategy{
		Total: coretesting.LongWait,
		Delay: 10 * time.Millisecond,
	}
	running := false
	for a := shortAttempt.Start(); a.Next(); {
		_, running = caasunitprovisioner.AppWorker(w, "gitlab")
		if running {
			break
		}
	}
	c.Assert(running, jc.IsTrue)

	s.lifeGetter.SetErrors(errors.NotFoundf("application"))
	select {
	case s.applicationChanges <- []string{"gitlab"}:
	case <-time.After(coretesting.LongWait):
		c.Fatal("timed out sending applications change")
	}

	select {
	case <-s.serviceDeleted:
	case <-time.After(coretesting.LongWait):
		c.Fatal("timed out waiting for service to be deleted")
	}

	// Check that the gitlab worker is running or not;
	// given it time to shutdown.
	for a := shortAttempt.Start(); a.Next(); {
		_, running = caasunitprovisioner.AppWorker(w, "gitlab")
		if !running {
			break
		}
	}
	c.Assert(running, jc.IsFalse)

	// Check the undertaker worker clears application resources.
	s.containerBroker.SetErrors(nil, errors.NotFoundf("operator"))
	s.containerBroker.units = nil

	select {
	case s.caasUnitsChanges <- struct{}{}:
	case <-time.After(coretesting.LongWait):
		c.Fatal("timed out sending units change")
	}

	select {
	case s.caasOperatorChanges <- struct{}{}:
	case <-time.After(coretesting.LongWait):
		c.Fatal("timed out sending operator change")
	}

	select {
	case <-s.resourcesCleared:
	case <-time.After(coretesting.LongWait):
		c.Fatal("timed out waiting for resources to be cleared")
	}
}

func (s *WorkerSuite) TestRemoveOperatorApplicationWaitsForResources(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.applicationGetter.deploymentMode = caas.ModeOperator

	w, err := caasunitprovisioner.NewWorker(s.config)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.CleanKill(c, w)

	select {
	case s.applicationChanges <- []string{"gitlab"}:
	case <-time.After(coretesting.LongWait):
		c.Fatal("timed out sending applications change")
	}

	// Check that the gitlab worker is running or not;
	// given it time to startup.
	shortAttempt := &utils.AttemptStrategy{
		Total: coretesting.LongWait,
		Delay: 10 * time.Millisecond,
	}
	running := false
	for a := shortAttempt.Start(); a.Next(); {
		_, running = caasunitprovisioner.AppWorker(w, "gitlab")
		if running {
			break
		}
	}
	c.Assert(running, jc.IsTrue)

	s.lifeGetter.SetErrors(errors.NotFoundf("application"))
	select {
	case s.applicationChanges <- []string{"gitlab"}:
	case <-time.After(coretesting.LongWait):
		c.Fatal("timed out sending applications change")
	}

	select {
	case <-s.serviceDeleted:
	case <-time.After(coretesting.LongWait):
		c.Fatal("timed out waiting for service to be deleted")
	}

	// Check that the gitlab worker is running or not;
	// given it time to shutdown.
	for a := shortAttempt.Start(); a.Next(); {
		_, running = caasunitprovisioner.AppWorker(w, "gitlab")
		if !running {
			break
		}
	}
	c.Assert(running, jc.IsFalse)

	// Check the undertaker worker clears application resources.
	s.containerBroker.units = nil
	select {
	case s.caasUnitsChanges <- struct{}{}:
	case <-time.After(coretesting.LongWait):
		c.Fatal("timed out sending units change")
	}

	select {
	case <-s.resourcesCleared:
	case <-time.After(coretesting.LongWait):
		c.Fatal("timed out waiting for resources to be cleared")
	}
}

func (s *WorkerSuite) TestWatcherErrorStopsWorker(c *gc.C) {
	defer s.setupMocks(c).Finish()

	w, err := caasunitprovisioner.NewWorker(s.config)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.DirtyKill(c, w)

	s.applicationGetter.scale = 1
	select {
	case s.applicationChanges <- []string{"gitlab"}:
	case <-time.After(coretesting.LongWait):
		c.Fatal("timed out sending applications change")
	}

	select {
	case s.applicationScaleChanges <- struct{}{}:
	case <-time.After(coretesting.LongWait):
		c.Fatal("timed out sending scale change")
	}

	s.podSpecGetter.watcher.KillErr(errors.New("splat"))
	workertest.CheckKilled(c, s.podSpecGetter.watcher)
	workertest.CheckKilled(c, s.applicationGetter.watcher)
	err = workertest.CheckKilled(c, w)
	c.Assert(err, gc.ErrorMatches, "splat")
}

func (s *WorkerSuite) TestUnitsChange(c *gc.C) {
	w, err := caasunitprovisioner.NewWorker(s.config)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.DirtyKill(c, w)

	select {
	case s.applicationChanges <- []string{"gitlab"}:
	case <-time.After(coretesting.LongWait):
		c.Fatal("timed out sending applications change")
	}
	defer workertest.CleanKill(c, w)

	for a := coretesting.LongAttempt.Start(); a.Next(); {
		if len(s.containerBroker.Calls()) >= 2 {
			break
		}
	}
	s.containerBroker.CheckCallNames(c, "WatchUnits", "WatchOperator")

	s.assertUnitChange(c, status.Allocating, status.Allocating)
	s.assertUnitChange(c, status.Allocating, status.Unknown)
}

func (s *WorkerSuite) TestOperatorChange(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.statusSetter.EXPECT().SetOperatorStatus(
		"gitlab", status.Active, "testing 1. 2. 3.", map[string]interface{}{"zip": "zap"})

	w, err := caasunitprovisioner.NewWorker(s.config)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.DirtyKill(c, w)

	select {
	case s.applicationChanges <- []string{"gitlab"}:
	case <-time.After(coretesting.LongWait):
		c.Fatal("timed out sending applications change")
	}

	for a := coretesting.LongAttempt.Start(); a.Next(); {
		if len(s.containerBroker.Calls()) >= 2 {
			break
		}
	}
	s.containerBroker.CheckCallNames(c, "WatchUnits", "WatchOperator")
	s.containerBroker.ResetCalls()

	// Initial event
	s.containerBroker.SetErrors(errors.NotFoundf("gitlab"))
	select {
	case s.caasOperatorChanges <- struct{}{}:
	case <-time.After(coretesting.LongWait):
		c.Fatal("timed out sending applications change")
	}

	for a := coretesting.LongAttempt.Start(); a.Next(); {
		if len(s.containerBroker.Calls()) > 0 {
			break
		}
	}

	s.containerBroker.CheckCallNames(c, "Operator")
	c.Assert(s.containerBroker.Calls()[0].Args, jc.DeepEquals, []interface{}{"gitlab"})
	s.containerBroker.ResetCalls()

	select {
	case s.caasOperatorChanges <- struct{}{}:
	case <-time.After(coretesting.LongWait):
		c.Fatal("timed out sending applications change")
	}
	s.containerBroker.reportedOperatorStatus = status.Active
	for a := coretesting.LongAttempt.Start(); a.Next(); {
		if len(s.containerBroker.Calls()) > 0 {
			break
		}
	}
	s.containerBroker.CheckCallNames(c, "Operator")
	c.Assert(s.containerBroker.Calls()[0].Args, jc.DeepEquals, []interface{}{"gitlab"})

}

func (s *WorkerSuite) assertUnitChange(c *gc.C, reported, expectedUnitStatus status.Status) {
	defer s.setupMocks(c).Finish()

	s.containerBroker.ResetCalls()
	s.unitUpdater.ResetCalls()
	s.containerBroker.reportedUnitStatus = reported

	select {
	case s.caasUnitsChanges <- struct{}{}:
	case <-time.After(coretesting.LongWait):
		c.Fatal("timed out sending units change")
	}

	for a := coretesting.LongAttempt.Start(); a.Next(); {
		if len(s.containerBroker.Calls()) > 0 {
			break
		}
	}
	s.containerBroker.CheckCallNames(c, "Units")
	c.Assert(s.containerBroker.Calls()[0].Args, jc.DeepEquals, []interface{}{"gitlab", caas.ModeWorkload})

	for a := coretesting.LongAttempt.Start(); a.Next(); {
		if len(s.unitUpdater.Calls()) > 0 {
			break
		}
	}
	s.unitUpdater.CheckCallNames(c, "UpdateUnits")
	scale := 4
	c.Assert(s.unitUpdater.Calls()[0].Args, jc.DeepEquals, []interface{}{
		params.UpdateApplicationUnits{
			ApplicationTag: names.NewApplicationTag("gitlab").String(),
			Scale:          &scale,
			Units: []params.ApplicationUnitParams{
				{ProviderId: "u1", Address: "10.0.0.1", Ports: []string(nil), Status: expectedUnitStatus.String(),
					Stateful: true,
					FilesystemInfo: []params.KubernetesFilesystemInfo{
						{StorageName: "database", MountPoint: "/path-to-here", ReadOnly: true,
							FilesystemId: "fs-id", Size: 100, Pool: "",
							Volume: params.KubernetesVolumeInfo{
								VolumeId: "vol-id", Size: 200,
								Persistent: true, Status: "error", Info: "vol not ready"},
							Status: "attaching", Info: "not ready"},
					}},
			},
		},
	})
}

func (s *WorkerSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.statusSetter = caasunitprovisioner.NewMockProvisioningStatusSetter(ctrl)

	s.config = caasunitprovisioner.Config{
		ApplicationGetter:        &s.applicationGetter,
		ApplicationUpdater:       &s.applicationUpdater,
		ServiceBroker:            &s.serviceBroker,
		ContainerBroker:          &s.containerBroker,
		ProvisioningInfoGetter:   &s.podSpecGetter,
		LifeGetter:               &s.lifeGetter,
		UnitUpdater:              &s.unitUpdater,
		ProvisioningStatusSetter: s.statusSetter,
		Logger:                   loggo.GetLogger("test"),
	}

	return ctrl
}
