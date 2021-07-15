// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasapplicationprovisioner_test

import (
	"time"

	"github.com/golang/mock/gomock"
	"github.com/juju/charm/v8"
	charmresource "github.com/juju/charm/v8/resource"
	"github.com/juju/clock/testclock"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names/v4"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v2"
	"github.com/juju/worker/v2/workertest"
	gc "gopkg.in/check.v1"

	api "github.com/juju/juju/api/caasapplicationprovisioner"
	charmscommon "github.com/juju/juju/api/common/charms"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/caas"
	caasmocks "github.com/juju/juju/caas/mocks"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/resources"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/watchertest"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker/caasapplicationprovisioner"
	"github.com/juju/juju/worker/caasapplicationprovisioner/mocks"
)

var _ = gc.Suite(&ApplicationWorkerSuite{})

type ApplicationWorkerSuite struct {
	coretesting.BaseSuite

	clock    *testclock.Clock
	modelTag names.ModelTag
	logger   loggo.Logger
}

func (s *ApplicationWorkerSuite) SetUpTest(c *gc.C) {
	s.clock = testclock.NewClock(time.Now())
	s.modelTag = names.NewModelTag("ffffffff-ffff-ffff-ffff-ffffffffffff")
	s.logger = loggo.GetLogger("test")
}

func (s *ApplicationWorkerSuite) TestWorker(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	charmInfo := &charmscommon.CharmInfo{
		Meta: &charm.Meta{
			Name: "test",

			Containers: map[string]charm.Container{
				"test": {
					Resource: "test-oci",
				},
			},
			Resources: map[string]charmresource.Meta{
				"test-oci": {
					Type: charmresource.TypeContainerImage,
				},
			},
		},
		Manifest: &charm.Manifest{
			Bases: []charm.Base{{
				Name: "ubuntu",
				Channel: charm.Channel{
					Track: "20.04",
					Risk:  "stable",
				},
			}},
		},
	}
	appProvisioningInfo := api.ProvisioningInfo{
		Series: "focal",
		CharmURL: &charm.URL{
			Schema:   "cs",
			Name:     "test",
			Revision: -1,
		},
	}
	ociResources := map[string]resources.DockerImageDetails{
		"test-oci": {
			RegistryPath: "some/test:img",
		},
	}

	notifyReady := make(chan struct{}, 1)

	appStateChan := make(chan struct{}, 1)
	appStateWatcher := watchertest.NewMockNotifyWatcher(appStateChan)

	appChan := make(chan struct{}, 1)
	appWatcher := watchertest.NewMockNotifyWatcher(appChan)

	appReplicasChan := make(chan struct{}, 1)
	appReplicasWatcher := watchertest.NewMockNotifyWatcher(appReplicasChan)

	appScaleChan := make(chan struct{}, 1)
	appScaleWatcher := watchertest.NewMockNotifyWatcher(appScaleChan)

	appTrustHashChan := make(chan []string, 1)
	appTrushHashWatcher := watchertest.NewMockStringsWatcher(appTrustHashChan)

	brokerApp := caasmocks.NewMockApplication(ctrl)
	broker := mocks.NewMockCAASBroker(ctrl)
	facade := mocks.NewMockCAASProvisionerFacade(ctrl)
	unitFacade := mocks.NewMockCAASUnitProvisionerFacade(ctrl)

	done := make(chan struct{})
	gomock.InOrder(
		// Verify charm is v2
		facade.EXPECT().WatchApplication("test").Return(appStateWatcher, nil),
		facade.EXPECT().ApplicationCharmInfo("test").Return(charmInfo, nil),

		// Operator delete loop
		broker.EXPECT().OperatorExists("test").Return(caas.DeploymentState{Exists: false}, nil),

		// Pre-loop setup
		facade.EXPECT().SetPassword("test", gomock.Any()).Return(nil),
		broker.EXPECT().Application("test", caas.DeploymentStateful).Return(brokerApp),
		unitFacade.EXPECT().WatchApplicationScale("test").Return(appScaleWatcher, nil),
		unitFacade.EXPECT().WatchApplicationTrustHash("test").Return(appTrushHashWatcher, nil),

		// Initial run - Ensure() for the application.
		facade.EXPECT().Life("test").Return(life.Alive, nil),
		facade.EXPECT().WatchApplication("test").Return(appStateWatcher, nil),
		facade.EXPECT().ProvisioningInfo("test").Return(appProvisioningInfo, nil),
		facade.EXPECT().CharmInfo("cs:test").Return(charmInfo, nil),
		brokerApp.EXPECT().Exists().Return(caas.DeploymentState{}, nil),
		facade.EXPECT().ApplicationOCIResources("test").Return(ociResources, nil),
		brokerApp.EXPECT().Ensure(gomock.Any()).DoAndReturn(func(config caas.ApplicationConfig) error {
			mc := jc.NewMultiChecker()
			mc.AddExpr(`_.IntroductionSecret`, gc.HasLen, 24)
			mc.AddExpr(`_.Charm`, gc.NotNil)
			c.Check(config, mc, caas.ApplicationConfig{
				CharmBaseImage: resources.DockerImageDetails{
					RegistryPath: "jujusolutions/charm-base:ubuntu-20.04",
				},
				Containers: map[string]caas.ContainerConfig{
					"test": {
						Name: "test",
						Image: resources.DockerImageDetails{
							RegistryPath: "some/test:img",
						},
					},
				},
			})
			return nil
		}),
		facade.EXPECT().SetOperatorStatus("test", status.Active, "deployed", nil).Return(nil),
		brokerApp.EXPECT().Watch().Return(appWatcher, nil),
		brokerApp.EXPECT().WatchReplicas().DoAndReturn(func() (watcher.NotifyWatcher, error) {
			appReplicasChan <- struct{}{}
			return appReplicasWatcher, nil
		}),

		// Got replicaChanges -> updateState().
		facade.EXPECT().Units("test").DoAndReturn(func(string) ([]names.Tag, error) {
			return []names.Tag{
				names.NewUnitTag("test/0"),
			}, nil
		}),
		brokerApp.EXPECT().State().DoAndReturn(func() (caas.ApplicationState, error) {
			return caas.ApplicationState{
				DesiredReplicas: 1,
				Replicas:        []string{"test-0"},
			}, nil
		}),
		brokerApp.EXPECT().Service().DoAndReturn(func() (*caas.Service, error) {
			return &caas.Service{
				Id:        "deadbeef",
				Addresses: network.NewProviderAddresses("10.6.6.6"),
			}, nil
		}),
		unitFacade.EXPECT().UpdateApplicationService(params.UpdateApplicationServiceArg{
			ApplicationTag: "application-test",
			ProviderId:     "deadbeef",
			Addresses:      params.FromProviderAddresses(network.NewProviderAddress("10.6.6.6")),
		}).Return(nil),
		facade.EXPECT().GarbageCollect("test", []names.Tag{names.NewUnitTag("test/0")}, 1, []string{"test-0"}, false).DoAndReturn(func(appName string, observedUnits []names.Tag, desiredReplicas int, activePodNames []string, force bool) error {
			return nil
		}),
		brokerApp.EXPECT().Units().Return([]caas.Unit{{
			Id:      "test-0",
			Address: "10.10.10.1",
			Dying:   false,
			Status: status.StatusInfo{
				Status: status.Active,
			},
			FilesystemInfo: []caas.FilesystemInfo{{
				StorageName:  "database",
				FilesystemId: "db-0",
				Size:         1024,
				MountPoint:   "/mnt/test",
				ReadOnly:     false,
				Status: status.StatusInfo{
					Status:  status.Active,
					Message: "bound",
				},
				Volume: caas.VolumeInfo{
					Persistent: true,
					Size:       1024,
					VolumeId:   "pv-1234",
					Status: status.StatusInfo{
						Status:  status.Active,
						Message: "bound",
					},
				},
			}},
		}}, nil),
		facade.EXPECT().UpdateUnits(params.UpdateApplicationUnits{
			ApplicationTag: "application-test",
			Status:         params.EntityStatus{},
			Units: []params.ApplicationUnitParams{{
				ProviderId: "test-0",
				Address:    "10.10.10.1",
				Status:     "active",
				FilesystemInfo: []params.KubernetesFilesystemInfo{{
					StorageName:  "database",
					FilesystemId: "db-0",
					Size:         1024,
					MountPoint:   "/mnt/test",
					ReadOnly:     false,
					Status:       "active",
					Info:         "bound",
					Volume: params.KubernetesVolumeInfo{
						Persistent: true,
						Size:       1024,
						VolumeId:   "pv-1234",
						Status:     "active",
						Info:       "bound",
					},
				}},
			}},
		}).DoAndReturn(func(_ params.UpdateApplicationUnits) (*params.UpdateApplicationUnitsInfo, error) {
			appStateChan <- struct{}{}
			return nil, nil
		}),

		// Second run - Ensure() for the application.
		facade.EXPECT().Life("test").DoAndReturn(func(string) (life.Value, error) {
			return life.Alive, nil
		}),
		facade.EXPECT().ProvisioningInfo("test").DoAndReturn(func(string) (api.ProvisioningInfo, error) {
			return appProvisioningInfo, nil
		}),
		facade.EXPECT().CharmInfo("cs:test").DoAndReturn(func(string) (*charmscommon.CharmInfo, error) {
			return charmInfo, nil
		}),
		brokerApp.EXPECT().Exists().DoAndReturn(func() (caas.DeploymentState, error) {
			return caas.DeploymentState{}, nil
		}),
		facade.EXPECT().ApplicationOCIResources("test").DoAndReturn(func(string) (map[string]resources.DockerImageDetails, error) {
			appChan <- struct{}{}
			return ociResources, nil
		}),
		// Second run should not Ensure since unchanged.
		facade.EXPECT().SetOperatorStatus("test", status.Active, "unchanged", nil).Return(nil),

		// Got appChanges -> updateState().
		facade.EXPECT().Units("test").DoAndReturn(func(string) ([]names.Tag, error) {
			return []names.Tag{
				names.NewUnitTag("test/0"),
			}, nil
		}),
		brokerApp.EXPECT().State().DoAndReturn(func() (caas.ApplicationState, error) {
			return caas.ApplicationState{
				DesiredReplicas: 0,
				Replicas:        []string(nil),
			}, nil
		}),
		brokerApp.EXPECT().Service().DoAndReturn(func() (*caas.Service, error) {
			return &caas.Service{
				Id:        "deadbeef",
				Addresses: network.NewProviderAddresses("10.6.6.6"),
			}, nil
		}),
		unitFacade.EXPECT().UpdateApplicationService(params.UpdateApplicationServiceArg{
			ApplicationTag: "application-test",
			ProviderId:     "deadbeef",
			Addresses:      params.FromProviderAddresses(network.NewProviderAddress("10.6.6.6")),
		}).Return(nil),
		facade.EXPECT().GarbageCollect("test", []names.Tag{names.NewUnitTag("test/0")}, 0, []string(nil), false).DoAndReturn(func(appName string, observedUnits []names.Tag, desiredReplicas int, activePodNames []string, force bool) error {
			notifyReady <- struct{}{}
			return nil
		}),

		brokerApp.EXPECT().Units().Return([]caas.Unit{{
			Id:    "test-0",
			Dying: true,
			Status: status.StatusInfo{
				Status: status.Terminated,
			},
		}}, nil),
		facade.EXPECT().UpdateUnits(params.UpdateApplicationUnits{
			ApplicationTag: "application-test",
			Status:         params.EntityStatus{},
		}).Return(nil, nil),

		// 1st Notify() - dying.
		facade.EXPECT().Life("test").DoAndReturn(func(string) (life.Value, error) {
			return life.Dying, nil
		}),
		brokerApp.EXPECT().Delete().DoAndReturn(func() error {
			notifyReady <- struct{}{}
			return nil
		}),

		// 2nd Notify() - dead.
		facade.EXPECT().Life("test").DoAndReturn(func(string) (life.Value, error) {
			return life.Dead, nil
		}),
		brokerApp.EXPECT().Delete().DoAndReturn(func() error {
			return nil
		}),
		brokerApp.EXPECT().Exists().DoAndReturn(func() (caas.DeploymentState, error) {
			return caas.DeploymentState{
				Exists:      false,
				Terminating: false,
			}, nil
		}),
		facade.EXPECT().Units("test").DoAndReturn(func(string) ([]names.Tag, error) {
			return []names.Tag(nil), nil
		}),
		brokerApp.EXPECT().State().DoAndReturn(func() (caas.ApplicationState, error) {
			return caas.ApplicationState{
				DesiredReplicas: 0,
				Replicas:        []string(nil),
			}, nil
		}),
		brokerApp.EXPECT().Service().DoAndReturn(func() (*caas.Service, error) {
			return nil, errors.NotFoundf("test")
		}),
		facade.EXPECT().GarbageCollect("test", []names.Tag(nil), 0, []string(nil), true).DoAndReturn(func(appName string, observedUnits []names.Tag, desiredReplicas int, activePodNames []string, force bool) error {
			close(done)
			close(notifyReady)
			return nil
		}),
	)

	appWorker := s.startAppWorker(c, facade, broker, unitFacade)

	go func(w appNotifyWorker) {
		for {
			select {
			case _, ok := <-notifyReady:
				if !ok {
					return
				}
				w.Notify()
			}
		}
	}(appWorker.(appNotifyWorker))

	s.waitDone(c, done)
	workertest.CleanKill(c, appWorker)
}

func (s *ApplicationWorkerSuite) startAppWorker(
	c *gc.C,
	facade caasapplicationprovisioner.CAASProvisionerFacade,
	broker caasapplicationprovisioner.CAASBroker,
	unitFacade caasapplicationprovisioner.CAASUnitProvisionerFacade,
) worker.Worker {
	config := caasapplicationprovisioner.AppWorkerConfig{
		Name:       "test",
		Facade:     facade,
		Broker:     broker,
		ModelTag:   s.modelTag,
		Clock:      s.clock,
		Logger:     s.logger,
		UnitFacade: unitFacade,
	}
	startFunc := caasapplicationprovisioner.NewAppWorker(config)
	c.Assert(startFunc, gc.NotNil)
	appWorker, err := startFunc()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(appWorker, gc.NotNil)
	return appWorker
}

func (s *ApplicationWorkerSuite) waitDone(c *gc.C, done chan struct{}) {
	select {
	case <-done:
	case <-time.After(coretesting.ShortWait):
		c.Errorf("timed out waiting for worker")
	}
}

type appNotifyWorker interface {
	worker.Worker
	Notify()
}

func (s *ApplicationWorkerSuite) TestUpgrade(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	charmInfoV1 := &charmscommon.CharmInfo{
		Meta: &charm.Meta{Name: "test"},
	}
	charmInfoV2 := &charmscommon.CharmInfo{
		Meta:     &charm.Meta{Name: "test"},
		Manifest: &charm.Manifest{Bases: []charm.Base{{}}},
	}

	appStateChan := make(chan struct{}, 1)
	appStateWatcher := watchertest.NewMockNotifyWatcher(appStateChan)
	broker := mocks.NewMockCAASBroker(ctrl)
	facade := mocks.NewMockCAASProvisionerFacade(ctrl)
	done := make(chan struct{})

	gomock.InOrder(
		// Wait till charm is v2
		facade.EXPECT().WatchApplication("test").Return(appStateWatcher, nil),
		facade.EXPECT().ApplicationCharmInfo("test").Return(charmInfoV1, nil),
		facade.EXPECT().Life("test").DoAndReturn(func(appName string) (life.Value, error) {
			appStateChan <- struct{}{}
			return life.Alive, nil
		}),
		facade.EXPECT().ApplicationCharmInfo("test").Return(charmInfoV2, nil),

		// Operator delete loop
		broker.EXPECT().OperatorExists("test").Return(caas.DeploymentState{Exists: false}, nil),

		// Make SetPassword return an error to exit early (we've tested what
		// we want to above).
		facade.EXPECT().SetPassword("test", gomock.Any()).DoAndReturn(func(appName, password string) error {
			done <- struct{}{}
			return errors.New("exit early error")
		}),
	)

	appWorker := s.startAppWorker(c, facade, broker, nil)

	s.waitDone(c, done)
	workertest.DirtyKill(c, appWorker)
}

func (s *ApplicationWorkerSuite) TestUpgradeInfoNotFound(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	appStateChan := make(chan struct{}, 1)
	appStateWatcher := watchertest.NewMockNotifyWatcher(appStateChan)
	broker := mocks.NewMockCAASBroker(ctrl)
	facade := mocks.NewMockCAASProvisionerFacade(ctrl)
	done := make(chan struct{})

	gomock.InOrder(
		// Wait till charm is v2
		facade.EXPECT().WatchApplication("test").Return(appStateWatcher, nil),
		facade.EXPECT().ApplicationCharmInfo("test").DoAndReturn(func(appName string) (*charmscommon.CharmInfo, error) {
			done <- struct{}{}
			return nil, errors.NotFoundf("test charm")
		}),
	)

	appWorker := s.startAppWorker(c, facade, broker, nil)

	s.waitDone(c, done)
	workertest.CleanKill(c, appWorker)
}

func (s *ApplicationWorkerSuite) TestUpgradeLifeNotFound(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	charmInfoV1 := &charmscommon.CharmInfo{
		Meta: &charm.Meta{Name: "test"},
	}

	appStateChan := make(chan struct{}, 1)
	appStateWatcher := watchertest.NewMockNotifyWatcher(appStateChan)
	broker := mocks.NewMockCAASBroker(ctrl)
	facade := mocks.NewMockCAASProvisionerFacade(ctrl)
	done := make(chan struct{})

	gomock.InOrder(
		// Wait till charm is v2
		facade.EXPECT().WatchApplication("test").Return(appStateWatcher, nil),
		facade.EXPECT().ApplicationCharmInfo("test").Return(charmInfoV1, nil),
		facade.EXPECT().Life("test").DoAndReturn(func(appName string) (life.Value, error) {
			done <- struct{}{}
			return "", errors.NotFoundf("test charm")
		}),
	)

	appWorker := s.startAppWorker(c, facade, broker, nil)

	s.waitDone(c, done)
	workertest.CleanKill(c, appWorker)
}

func (s *ApplicationWorkerSuite) TestUpgradeLifeDead(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	charmInfoV1 := &charmscommon.CharmInfo{
		Meta: &charm.Meta{Name: "test"},
	}

	appStateChan := make(chan struct{}, 1)
	appStateWatcher := watchertest.NewMockNotifyWatcher(appStateChan)
	broker := mocks.NewMockCAASBroker(ctrl)
	facade := mocks.NewMockCAASProvisionerFacade(ctrl)
	done := make(chan struct{})

	gomock.InOrder(
		// Wait till charm is v2
		facade.EXPECT().WatchApplication("test").Return(appStateWatcher, nil),
		facade.EXPECT().ApplicationCharmInfo("test").Return(charmInfoV1, nil),
		facade.EXPECT().Life("test").DoAndReturn(func(appName string) (life.Value, error) {
			done <- struct{}{}
			return life.Dead, nil
		}),
	)

	appWorker := s.startAppWorker(c, facade, broker, nil)

	s.waitDone(c, done)
	workertest.CleanKill(c, appWorker)
}

func (s *ApplicationWorkerSuite) TestDeleteOperator(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	charmInfo := &charmscommon.CharmInfo{
		Meta:     &charm.Meta{Name: "test"},
		Manifest: &charm.Manifest{Bases: []charm.Base{{}}},
	}
	units := []names.Tag{
		names.NewUnitTag("test/0"),
		names.NewUnitTag("test/1"),
	}

	appStateChan := make(chan struct{}, 1)
	appStateWatcher := watchertest.NewMockNotifyWatcher(appStateChan)
	broker := mocks.NewMockCAASBroker(ctrl)
	facade := mocks.NewMockCAASProvisionerFacade(ctrl)
	done := make(chan struct{})

	gomock.InOrder(
		// Verify charm is v2
		facade.EXPECT().WatchApplication("test").Return(appStateWatcher, nil),
		facade.EXPECT().ApplicationCharmInfo("test").Return(charmInfo, nil),

		// Operator delete loop (with a retry)
		broker.EXPECT().OperatorExists("test").Return(caas.DeploymentState{Exists: true}, nil),
		broker.EXPECT().DeleteOperator("test").Return(nil),
		broker.EXPECT().DeleteService("test").DoAndReturn(func(appName string) error {
			// TODO(benhoyt)
			//s.clock.WaitAdvance(time.Second, time.Second, 1)
			return nil
		}),
		broker.EXPECT().OperatorExists("test").Return(caas.DeploymentState{Exists: false}, nil),

		// Clean up units
		facade.EXPECT().Units("test").Return(units, nil),
		facade.EXPECT().GarbageCollect("test", units, 0, nil, true).Return(nil),

		// Make SetPassword return an error to exit early (we've tested what
		// we want to above).
		facade.EXPECT().SetPassword("test", gomock.Any()).DoAndReturn(func(appName, password string) error {
			done <- struct{}{}
			return errors.New("exit early error")
		}),
	)

	appWorker := s.startAppWorker(c, facade, broker, nil)

	s.waitDone(c, done)
	workertest.DirtyKill(c, appWorker)
}
