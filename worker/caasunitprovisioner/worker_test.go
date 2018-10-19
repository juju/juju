// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasunitprovisioner_test

import (
	"time"

	"github.com/juju/clock/testclock"
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"
	"gopkg.in/juju/worker.v1"
	"gopkg.in/juju/worker.v1/workertest"
	apiextensionsv1beta1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"

	apicaasunitprovisioner "github.com/juju/juju/api/caasunitprovisioner"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/caas"
	"github.com/juju/juju/constraints"
	"github.com/juju/juju/core/application"
	"github.com/juju/juju/core/life"
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
	statusSetter       mockProvisioningStatusSetter

	applicationChanges      chan []string
	applicationScaleChanges chan struct{}
	caasUnitsChanges        chan struct{}
	caasOperatorChanges     chan struct{}
	containerSpecChanges    chan struct{}
	serviceDeleted          chan struct{}
	serviceEnsured          chan struct{}
	serviceUpdated          chan struct{}
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

	parsedSpec = caas.PodSpec{
		Containers: []caas.ContainerSpec{{
			Name:  "gitlab",
			Image: "gitlab/latest",
			Ports: []caas.ContainerPort{
				{ContainerPort: 80, Protocol: "TCP"},
				{ContainerPort: 443},
			},
			Config: map[string]interface{}{
				"attr": "foo=bar; fred=blogs",
				"foo":  "bar",
			}},
		}}

	expectedServiceParams = &caas.ServiceParams{
		PodSpec:      &parsedSpec,
		ResourceTags: map[string]string{"foo": "bar"},
		Placement:    "placement",
		Constraints:  constraints.MustParse("mem=4G"),
		Filesystems: []storage.KubernetesFilesystemParams{{
			StorageName: "database",
			Size:        100,
		}},
	}
)

func (s *WorkerSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)

	s.applicationChanges = make(chan []string)
	s.applicationScaleChanges = make(chan struct{})
	s.caasUnitsChanges = make(chan struct{})
	s.caasOperatorChanges = make(chan struct{})
	s.containerSpecChanges = make(chan struct{}, 1)
	s.serviceDeleted = make(chan struct{})
	s.serviceEnsured = make(chan struct{})
	s.serviceUpdated = make(chan struct{})

	s.applicationGetter = mockApplicationGetter{
		watcher:      watchertest.NewMockStringsWatcher(s.applicationChanges),
		scaleWatcher: watchertest.NewMockNotifyWatcher(s.applicationScaleChanges),
	}
	s.applicationUpdater = mockApplicationUpdater{
		updated: s.serviceUpdated,
	}

	s.podSpecGetter = mockProvisioningInfoGetterGetter{
		watcher: watchertest.NewMockNotifyWatcher(s.containerSpecChanges),
	}
	s.podSpecGetter.setProvisioningInfo(apicaasunitprovisioner.ProvisioningInfo{
		PodSpec:     containerSpec,
		Tags:        map[string]string{"foo": "bar"},
		Placement:   "placement",
		Constraints: constraints.MustParse("mem=4G"),
		Filesystems: []storage.KubernetesFilesystemParams{{
			StorageName: "database",
			Size:        100,
		}},
	})

	s.unitUpdater = mockUnitUpdater{}

	s.containerBroker = mockContainerBroker{
		unitsWatcher:    watchertest.NewMockNotifyWatcher(s.caasUnitsChanges),
		operatorWatcher: watchertest.NewMockNotifyWatcher(s.caasOperatorChanges),
		podSpec:         &parsedSpec,
	}
	s.lifeGetter = mockLifeGetter{}
	s.lifeGetter.setLife(life.Alive)
	s.serviceBroker = mockServiceBroker{
		ensured: s.serviceEnsured,
		deleted: s.serviceDeleted,
		podSpec: &parsedSpec,
	}
	s.statusSetter = mockProvisioningStatusSetter{}

	s.config = caasunitprovisioner.Config{
		ApplicationGetter:        &s.applicationGetter,
		ApplicationUpdater:       &s.applicationUpdater,
		ServiceBroker:            &s.serviceBroker,
		ContainerBroker:          &s.containerBroker,
		ProvisioningInfoGetter:   &s.podSpecGetter,
		LifeGetter:               &s.lifeGetter,
		UnitUpdater:              &s.unitUpdater,
		ProvisioningStatusSetter: &s.statusSetter,
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
	w, err := caasunitprovisioner.NewWorker(s.config)
	c.Assert(err, jc.ErrorIsNil)
	workertest.CheckAlive(c, w)
	workertest.CleanKill(c, w)
}

func (s *WorkerSuite) setupNewUnitScenario(c *gc.C) worker.Worker {
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
	s.statusSetter.CheckCall(c, 0, "SetOperatorStatus", "gitlab", status.Waiting, "ensuring", map[string]interface{}{"foo": "bar"})
	select {
	case <-s.serviceUpdated:
	case <-time.After(coretesting.LongWait):
		c.Fatal("timed out waiting for service to be updated")
	}
	return w
}

func (s *WorkerSuite) TestScaleChanged(c *gc.C) {
	w := s.setupNewUnitScenario(c)
	defer workertest.CleanKill(c, w)

	s.applicationGetter.CheckCallNames(c, "WatchApplications", "WatchApplicationScale", "ApplicationScale", "ApplicationConfig")
	s.podSpecGetter.CheckCallNames(c, "WatchPodSpec", "ProvisioningInfo", "ProvisioningInfo")
	s.podSpecGetter.CheckCall(c, 0, "WatchPodSpec", "gitlab")
	s.podSpecGetter.CheckCall(c, 1, "ProvisioningInfo", "gitlab") // not found
	s.podSpecGetter.CheckCall(c, 2, "ProvisioningInfo", "gitlab")
	s.lifeGetter.CheckCallNames(c, "Life")
	s.lifeGetter.CheckCall(c, 0, "Life", "gitlab")
	s.serviceBroker.CheckCallNames(c, "EnsureService", "Service")
	s.serviceBroker.CheckCall(c, 0, "EnsureService",
		"gitlab", expectedServiceParams, 1, application.ConfigAttributes{"juju-external-hostname": "exthost"})
	s.serviceBroker.CheckCall(c, 1, "Service", "gitlab")

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

	newExpectedParams := *expectedServiceParams
	newExpectedParams.PodSpec = &parsedSpec
	s.serviceBroker.CheckCallNames(c, "EnsureService")
	s.serviceBroker.CheckCall(c, 0, "EnsureService",
		"gitlab", &newExpectedParams, 2, application.ConfigAttributes{"juju-external-hostname": "exthost"})

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
		"gitlab", &newExpectedParams, 1, application.ConfigAttributes{"juju-external-hostname": "exthost"})
}

func (s *WorkerSuite) TestNewPodSpecChange(c *gc.C) {
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
    image-name: gitlab/latest
`[1:]

		anotherParsedSpec = caas.PodSpec{
			Containers: []caas.ContainerSpec{{
				Name:  "gitlab",
				Image: "gitlab/latest",
			}}}
	)

	s.serviceBroker.podSpec = &anotherParsedSpec

	s.podSpecGetter.setProvisioningInfo(apicaasunitprovisioner.ProvisioningInfo{
		PodSpec: anotherSpec,
		Tags:    map[string]string{"foo": "bar"},
	})
	s.sendContainerSpecChange(c)
	s.podSpecGetter.assertSpecRetrieved(c)

	select {
	case <-s.serviceEnsured:
	case <-time.After(coretesting.LongWait):
		c.Fatal("timed out waiting for service to be ensured")
	}

	expectedParams := &caas.ServiceParams{
		PodSpec:      &anotherParsedSpec,
		ResourceTags: map[string]string{"foo": "bar"},
	}
	s.serviceBroker.CheckCallNames(c, "EnsureService")
	s.serviceBroker.CheckCall(c, 0, "EnsureService",
		"gitlab", expectedParams, 1, application.ConfigAttributes{"juju-external-hostname": "exthost"})
}

func (s *WorkerSuite) TestNewPodSpecChangeCrd(c *gc.C) {
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

	float64Ptr := func(f float64) *float64 { return &f }

	var (
		anotherSpec = `
crd:
  - group: kubeflow.org
    version: v1alpha2
    scope: Namespaced
    kind: TFJob
    validation:
      properties:
        tfReplicaSpecs:
          properties:
            Worker:
              properties:
                replicas:
                  type: integer
                  minimum: 1
`[1:]

		anotherParsedSpec = caas.PodSpec{
			CustomResourceDefinitions: []caas.CustomResourceDefinition{
				{
					Kind:    "TFJob",
					Group:   "kubeflow.org",
					Version: "v1alpha2",
					Scope:   "Namespaced",
					Validation: caas.CustomResourceDefinitionValidation{
						Properties: map[string]apiextensionsv1beta1.JSONSchemaProps{
							"tfReplicaSpecs": {
								Properties: map[string]apiextensionsv1beta1.JSONSchemaProps{
									"Worker": {
										Properties: map[string]apiextensionsv1beta1.JSONSchemaProps{
											"replicas": {
												Type:    "integer",
												Minimum: float64Ptr(1),
											},
										},
									},
								},
							},
						},
					},
				},
			},
		}
	)

	s.serviceBroker.podSpec = &anotherParsedSpec

	s.podSpecGetter.setProvisioningInfo(apicaasunitprovisioner.ProvisioningInfo{
		PodSpec: anotherSpec,
		Tags:    map[string]string{"foo": "bar"},
	})
	s.sendContainerSpecChange(c)
	s.podSpecGetter.assertSpecRetrieved(c)

	select {
	case <-s.serviceEnsured:
	case <-time.After(coretesting.LongWait):
		c.Fatal("timed out waiting for service to be ensured")
	}

	s.serviceBroker.CheckCallNames(c, "EnsureCustomResourceDefinition", "EnsureService")
	s.serviceBroker.CheckCall(c, 0, "EnsureCustomResourceDefinition", "gitlab", &anotherParsedSpec)
}

func (s *WorkerSuite) TestUnitAllRemoved(c *gc.C) {
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
	case s.applicationScaleChanges <- struct{}{}:
		c.Fatal("unexpected watch for application scale")
	case <-time.After(coretesting.ShortWait):
	}

	workertest.CleanKill(c, w)
	// There should just be the initial watch call, no subsequent calls to watch/get scale etc.
	s.applicationGetter.CheckCallNames(c, "WatchApplications")
}

func (s *WorkerSuite) TestRemoveApplicationStopsWatchingApplicationScale(c *gc.C) {
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

func (s *WorkerSuite) TestWatcherErrorStopsWorker(c *gc.C) {
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
		if len(s.containerBroker.Calls()) > 0 {
			break
		}
	}
	s.containerBroker.CheckCallNames(c, "WatchUnits", "WatchOperator")

	s.assertUnitChange(c, status.Allocating, status.Allocating)
	s.assertUnitChange(c, status.Allocating, status.Unknown)
}

func (s *WorkerSuite) TestOperatorChange(c *gc.C) {
	w, err := caasunitprovisioner.NewWorker(s.config)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.DirtyKill(c, w)

	select {
	case s.applicationChanges <- []string{"gitlab"}:
	case <-time.After(coretesting.LongWait):
		c.Fatal("timed out sending applications change")
	}

	for a := coretesting.LongAttempt.Start(); a.Next(); {
		if len(s.containerBroker.Calls()) > 0 {
			break
		}
	}
	s.containerBroker.CheckCallNames(c, "WatchUnits", "WatchOperator")
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

	s.statusSetter.CheckCallNames(c, "SetOperatorStatus")
	c.Assert(s.statusSetter.Calls()[0].Args, jc.DeepEquals, []interface{}{
		"gitlab", status.Active, "testing 1. 2. 3.", map[string]interface{}{"zip": "zap"},
	})
}

func (s *WorkerSuite) assertUnitChange(c *gc.C, reported, expected status.Status) {
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
	c.Assert(s.containerBroker.Calls()[0].Args, jc.DeepEquals, []interface{}{"gitlab"})

	for a := coretesting.LongAttempt.Start(); a.Next(); {
		if len(s.unitUpdater.Calls()) > 0 {
			break
		}
	}
	s.unitUpdater.CheckCallNames(c, "UpdateUnits")
	c.Assert(s.unitUpdater.Calls()[0].Args, jc.DeepEquals, []interface{}{
		params.UpdateApplicationUnits{
			ApplicationTag: names.NewApplicationTag("gitlab").String(),
			Units: []params.ApplicationUnitParams{
				{ProviderId: "u1", Address: "10.0.0.1", Ports: []string(nil), Status: expected.String(),
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
