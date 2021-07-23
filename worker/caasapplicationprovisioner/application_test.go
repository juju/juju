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

	modelTag names.ModelTag
	logger   loggo.Logger

	appCharmInfo        *charmscommon.CharmInfo
	appCharmURL         *charm.URL
	appProvisioningInfo api.ProvisioningInfo
	ociResources        map[string]resources.DockerImageDetails
}

func (s *ApplicationWorkerSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)

	s.modelTag = names.NewModelTag("ffffffff-ffff-ffff-ffff-ffffffffffff")
	s.logger = loggo.GetLogger("test")
}

type testCase struct {
	clock      *testclock.Clock
	facade     *mocks.MockCAASProvisionerFacade
	broker     *mocks.MockCAASBroker
	brokerApp  *caasmocks.MockApplication
	unitFacade *mocks.MockCAASUnitProvisionerFacade

	appScaleChan     chan struct{}
	notifyReady      chan struct{}
	appStateChan     chan struct{}
	appChan          chan struct{}
	appReplicasChan  chan struct{}
	appTrustHashChan chan []string
}

func (s *ApplicationWorkerSuite) getWorker(c *gc.C) (func(...*gomock.Call) worker.Worker, testCase, *gomock.Controller) {
	ctrl := gomock.NewController(c)

	tc := testCase{}

	tc.clock = testclock.NewClock(time.Time{})
	tc.facade = mocks.NewMockCAASProvisionerFacade(ctrl)
	tc.broker = mocks.NewMockCAASBroker(ctrl)
	tc.unitFacade = mocks.NewMockCAASUnitProvisionerFacade(ctrl)
	tc.brokerApp = caasmocks.NewMockApplication(ctrl)

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

	tc.appScaleChan = make(chan struct{}, 1)
	tc.notifyReady = make(chan struct{}, 1)
	tc.appStateChan = make(chan struct{}, 1)
	tc.appChan = make(chan struct{}, 1)
	tc.appReplicasChan = make(chan struct{}, 1)
	tc.appTrustHashChan = make(chan []string, 1)

	startFunc := func(additionalAssertCalls ...*gomock.Call) worker.Worker {
		config := caasapplicationprovisioner.AppWorkerConfig{
			Name:       "test",
			Facade:     tc.facade,
			Broker:     tc.broker,
			ModelTag:   s.modelTag,
			Clock:      tc.clock,
			Logger:     s.logger,
			UnitFacade: tc.unitFacade,
		}
		expectedCalls := append([]*gomock.Call{
			// Initialize in loop.
			tc.facade.EXPECT().ApplicationCharmURL("test").DoAndReturn(func(string) (*charm.URL, error) {
				return s.appCharmURL, nil
			}),
			tc.facade.EXPECT().CharmInfo("cs:test").DoAndReturn(func(string) (*charmscommon.CharmInfo, error) {
				return s.appCharmInfo, nil
			}),
			tc.facade.EXPECT().SetPassword("test", gomock.Any()).Return(nil),
			tc.broker.EXPECT().Application("test", caas.DeploymentStateful).AnyTimes().Return(tc.brokerApp),
			tc.unitFacade.EXPECT().WatchApplicationScale("test").Return(watchertest.NewMockNotifyWatcher(tc.appScaleChan), nil),
			tc.unitFacade.EXPECT().WatchApplicationTrustHash("test").Return(watchertest.NewMockStringsWatcher(tc.appTrustHashChan), nil),
		},
			// ROUND 0 - Ensure() for the application.
			tc.facade.EXPECT().Life("test").DoAndReturn(func(string) (life.Value, error) {
				return life.Alive, nil
			}),
			tc.facade.EXPECT().WatchApplication("test").Return(watchertest.NewMockNotifyWatcher(tc.appStateChan), nil),
			tc.facade.EXPECT().ProvisioningInfo("test").DoAndReturn(func(string) (api.ProvisioningInfo, error) {
				return s.appProvisioningInfo, nil
			}),
			tc.facade.EXPECT().CharmInfo("cs:test").DoAndReturn(func(string) (*charmscommon.CharmInfo, error) {
				return s.appCharmInfo, nil
			}),
			tc.brokerApp.EXPECT().Exists().DoAndReturn(func() (caas.DeploymentState, error) {
				return caas.DeploymentState{}, nil
			}),
			tc.facade.EXPECT().ApplicationOCIResources("test").DoAndReturn(func(string) (map[string]resources.DockerImageDetails, error) {
				return s.ociResources, nil
			}),
			tc.brokerApp.EXPECT().Ensure(gomock.Any()).DoAndReturn(func(config caas.ApplicationConfig) error {
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
			tc.brokerApp.EXPECT().Watch().Return(watchertest.NewMockNotifyWatcher(tc.appChan), nil),
			tc.brokerApp.EXPECT().WatchReplicas().DoAndReturn(func() (watcher.NotifyWatcher, error) {
				return watchertest.NewMockNotifyWatcher(tc.appReplicasChan), nil
			}),
			// refresh application status - test separately.
			tc.brokerApp.EXPECT().State().
				DoAndReturn(func() (caas.ApplicationState, error) {
					tc.notifyReady <- struct{}{}
					return caas.ApplicationState{}, errors.NotFoundf("")
				}),
		)

		gomock.InOrder(append(expectedCalls, additionalAssertCalls...)...)

		f := caasapplicationprovisioner.NewAppWorker(config)
		c.Assert(f, gc.NotNil)
		appWorker, err := f()
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(appWorker, gc.NotNil)
		s.AddCleanup(func(c *gc.C) {
			close(tc.notifyReady)
			workertest.CleanKill(c, appWorker)
		})
		return appWorker
	}
	return startFunc, tc, ctrl
}

var unitsAPIResultSingleActive = []params.CAASUnit{
	{
		Tag: names.NewUnitTag("test/0"),
		UnitStatus: &params.UnitStatus{
			AgentStatus: params.DetailedStatus{Status: "active"},
		},
	},
}

var unitsAPIResultPartialActive = []params.CAASUnit{
	{
		Tag: names.NewUnitTag("test/0"),
		UnitStatus: &params.UnitStatus{
			AgentStatus: params.DetailedStatus{Status: "active"},
		},
	},
	{
		Tag: names.NewUnitTag("test/1"),
		UnitStatus: &params.UnitStatus{
			AgentStatus: params.DetailedStatus{Status: "waiting"},
		},
	},
	{
		Tag: names.NewUnitTag("test/2"),
		UnitStatus: &params.UnitStatus{
			AgentStatus: params.DetailedStatus{Status: "waiting"},
		},
	},
}

func (s *ApplicationWorkerSuite) TestWorker(c *gc.C) {
	newAppWorker, tc, ctrl := s.getWorker(c)
	defer ctrl.Finish()

	done := make(chan struct{})

	assertionCalls := []*gomock.Call{
		// Got replicaChanges -> updateState().
		tc.facade.EXPECT().Units("test").DoAndReturn(func(string) ([]params.CAASUnit, error) {
			return unitsAPIResultSingleActive, nil
		}),
		tc.brokerApp.EXPECT().State().DoAndReturn(func() (caas.ApplicationState, error) {
			return caas.ApplicationState{
				DesiredReplicas: 1,
				Replicas:        []string{"test-0"},
			}, nil
		}),
		tc.brokerApp.EXPECT().Service().DoAndReturn(func() (*caas.Service, error) {
			return &caas.Service{
				Id:        "deadbeef",
				Addresses: network.NewProviderAddresses("10.6.6.6"),
			}, nil
		}),
		tc.unitFacade.EXPECT().UpdateApplicationService(params.UpdateApplicationServiceArg{
			ApplicationTag: "application-test",
			ProviderId:     "deadbeef",
			Addresses:      params.FromProviderAddresses(network.NewProviderAddress("10.6.6.6")),
		}).Return(nil),
		tc.facade.EXPECT().GarbageCollect("test", []names.Tag{names.NewUnitTag("test/0")}, 1, []string{"test-0"}, false).
			DoAndReturn(
				func(_ string, _ []names.Tag, _ int, _ []string, _ bool) error { return nil },
			),
		tc.brokerApp.EXPECT().Units().Return([]caas.Unit{{
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
		tc.facade.EXPECT().UpdateUnits(params.UpdateApplicationUnits{
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

		// refresh application status - test separately.
		tc.brokerApp.EXPECT().State().
			DoAndReturn(func() (caas.ApplicationState, error) {
				tc.notifyReady <- struct{}{}
				return caas.ApplicationState{}, errors.NotFoundf("")
			}),

		// Second run - Ensure() for the application.
		// Should not Ensure since unchanged.
		tc.facade.EXPECT().Life("test").DoAndReturn(func(string) (life.Value, error) {
			return life.Alive, nil
		}),
		tc.facade.EXPECT().ProvisioningInfo("test").DoAndReturn(func(string) (api.ProvisioningInfo, error) {
			return s.appProvisioningInfo, nil
		}),
		tc.facade.EXPECT().CharmInfo("cs:test").DoAndReturn(func(string) (*charmscommon.CharmInfo, error) {
			return s.appCharmInfo, nil
		}),
		tc.brokerApp.EXPECT().Exists().DoAndReturn(func() (caas.DeploymentState, error) {
			return caas.DeploymentState{}, nil
		}),
		tc.facade.EXPECT().ApplicationOCIResources("test").DoAndReturn(func(string) (map[string]resources.DockerImageDetails, error) {
			return s.ociResources, nil
		}),

		// refresh application status - test separately.
		tc.brokerApp.EXPECT().State().
			DoAndReturn(func() (caas.ApplicationState, error) {
				tc.notifyReady <- struct{}{}
				return caas.ApplicationState{}, errors.NotFoundf("")
			}),

		// Got appChanges -> updateState().
		tc.facade.EXPECT().Units("test").DoAndReturn(func(string) ([]params.CAASUnit, error) {
			return unitsAPIResultSingleActive, nil
		}),
		tc.brokerApp.EXPECT().State().DoAndReturn(func() (caas.ApplicationState, error) {
			return caas.ApplicationState{
				DesiredReplicas: 0,
				Replicas:        []string(nil),
			}, nil
		}),
		tc.brokerApp.EXPECT().Service().DoAndReturn(func() (*caas.Service, error) {
			return &caas.Service{
				Id:        "deadbeef",
				Addresses: network.NewProviderAddresses("10.6.6.6"),
			}, nil
		}),
		tc.unitFacade.EXPECT().UpdateApplicationService(params.UpdateApplicationServiceArg{
			ApplicationTag: "application-test",
			ProviderId:     "deadbeef",
			Addresses:      params.FromProviderAddresses(network.NewProviderAddress("10.6.6.6")),
		}).Return(nil),
		tc.facade.EXPECT().GarbageCollect("test", []names.Tag{names.NewUnitTag("test/0")}, 0, []string(nil), false).DoAndReturn(func(appName string, observedUnits []names.Tag, desiredReplicas int, activePodNames []string, force bool) error {
			return nil
		}),

		tc.brokerApp.EXPECT().Units().Return([]caas.Unit{{
			Id:    "test-0",
			Dying: true,
			Status: status.StatusInfo{
				Status: status.Terminated,
			},
		}}, nil),
		tc.facade.EXPECT().UpdateUnits(params.UpdateApplicationUnits{
			ApplicationTag: "application-test",
			Status:         params.EntityStatus{},
		}).Return(nil, nil),

		// refresh application status - test separately.
		tc.brokerApp.EXPECT().State().
			DoAndReturn(func() (caas.ApplicationState, error) {
				tc.notifyReady <- struct{}{}
				return caas.ApplicationState{}, errors.NotFoundf("")
			}),

		// 1st Notify() - dying.
		tc.facade.EXPECT().Life("test").DoAndReturn(func(string) (life.Value, error) {
			return life.Dying, nil
		}),
		tc.brokerApp.EXPECT().Delete().DoAndReturn(func() error {
			tc.notifyReady <- struct{}{}
			return nil
		}),

		// 2nd Notify() - dead.
		tc.facade.EXPECT().Life("test").DoAndReturn(func(string) (life.Value, error) {
			return life.Dead, nil
		}),
		tc.brokerApp.EXPECT().Delete().DoAndReturn(func() error {
			return nil
		}),
		tc.brokerApp.EXPECT().Exists().DoAndReturn(func() (caas.DeploymentState, error) {
			return caas.DeploymentState{
				Exists:      false,
				Terminating: false,
			}, nil
		}),
		tc.facade.EXPECT().Units("test").DoAndReturn(func(string) ([]params.CAASUnit, error) {
			return []params.CAASUnit(nil), nil
		}),
		tc.brokerApp.EXPECT().State().DoAndReturn(func() (caas.ApplicationState, error) {
			return caas.ApplicationState{
				DesiredReplicas: 0,
				Replicas:        []string(nil),
			}, nil
		}),
		tc.brokerApp.EXPECT().Service().DoAndReturn(func() (*caas.Service, error) {
			return nil, errors.NotFoundf("test")
		}),
		tc.facade.EXPECT().GarbageCollect("test", []names.Tag(nil), 0, []string(nil), true).DoAndReturn(func(appName string, observedUnits []names.Tag, desiredReplicas int, activePodNames []string, force bool) error {
			close(done)
			return nil
		}),
	}

	appWorker := newAppWorker(assertionCalls...)

	go func(w appNotifyWorker) {
		steps := []func(){
			// Test replica changes.
			func() { tc.appReplicasChan <- struct{}{} },
			// Test app state changes.
			func() { tc.appStateChan <- struct{}{} },
			// Test app changes from cloud.
			func() { tc.appChan <- struct{}{} },
			// Test Notify - dying.
			w.Notify,
			// Test Notify - dead.
			w.Notify,
		}
		for _, step := range steps {
			<-tc.notifyReady
			step()
		}
	}(appWorker.(appNotifyWorker))

	select {
	case <-done:
	case <-time.After(coretesting.ShortWait):
		c.Errorf("timed out waiting for worker")
	}
}

func (s *ApplicationWorkerSuite) TestScaleChanges(c *gc.C) {
	newAppWorker, tc, ctrl := s.getWorker(c)
	defer ctrl.Finish()

	done := make(chan struct{})

	assertionCalls := []*gomock.Call{
		tc.unitFacade.EXPECT().ApplicationScale("test").Return(3, nil),
		tc.brokerApp.EXPECT().Scale(3).Return(nil),

		// refresh application status - test separately.
		tc.brokerApp.EXPECT().State().
			DoAndReturn(func() (caas.ApplicationState, error) {
				close(done)
				return caas.ApplicationState{}, errors.NotFoundf("")
			}),
	}

	appWorker := newAppWorker(assertionCalls...)

	go func(w appNotifyWorker) {
		<-tc.notifyReady
		tc.appScaleChan <- struct{}{}
	}(appWorker.(appNotifyWorker))

	select {
	case <-done:
	case <-time.After(coretesting.ShortWait):
		c.Errorf("timed out waiting for worker")
	}
}

func (s *ApplicationWorkerSuite) TestTrustChanges(c *gc.C) {
	newAppWorker, tc, ctrl := s.getWorker(c)
	defer ctrl.Finish()

	done := make(chan struct{})
	assertionCalls := []*gomock.Call{
		tc.unitFacade.EXPECT().ApplicationTrust("test").Return(true, nil),
		tc.brokerApp.EXPECT().Trust(true).Return(nil),

		// refresh application status - test separately.
		tc.brokerApp.EXPECT().State().
			DoAndReturn(func() (caas.ApplicationState, error) {
				close(done)
				return caas.ApplicationState{}, errors.NotFoundf("")
			}),
	}

	appWorker := newAppWorker(assertionCalls...)

	go func(w appNotifyWorker) {
		<-tc.notifyReady
		tc.appTrustHashChan <- []string{"test"}
	}(appWorker.(appNotifyWorker))

	select {
	case <-done:
	case <-time.After(coretesting.ShortWait):
		c.Errorf("timed out waiting for worker")
	}
}

func (s *ApplicationWorkerSuite) assertRefreshApplicationStatus(
	c *gc.C, tc testCase, workerGetter func(additionalAssertCalls ...*gomock.Call) worker.Worker,
	assertionCalls ...*gomock.Call,
) {
	appWorker := workerGetter(assertionCalls...)
	go func(w appNotifyWorker) {
		<-tc.notifyReady
		w.Notify()
	}(appWorker.(appNotifyWorker))
}

func (s *ApplicationWorkerSuite) TestRefreshApplicationStatusNewUnitsAllocating(c *gc.C) {
	newAppWorker, tc, ctrl := s.getWorker(c)
	defer ctrl.Finish()

	done := make(chan struct{})

	s.assertRefreshApplicationStatus(c, tc, newAppWorker,
		tc.facade.EXPECT().Life("test").DoAndReturn(func(string) (life.Value, error) {
			return life.Alive, nil
		}),
		tc.facade.EXPECT().ProvisioningInfo("test").DoAndReturn(func(string) (api.ProvisioningInfo, error) {
			return s.appProvisioningInfo, nil
		}),
		tc.facade.EXPECT().CharmInfo("cs:test").DoAndReturn(func(string) (*charmscommon.CharmInfo, error) {
			return s.appCharmInfo, nil
		}),
		tc.brokerApp.EXPECT().Exists().DoAndReturn(func() (caas.DeploymentState, error) {
			return caas.DeploymentState{}, nil
		}),
		tc.facade.EXPECT().ApplicationOCIResources("test").DoAndReturn(func(string) (map[string]resources.DockerImageDetails, error) {
			return s.ociResources, nil
		}),

		// Desired: 3;
		tc.brokerApp.EXPECT().State().
			DoAndReturn(func() (caas.ApplicationState, error) {
				return caas.ApplicationState{DesiredReplicas: 3}, nil
			}),
		// Active unit: 1;
		tc.facade.EXPECT().Units("test").DoAndReturn(func(string) ([]params.CAASUnit, error) {
			c.Assert(len(unitsAPIResultSingleActive), gc.DeepEquals, 1)
			return unitsAPIResultSingleActive, nil
		}),
		tc.facade.EXPECT().SetOperatorStatus("test", status.Waiting, "waiting for units settled down", nil).
			DoAndReturn(func(string, status.Status, string, map[string]interface{}) error {
				close(done)
				return nil
			}),
	)
	select {
	case <-done:
	case <-time.After(coretesting.ShortWait):
		c.Errorf("timed out waiting for worker")
	}
}

func (s *ApplicationWorkerSuite) TestRefreshApplicationStatusAllUnitsAreSettled(c *gc.C) {
	newAppWorker, tc, ctrl := s.getWorker(c)
	defer ctrl.Finish()

	done := make(chan struct{})

	s.assertRefreshApplicationStatus(c, tc, newAppWorker,
		tc.facade.EXPECT().Life("test").DoAndReturn(func(string) (life.Value, error) {
			return life.Alive, nil
		}),
		tc.facade.EXPECT().ProvisioningInfo("test").DoAndReturn(func(string) (api.ProvisioningInfo, error) {
			return s.appProvisioningInfo, nil
		}),
		tc.facade.EXPECT().CharmInfo("cs:test").DoAndReturn(func(string) (*charmscommon.CharmInfo, error) {
			return s.appCharmInfo, nil
		}),
		tc.brokerApp.EXPECT().Exists().DoAndReturn(func() (caas.DeploymentState, error) {
			return caas.DeploymentState{}, nil
		}),
		tc.facade.EXPECT().ApplicationOCIResources("test").DoAndReturn(func(string) (map[string]resources.DockerImageDetails, error) {
			return s.ociResources, nil
		}),

		// Desired: 1;
		tc.brokerApp.EXPECT().State().
			DoAndReturn(func() (caas.ApplicationState, error) {
				return caas.ApplicationState{DesiredReplicas: 1}, nil
			}),
		// Active unit: 1;
		tc.facade.EXPECT().Units("test").DoAndReturn(func(string) ([]params.CAASUnit, error) {
			c.Assert(len(unitsAPIResultSingleActive), gc.DeepEquals, 1)
			return unitsAPIResultSingleActive, nil
		}),
		tc.facade.EXPECT().SetOperatorStatus("test", status.Active, "", nil).
			DoAndReturn(func(string, status.Status, string, map[string]interface{}) error {
				close(done)
				return nil
			}),
	)
	select {
	case <-done:
	case <-time.After(coretesting.ShortWait):
		c.Errorf("timed out waiting for worker")
	}
}

func (s *ApplicationWorkerSuite) TestRefreshApplicationStatusTransitionFromWaitingToActive(c *gc.C) {
	newAppWorker, tc, ctrl := s.getWorker(c)
	defer ctrl.Finish()

	done := make(chan struct{})

	s.assertRefreshApplicationStatus(c, tc, newAppWorker,
		tc.facade.EXPECT().Life("test").DoAndReturn(func(string) (life.Value, error) {
			return life.Alive, nil
		}),
		tc.facade.EXPECT().ProvisioningInfo("test").DoAndReturn(func(string) (api.ProvisioningInfo, error) {
			return s.appProvisioningInfo, nil
		}),
		tc.facade.EXPECT().CharmInfo("cs:test").DoAndReturn(func(string) (*charmscommon.CharmInfo, error) {
			return s.appCharmInfo, nil
		}),
		tc.brokerApp.EXPECT().Exists().DoAndReturn(func() (caas.DeploymentState, error) {
			return caas.DeploymentState{}, nil
		}),
		tc.facade.EXPECT().ApplicationOCIResources("test").DoAndReturn(func(string) (map[string]resources.DockerImageDetails, error) {
			return s.ociResources, nil
		}),
		// No change, so no Ensure().

		// Scaled up to 3;
		tc.brokerApp.EXPECT().State().
			DoAndReturn(func() (caas.ApplicationState, error) {
				return caas.ApplicationState{DesiredReplicas: 3}, nil
			}),
		// Total 3, active 1, waiting 2;
		tc.facade.EXPECT().Units("test").DoAndReturn(func(string) ([]params.CAASUnit, error) {
			c.Assert(len(unitsAPIResultPartialActive), gc.DeepEquals, 3)
			return unitsAPIResultPartialActive, nil
		}),
		tc.facade.EXPECT().SetOperatorStatus("test", status.Waiting, "waiting for units settled down", nil).
			DoAndReturn(func(string, status.Status, string, map[string]interface{}) error {
				err := tc.clock.WaitAdvance(10*time.Second, coretesting.ShortWait, 2)
				c.Assert(err, jc.ErrorIsNil)
				close(done)
				return nil
			}),
	)

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		c.Errorf("timed out waiting for worker")
	}
}

func (s *ApplicationWorkerSuite) TestRefreshApplicationStatusNoOpsForDyingApplication(c *gc.C) {
	newAppWorker, tc, ctrl := s.getWorker(c)
	defer ctrl.Finish()

	done := make(chan struct{})

	s.assertRefreshApplicationStatus(c, tc, newAppWorker,
		tc.facade.EXPECT().Life("test").DoAndReturn(func(string) (life.Value, error) {
			return life.Dying, nil
		}),
		tc.brokerApp.EXPECT().Delete().DoAndReturn(func() error {
			close(done)
			return nil
		}),
	)
	select {
	case <-done:
	case <-time.After(coretesting.ShortWait):
		c.Errorf("timed out waiting for worker")
	}
}

func (s *ApplicationWorkerSuite) TestRefreshApplicationStatusNoOpsForDeadApplication(c *gc.C) {
	newAppWorker, tc, ctrl := s.getWorker(c)
	defer ctrl.Finish()

	done := make(chan struct{})

	s.assertRefreshApplicationStatus(c, tc, newAppWorker,
		tc.facade.EXPECT().Life("test").DoAndReturn(func(string) (life.Value, error) {
			return life.Dead, nil
		}),
		tc.brokerApp.EXPECT().Delete().DoAndReturn(func() error {
			return nil
		}),
		tc.brokerApp.EXPECT().Exists().DoAndReturn(func() (caas.DeploymentState, error) {
			return caas.DeploymentState{
				Exists:      false,
				Terminating: false,
			}, nil
		}),
		tc.facade.EXPECT().Units("test").DoAndReturn(func(string) ([]params.CAASUnit, error) {
			return []params.CAASUnit(nil), nil
		}),
		tc.brokerApp.EXPECT().State().DoAndReturn(func() (caas.ApplicationState, error) {
			return caas.ApplicationState{
				DesiredReplicas: 0,
				Replicas:        []string(nil),
			}, nil
		}),
		tc.brokerApp.EXPECT().Service().DoAndReturn(func() (*caas.Service, error) {
			return nil, errors.NotFoundf("test")
		}),
		tc.facade.EXPECT().GarbageCollect("test", []names.Tag(nil), 0, []string(nil), true).DoAndReturn(func(appName string, observedUnits []names.Tag, desiredReplicas int, activePodNames []string, force bool) error {
			close(done)
			return nil
		}),
	)
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
