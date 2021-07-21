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

	facade     *mocks.MockCAASProvisionerFacade
	broker     *mocks.MockCAASBroker
	brokerApp  *caasmocks.MockApplication
	unitFacade *mocks.MockCAASUnitProvisionerFacade

	appCharmInfo        *charmscommon.CharmInfo
	appCharmURL         *charm.URL
	appProvisioningInfo api.ProvisioningInfo
	ociResources        map[string]resources.DockerImageDetails

	appScaleChan     chan struct{}
	notifyReady      chan struct{}
	appStateChan     chan struct{}
	appChan          chan struct{}
	appReplicasChan  chan struct{}
	appTrustHashChan chan []string
}

func (s *ApplicationWorkerSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)

	s.clock = testclock.NewClock(time.Now())
	s.modelTag = names.NewModelTag("ffffffff-ffff-ffff-ffff-ffffffffffff")
	s.logger = loggo.GetLogger("test")
}

func (s *ApplicationWorkerSuite) TearDownTest(c *gc.C) {
	s.BaseSuite.TearDownTest(c)

	close(s.notifyReady)
	s.clock = nil
	s.facade = nil
	s.broker = nil
	s.brokerApp = nil
	s.unitFacade = nil
}

func (s *ApplicationWorkerSuite) getWorker(c *gc.C) (func(...*gomock.Call) worker.Worker, *gomock.Controller) {
	ctrl := gomock.NewController(c)

	s.facade = mocks.NewMockCAASProvisionerFacade(ctrl)
	s.broker = mocks.NewMockCAASBroker(ctrl)
	s.unitFacade = mocks.NewMockCAASUnitProvisionerFacade(ctrl)
	s.brokerApp = caasmocks.NewMockApplication(ctrl)

	s.appCharmURL = &charm.URL{
		Schema:   "cs",
		Name:     "test",
		Revision: -1,
	}
	s.appCharmInfo = &charmscommon.CharmInfo{
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
	s.appProvisioningInfo = api.ProvisioningInfo{
		Series:   "focal",
		CharmURL: s.appCharmURL,
	}
	s.ociResources = map[string]resources.DockerImageDetails{
		"test-oci": {
			RegistryPath: "some/test:img",
		},
	}

	s.appScaleChan = make(chan struct{}, 1)
	s.notifyReady = make(chan struct{}, 1)
	s.appStateChan = make(chan struct{}, 1)
	s.appChan = make(chan struct{}, 1)
	s.appReplicasChan = make(chan struct{}, 1)
	s.appTrustHashChan = make(chan []string, 1)

	startFunc := func(additionalAssertCalls ...*gomock.Call) worker.Worker {
		config := caasapplicationprovisioner.AppWorkerConfig{
			Name:       "test",
			Facade:     s.facade,
			Broker:     s.broker,
			ModelTag:   s.modelTag,
			Clock:      s.clock,
			Logger:     s.logger,
			UnitFacade: s.unitFacade,
		}
		expectedCalls := append([]*gomock.Call{
			// Initialize in loop.
			s.facade.EXPECT().ApplicationCharmURL("test").DoAndReturn(func(string) (*charm.URL, error) {
				return s.appCharmURL, nil
			}),
			s.facade.EXPECT().CharmInfo("cs:test").DoAndReturn(func(string) (*charmscommon.CharmInfo, error) {
				return s.appCharmInfo, nil
			}),
			s.facade.EXPECT().SetPassword("test", gomock.Any()).Return(nil),
			s.broker.EXPECT().Application("test", caas.DeploymentStateful).AnyTimes().Return(s.brokerApp),
			s.unitFacade.EXPECT().WatchApplicationScale("test").Return(watchertest.NewMockNotifyWatcher(s.appScaleChan), nil),
			s.unitFacade.EXPECT().WatchApplicationTrustHash("test").Return(watchertest.NewMockStringsWatcher(s.appTrustHashChan), nil),
		},
			// ROUND 0 - Ensure() for the application.
			s.facade.EXPECT().Life("test").DoAndReturn(func(string) (life.Value, error) {
				return life.Alive, nil
			}),
			s.facade.EXPECT().WatchApplication("test").Return(watchertest.NewMockNotifyWatcher(s.appStateChan), nil),
			s.facade.EXPECT().ProvisioningInfo("test").DoAndReturn(func(string) (api.ProvisioningInfo, error) {
				return s.appProvisioningInfo, nil
			}),
			s.facade.EXPECT().CharmInfo("cs:test").DoAndReturn(func(string) (*charmscommon.CharmInfo, error) {
				return s.appCharmInfo, nil
			}),
			s.brokerApp.EXPECT().Exists().DoAndReturn(func() (caas.DeploymentState, error) {
				return caas.DeploymentState{}, nil
			}),
			s.facade.EXPECT().ApplicationOCIResources("test").DoAndReturn(func(string) (map[string]resources.DockerImageDetails, error) {
				return s.ociResources, nil
			}),
			s.brokerApp.EXPECT().Ensure(gomock.Any()).DoAndReturn(func(config caas.ApplicationConfig) error {
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
			s.brokerApp.EXPECT().Watch().Return(watchertest.NewMockNotifyWatcher(s.appChan), nil),
			s.brokerApp.EXPECT().WatchReplicas().DoAndReturn(func() (watcher.NotifyWatcher, error) {
				return watchertest.NewMockNotifyWatcher(s.appReplicasChan), nil
			}),
			// refresh application status - test seperately.
			s.brokerApp.EXPECT().State().
				DoAndReturn(func() (caas.ApplicationState, error) {
					s.notifyReady <- struct{}{}
					return caas.ApplicationState{}, errors.NotFoundf("")
				}),
		)

		gomock.InOrder(append(expectedCalls, additionalAssertCalls...)...)

		f := caasapplicationprovisioner.NewAppWorker(config)
		c.Assert(f, gc.NotNil)
		appWorker, err := f()
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(appWorker, gc.NotNil)
		s.AddCleanup(func(c *gc.C) { workertest.CleanKill(c, appWorker) })
		return appWorker
	}
	return startFunc, ctrl
}

var unitsAPIResult = []params.CAASUnit{
	{
		Tag: names.NewUnitTag("test/0"),
		UnitStatus: &params.UnitStatus{
			AgentStatus: params.DetailedStatus{Status: "active"},
		},
	},
}

func (s *ApplicationWorkerSuite) TestWorker(c *gc.C) {
	newAppWorker, ctrl := s.getWorker(c)
	defer ctrl.Finish()

	done := make(chan struct{})

	assertionCalls := []*gomock.Call{
		// Got replicaChanges -> updateState().
		s.facade.EXPECT().Units("test").DoAndReturn(func(string) ([]params.CAASUnit, error) {
			return unitsAPIResult, nil
		}),
		s.brokerApp.EXPECT().State().DoAndReturn(func() (caas.ApplicationState, error) {
			return caas.ApplicationState{
				DesiredReplicas: 1,
				Replicas:        []string{"test-0"},
			}, nil
		}),
		s.brokerApp.EXPECT().Service().DoAndReturn(func() (*caas.Service, error) {
			return &caas.Service{
				Id:        "deadbeef",
				Addresses: network.NewProviderAddresses("10.6.6.6"),
			}, nil
		}),
		s.unitFacade.EXPECT().UpdateApplicationService(params.UpdateApplicationServiceArg{
			ApplicationTag: "application-test",
			ProviderId:     "deadbeef",
			Addresses:      params.FromProviderAddresses(network.NewProviderAddress("10.6.6.6")),
		}).Return(nil),
		s.facade.EXPECT().GarbageCollect("test", []names.Tag{names.NewUnitTag("test/0")}, 1, []string{"test-0"}, false).
			DoAndReturn(
				func(_ string, _ []names.Tag, _ int, _ []string, _ bool) error { return nil },
			),
		s.brokerApp.EXPECT().Units().Return([]caas.Unit{{
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
		s.facade.EXPECT().UpdateUnits(params.UpdateApplicationUnits{
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
			return nil, nil
		}),

		// refresh application status - test seperately.
		s.brokerApp.EXPECT().State().
			DoAndReturn(func() (caas.ApplicationState, error) {
				s.notifyReady <- struct{}{}
				return caas.ApplicationState{}, errors.NotFoundf("")
			}),

		// Second run - Ensure() for the application.
		// Should not Ensure since unchanged.
		s.facade.EXPECT().Life("test").DoAndReturn(func(string) (life.Value, error) {
			return life.Alive, nil
		}),
		s.facade.EXPECT().ProvisioningInfo("test").DoAndReturn(func(string) (api.ProvisioningInfo, error) {
			return s.appProvisioningInfo, nil
		}),
		s.facade.EXPECT().CharmInfo("cs:test").DoAndReturn(func(string) (*charmscommon.CharmInfo, error) {
			return s.appCharmInfo, nil
		}),
		s.brokerApp.EXPECT().Exists().DoAndReturn(func() (caas.DeploymentState, error) {
			return caas.DeploymentState{}, nil
		}),
		s.facade.EXPECT().ApplicationOCIResources("test").DoAndReturn(func(string) (map[string]resources.DockerImageDetails, error) {
			return s.ociResources, nil
		}),

		// refresh application status - test seperately.
		s.brokerApp.EXPECT().State().
			DoAndReturn(func() (caas.ApplicationState, error) {
				s.notifyReady <- struct{}{}
				return caas.ApplicationState{}, errors.NotFoundf("")
			}),

		// Got appChanges -> updateState().
		s.facade.EXPECT().Units("test").DoAndReturn(func(string) ([]params.CAASUnit, error) {
			return unitsAPIResult, nil
		}),
		s.brokerApp.EXPECT().State().DoAndReturn(func() (caas.ApplicationState, error) {
			return caas.ApplicationState{
				DesiredReplicas: 0,
				Replicas:        []string(nil),
			}, nil
		}),
		s.brokerApp.EXPECT().Service().DoAndReturn(func() (*caas.Service, error) {
			return &caas.Service{
				Id:        "deadbeef",
				Addresses: network.NewProviderAddresses("10.6.6.6"),
			}, nil
		}),
		s.unitFacade.EXPECT().UpdateApplicationService(params.UpdateApplicationServiceArg{
			ApplicationTag: "application-test",
			ProviderId:     "deadbeef",
			Addresses:      params.FromProviderAddresses(network.NewProviderAddress("10.6.6.6")),
		}).Return(nil),
		s.facade.EXPECT().GarbageCollect("test", []names.Tag{names.NewUnitTag("test/0")}, 0, []string(nil), false).DoAndReturn(func(appName string, observedUnits []names.Tag, desiredReplicas int, activePodNames []string, force bool) error {
			return nil
		}),

		s.brokerApp.EXPECT().Units().Return([]caas.Unit{{
			Id:    "test-0",
			Dying: true,
			Status: status.StatusInfo{
				Status: status.Terminated,
			},
		}}, nil),
		s.facade.EXPECT().UpdateUnits(params.UpdateApplicationUnits{
			ApplicationTag: "application-test",
			Status:         params.EntityStatus{},
		}).Return(nil, nil),

		// refresh application status - test seperately.
		s.brokerApp.EXPECT().State().
			DoAndReturn(func() (caas.ApplicationState, error) {
				s.notifyReady <- struct{}{}
				return caas.ApplicationState{}, errors.NotFoundf("")
			}),

		// 1st Notify() - dying.
		s.facade.EXPECT().Life("test").DoAndReturn(func(string) (life.Value, error) {
			return life.Dying, nil
		}),
		s.brokerApp.EXPECT().Delete().DoAndReturn(func() error {
			s.notifyReady <- struct{}{}
			return nil
		}),

		// 2nd Notify() - dead.
		s.facade.EXPECT().Life("test").DoAndReturn(func(string) (life.Value, error) {
			return life.Dead, nil
		}),
		s.brokerApp.EXPECT().Delete().DoAndReturn(func() error {
			return nil
		}),
		s.brokerApp.EXPECT().Exists().DoAndReturn(func() (caas.DeploymentState, error) {
			return caas.DeploymentState{
				Exists:      false,
				Terminating: false,
			}, nil
		}),
		s.facade.EXPECT().Units("test").DoAndReturn(func(string) ([]params.CAASUnit, error) {
			return []params.CAASUnit(nil), nil
		}),
		s.brokerApp.EXPECT().State().DoAndReturn(func() (caas.ApplicationState, error) {
			return caas.ApplicationState{
				DesiredReplicas: 0,
				Replicas:        []string(nil),
			}, nil
		}),
		s.brokerApp.EXPECT().Service().DoAndReturn(func() (*caas.Service, error) {
			return nil, errors.NotFoundf("test")
		}),
		s.facade.EXPECT().GarbageCollect("test", []names.Tag(nil), 0, []string(nil), true).DoAndReturn(func(appName string, observedUnits []names.Tag, desiredReplicas int, activePodNames []string, force bool) error {
			close(done)
			return nil
		}),
	}

	appWorker := newAppWorker(assertionCalls...)

	go func(w appNotifyWorker) {
		steps := []func(){
			// Test replica changes.
			func() { s.appReplicasChan <- struct{}{} },
			// Test app state changes.
			func() { s.appStateChan <- struct{}{} },
			// Test app changes from cloud.
			func() { s.appChan <- struct{}{} },
			// Test Notify - dying.
			w.Notify,
			// Test Notify - dead.
			w.Notify,
		}
		for _, step := range steps {
			<-s.notifyReady
			step()
		}
	}(appWorker.(appNotifyWorker))

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
