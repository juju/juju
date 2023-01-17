// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasapplicationprovisioner_test

import (
	"time"

	"github.com/golang/mock/gomock"
	"github.com/juju/charm/v10"
	charmresource "github.com/juju/charm/v10/resource"
	"github.com/juju/clock"
	"github.com/juju/clock/testclock"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names/v4"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v3"
	"github.com/juju/worker/v3/workertest"
	gc "gopkg.in/check.v1"

	charmscommon "github.com/juju/juju/api/common/charms"
	api "github.com/juju/juju/api/controller/caasapplicationprovisioner"
	"github.com/juju/juju/caas"
	caasmocks "github.com/juju/juju/caas/mocks"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/resources"
	"github.com/juju/juju/core/series"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/watchertest"
	"github.com/juju/juju/rpc/params"
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

	appScaleChan         chan struct{}
	notifyReady          chan struct{}
	appStateChan         chan struct{}
	appChan              chan struct{}
	appReplicasChan      chan struct{}
	appTrustHashChan     chan []string
	unitsChan            chan []string
	provisioningInfoChan chan struct{}
}

func (s *ApplicationWorkerSuite) getWorker(c *gc.C, name string) (func(...*gomock.Call) worker.Worker, testCase, *gomock.Controller) {
	ctrl := gomock.NewController(c)

	tc := testCase{}

	tc.clock = testclock.NewClock(time.Time{})
	tc.facade = mocks.NewMockCAASProvisionerFacade(ctrl)
	tc.broker = mocks.NewMockCAASBroker(ctrl)
	tc.unitFacade = mocks.NewMockCAASUnitProvisionerFacade(ctrl)
	tc.brokerApp = caasmocks.NewMockApplication(ctrl)

	s.appCharmInfo = &charmscommon.CharmInfo{
		Meta: &charm.Meta{
			Name: name,
			Containers: map[string]charm.Container{
				name: {
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
		Base: series.MakeDefaultBase("ubuntu", "20.04"),
		CharmURL: &charm.URL{
			Schema:   "ch",
			Name:     "test",
			Revision: -1,
		},
		Trust: true,
		Scale: 3,
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
	tc.unitsChan = make(chan []string, 1)
	tc.provisioningInfoChan = make(chan struct{}, 1)

	startFunc := func(additionalAssertCalls ...*gomock.Call) worker.Worker {
		config := caasapplicationprovisioner.AppWorkerConfig{
			Name:       name,
			Facade:     tc.facade,
			Broker:     tc.broker,
			ModelTag:   s.modelTag,
			Clock:      tc.clock,
			Logger:     s.logger,
			UnitFacade: tc.unitFacade,
		}
		expectedCalls := append([]*gomock.Call{},
			tc.broker.EXPECT().Application(name, caas.DeploymentStateful).Return(tc.brokerApp),
			tc.facade.EXPECT().Life(name).Return(life.Alive, nil),

			// Verify charm is v2
			tc.facade.EXPECT().WatchApplication(name).Return(watchertest.NewMockNotifyWatcher(tc.appStateChan), nil),
			tc.facade.EXPECT().ApplicationCharmInfo(name).Return(s.appCharmInfo, nil),

			// Operator delete loop
			tc.broker.EXPECT().OperatorExists(name).Return(caas.DeploymentState{Exists: false}, nil),

			// Pre-loop setup
			tc.facade.EXPECT().SetPassword(name, gomock.Any()).Return(nil),
			tc.unitFacade.EXPECT().WatchApplicationScale(name).Return(watchertest.NewMockNotifyWatcher(tc.appScaleChan), nil),
		)
		if name != "controller" {
			expectedCalls = append(expectedCalls,
				tc.unitFacade.EXPECT().WatchApplicationTrustHash(name).Return(watchertest.NewMockStringsWatcher(tc.appTrustHashChan), nil),
			)
		}
		expectedCalls = append(expectedCalls,
			tc.facade.EXPECT().WatchUnits(name).Return(watchertest.NewMockStringsWatcher(tc.unitsChan), nil),
			// Initial run - Ensure() for the application.
			tc.facade.EXPECT().Life(name).Return(life.Alive, nil),
			tc.facade.EXPECT().WatchProvisioningInfo(name).Return(watchertest.NewMockNotifyWatcher(tc.provisioningInfoChan), nil),
			tc.facade.EXPECT().ProvisioningInfo(name).Return(s.appProvisioningInfo, nil),
			tc.facade.EXPECT().CharmInfo("ch:test").Return(s.appCharmInfo, nil),
			tc.brokerApp.EXPECT().Exists().Return(caas.DeploymentState{}, nil),
			tc.facade.EXPECT().ApplicationOCIResources(name).Return(s.ociResources, nil),
		)
		if name != "controller" {
			expectedCalls = append(expectedCalls,

				tc.brokerApp.EXPECT().Ensure(gomock.Any()).DoAndReturn(func(config caas.ApplicationConfig) error {
					mc := jc.NewMultiChecker()
					mc.AddExpr(`_.IntroductionSecret`, gc.HasLen, 24)
					mc.AddExpr(`_.Charm`, gc.NotNil)
					c.Check(config, mc, caas.ApplicationConfig{
						CharmBaseImagePath: "jujusolutions/charm-base:ubuntu-20.04",
						Containers: map[string]caas.ContainerConfig{
							name: {
								Name: name,
								Image: resources.DockerImageDetails{
									RegistryPath: "some/test:img",
								},
							},
						},
						Trust:        true,
						InitialScale: 3,
					})
					return nil
				}))
		}
		expectedCalls = append(expectedCalls,
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

func (s *ApplicationWorkerSuite) waitDone(c *gc.C, done chan struct{}) {
	select {
	case <-done:
	case <-time.After(coretesting.ShortWait):
		c.Errorf("timed out waiting for worker")
	}
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
	s.assertWorker(c, "test")
}

func (s *ApplicationWorkerSuite) TestWorkerControllerApp(c *gc.C) {
	s.assertWorker(c, "controller")
}

func (s *ApplicationWorkerSuite) assertWorker(c *gc.C, name string) {
	newAppWorker, tc, ctrl := s.getWorker(c, name)
	defer ctrl.Finish()

	done := make(chan struct{})

	assertionCalls := []*gomock.Call{
		// Got replicaChanges -> updateState().
		tc.facade.EXPECT().Units(name).DoAndReturn(func(string) ([]params.CAASUnit, error) {
			return unitsAPIResultSingleActive, nil
		}),
		tc.brokerApp.EXPECT().State().DoAndReturn(func() (caas.ApplicationState, error) {
			return caas.ApplicationState{
				DesiredReplicas: 1,
				Replicas:        []string{name + "-0"},
			}, nil
		}),
		tc.brokerApp.EXPECT().Service().DoAndReturn(func() (*caas.Service, error) {
			return &caas.Service{
				Id:        "deadbeef",
				Addresses: network.NewMachineAddresses([]string{"10.6.6.6"}).AsProviderAddresses(),
			}, nil
		}),
		tc.unitFacade.EXPECT().UpdateApplicationService(params.UpdateApplicationServiceArg{
			ApplicationTag: "application-" + name,
			ProviderId:     "deadbeef",
			Addresses:      params.FromProviderAddresses(network.NewMachineAddress("10.6.6.6").AsProviderAddress()),
		}).Return(nil),
		tc.facade.EXPECT().GarbageCollect(name, []names.Tag{names.NewUnitTag("test/0")}, 1, []string{name + "-0"}, false).
			DoAndReturn(
				func(_ string, _ []names.Tag, _ int, _ []string, _ bool) error { return nil },
			),
		tc.brokerApp.EXPECT().Units().Return([]caas.Unit{{
			Id:      name + "-0",
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
			ApplicationTag: "application-" + name,
			Status:         params.EntityStatus{},
			Units: []params.ApplicationUnitParams{{
				ProviderId: name + "-0",
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
		tc.facade.EXPECT().Life(name).DoAndReturn(func(string) (life.Value, error) {
			return life.Alive, nil
		}),
		tc.facade.EXPECT().ProvisioningInfo(name).DoAndReturn(func(string) (api.ProvisioningInfo, error) {
			return s.appProvisioningInfo, nil
		}),
		tc.facade.EXPECT().CharmInfo("ch:test").DoAndReturn(func(string) (*charmscommon.CharmInfo, error) {
			return s.appCharmInfo, nil
		}),
		tc.brokerApp.EXPECT().Exists().DoAndReturn(func() (caas.DeploymentState, error) {
			return caas.DeploymentState{}, nil
		}),
		tc.facade.EXPECT().ApplicationOCIResources(name).DoAndReturn(func(string) (map[string]resources.DockerImageDetails, error) {
			return s.ociResources, nil
		}),

		// refresh application status - test separately.
		tc.brokerApp.EXPECT().State().
			DoAndReturn(func() (caas.ApplicationState, error) {
				tc.notifyReady <- struct{}{}
				return caas.ApplicationState{}, errors.NotFoundf("")
			}),

		// Got appChanges -> updateState().
		tc.facade.EXPECT().Units(name).DoAndReturn(func(string) ([]params.CAASUnit, error) {
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
				Addresses: network.NewMachineAddresses([]string{"10.6.6.6"}).AsProviderAddresses(),
			}, nil
		}),
		tc.unitFacade.EXPECT().UpdateApplicationService(params.UpdateApplicationServiceArg{
			ApplicationTag: "application-" + name,
			ProviderId:     "deadbeef",
			Addresses:      params.FromProviderAddresses(network.NewMachineAddress("10.6.6.6").AsProviderAddress()),
		}).Return(nil),
		tc.facade.EXPECT().GarbageCollect(name, []names.Tag{names.NewUnitTag("test/0")}, 0, []string(nil), false).DoAndReturn(func(appName string, observedUnits []names.Tag, desiredReplicas int, activePodNames []string, force bool) error {
			return nil
		}),

		tc.brokerApp.EXPECT().Units().Return([]caas.Unit{{
			Id:    name + "-0",
			Dying: true,
			Status: status.StatusInfo{
				Status: status.Terminated,
			},
		}}, nil),
		tc.facade.EXPECT().UpdateUnits(params.UpdateApplicationUnits{
			ApplicationTag: "application-" + name,
			Status:         params.EntityStatus{},
		}).Return(nil, nil),

		// refresh application status - test separately.
		tc.brokerApp.EXPECT().State().
			DoAndReturn(func() (caas.ApplicationState, error) {
				tc.notifyReady <- struct{}{}
				return caas.ApplicationState{}, errors.NotFoundf("")
			}),

		// 1st Notify() - dying.
		tc.facade.EXPECT().Life(name).DoAndReturn(func(string) (life.Value, error) {
			return life.Dying, nil
		}),
		tc.brokerApp.EXPECT().Scale(0).DoAndReturn(func(int) error {
			tc.notifyReady <- struct{}{}
			return nil
		}),

		// 2nd Notify() - dead.
		tc.facade.EXPECT().Life(name).DoAndReturn(func(string) (life.Value, error) {
			return life.Dead, nil
		}),
		tc.brokerApp.EXPECT().Scale(0).DoAndReturn(func(int) error {
			return nil
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
		tc.facade.EXPECT().ClearApplicationResources(name).Return(nil),
		tc.facade.EXPECT().Units(name).DoAndReturn(func(string) ([]params.CAASUnit, error) {
			return []params.CAASUnit(nil), nil
		}),
		tc.brokerApp.EXPECT().State().DoAndReturn(func() (caas.ApplicationState, error) {
			return caas.ApplicationState{
				DesiredReplicas: 0,
				Replicas:        []string(nil),
			}, nil
		}),
		tc.brokerApp.EXPECT().Service().DoAndReturn(func() (*caas.Service, error) {
			return nil, errors.NotFoundf(name)
		}),
		tc.facade.EXPECT().GarbageCollect(name, []names.Tag(nil), 0, []string(nil), true).DoAndReturn(func(appName string, observedUnits []names.Tag, desiredReplicas int, activePodNames []string, force bool) error {
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
			func() { tc.provisioningInfoChan <- struct{}{} },

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

	s.waitDone(c, done)
}

func (s *ApplicationWorkerSuite) TestScaleChanges(c *gc.C) {
	newAppWorker, tc, ctrl := s.getWorker(c, "test")
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

	s.waitDone(c, done)
}

func (s *ApplicationWorkerSuite) TestScaleRetry(c *gc.C) {
	newAppWorker, tc, ctrl := s.getWorker(c, "test")
	defer ctrl.Finish()

	done := make(chan struct{})
	assertionCalls := []*gomock.Call{
		tc.unitFacade.EXPECT().ApplicationScale("test").Return(3, nil),
		tc.brokerApp.EXPECT().Scale(3).DoAndReturn(func(int) error {
			go func() {
				// n is 5 due to:
				// * three times around the main select for the After(10s)
				// * After(0) from the initial appScaleWatcher change
				// * After(3s) for the retry
				err := tc.clock.WaitAdvance(3*time.Second, coretesting.ShortWait, 5)
				c.Assert(err, jc.ErrorIsNil)
			}()
			return errors.NotFoundf("")
		}),
		tc.unitFacade.EXPECT().ApplicationScale("test").Return(3, nil),
		tc.brokerApp.EXPECT().Scale(3).Return(nil),
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

	s.waitDone(c, done)
}

func (s *ApplicationWorkerSuite) TestTrustChanges(c *gc.C) {
	newAppWorker, tc, ctrl := s.getWorker(c, "test")
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

	s.waitDone(c, done)
}

func (s *ApplicationWorkerSuite) TestUnitChanges(c *gc.C) {
	newAppWorker, tc, ctrl := s.getWorker(c, "test")
	defer ctrl.Finish()

	done := make(chan struct{})
	assertionCalls := []*gomock.Call{
		tc.facade.EXPECT().Life("test-0").Return(life.Dead, nil),
		tc.facade.EXPECT().RemoveUnit("test-0").Return(nil),
		tc.facade.EXPECT().Life("test-1").Return(life.Alive, nil),
		tc.facade.EXPECT().Life("test-2").Return(life.Value(""), errors.NotFoundf("")),
		tc.facade.EXPECT().RemoveUnit("test-2").Return(nil),
		tc.facade.EXPECT().Life("test-3").Return(life.Dead, nil),
		tc.facade.EXPECT().RemoveUnit("test-3").Return(nil),
		tc.brokerApp.EXPECT().State().DoAndReturn(func() (caas.ApplicationState, error) {
			close(done)
			return caas.ApplicationState{}, errors.NotFoundf("")
		}),
	}

	appWorker := newAppWorker(assertionCalls...)

	go func(w appNotifyWorker) {
		<-tc.notifyReady
		tc.unitsChan <- []string{"test-0", "test-1", "test-2", "test-3"}
	}(appWorker.(appNotifyWorker))

	s.waitDone(c, done)
}

func (s *ApplicationWorkerSuite) TestTrustRetry(c *gc.C) {
	newAppWorker, tc, ctrl := s.getWorker(c, "test")
	defer ctrl.Finish()

	done := make(chan struct{})
	assertionCalls := []*gomock.Call{
		tc.unitFacade.EXPECT().ApplicationTrust("test").Return(true, nil),
		tc.brokerApp.EXPECT().Trust(true).DoAndReturn(func(bool) error {
			go func() {
				// n is 5 due to:
				// * three times around the main select for the After(10s)
				// * After(0) from the initial appTrustWatcher change
				// * After(3s) for the retry
				err := tc.clock.WaitAdvance(3*time.Second, coretesting.ShortWait, 5)
				c.Assert(err, jc.ErrorIsNil)
			}()
			return errors.NotFoundf("")
		}),
		tc.unitFacade.EXPECT().ApplicationTrust("test").Return(true, nil),
		tc.brokerApp.EXPECT().Trust(true).Return(nil),
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

	s.waitDone(c, done)
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
	newAppWorker, tc, ctrl := s.getWorker(c, "test")
	defer ctrl.Finish()

	done := make(chan struct{})

	s.assertRefreshApplicationStatus(c, tc, newAppWorker,
		tc.facade.EXPECT().Life("test").DoAndReturn(func(string) (life.Value, error) {
			return life.Alive, nil
		}),
		tc.facade.EXPECT().ProvisioningInfo("test").DoAndReturn(func(string) (api.ProvisioningInfo, error) {
			return s.appProvisioningInfo, nil
		}),
		tc.facade.EXPECT().CharmInfo("ch:test").DoAndReturn(func(string) (*charmscommon.CharmInfo, error) {
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
		tc.facade.EXPECT().SetOperatorStatus("test", status.Waiting, "waiting for units to settle down", nil).
			DoAndReturn(func(string, status.Status, string, map[string]interface{}) error {
				close(done)
				return nil
			}),
	)
	s.waitDone(c, done)
}

func (s *ApplicationWorkerSuite) TestRefreshApplicationStatusAllUnitsAreSettled(c *gc.C) {
	newAppWorker, tc, ctrl := s.getWorker(c, "test")
	defer ctrl.Finish()

	done := make(chan struct{})

	s.assertRefreshApplicationStatus(c, tc, newAppWorker,
		tc.facade.EXPECT().Life("test").DoAndReturn(func(string) (life.Value, error) {
			return life.Alive, nil
		}),
		tc.facade.EXPECT().ProvisioningInfo("test").DoAndReturn(func(string) (api.ProvisioningInfo, error) {
			return s.appProvisioningInfo, nil
		}),
		tc.facade.EXPECT().CharmInfo("ch:test").DoAndReturn(func(string) (*charmscommon.CharmInfo, error) {
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
	s.waitDone(c, done)
}

func (s *ApplicationWorkerSuite) TestRefreshApplicationStatusTransitionFromWaitingToActive(c *gc.C) {
	newAppWorker, tc, ctrl := s.getWorker(c, "test")
	defer ctrl.Finish()

	done := make(chan struct{})

	s.assertRefreshApplicationStatus(c, tc, newAppWorker,
		tc.facade.EXPECT().Life("test").DoAndReturn(func(string) (life.Value, error) {
			return life.Alive, nil
		}),
		tc.facade.EXPECT().ProvisioningInfo("test").DoAndReturn(func(string) (api.ProvisioningInfo, error) {
			return s.appProvisioningInfo, nil
		}),
		tc.facade.EXPECT().CharmInfo("ch:test").DoAndReturn(func(string) (*charmscommon.CharmInfo, error) {
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
		tc.facade.EXPECT().SetOperatorStatus("test", status.Waiting, "waiting for units to settle down", nil).
			DoAndReturn(func(string, status.Status, string, map[string]interface{}) error {
				err := tc.clock.WaitAdvance(10*time.Second, coretesting.ShortWait, 2)
				c.Assert(err, jc.ErrorIsNil)
				close(done)
				return nil
			}),
	)

	s.waitDone(c, done)
}

func (s *ApplicationWorkerSuite) TestRefreshApplicationStatusNoOpsForDyingApplication(c *gc.C) {
	newAppWorker, tc, ctrl := s.getWorker(c, "test")
	defer ctrl.Finish()

	done := make(chan struct{})

	s.assertRefreshApplicationStatus(c, tc, newAppWorker,
		tc.facade.EXPECT().Life("test").DoAndReturn(func(string) (life.Value, error) {
			return life.Dying, nil
		}),
		tc.brokerApp.EXPECT().Scale(0).DoAndReturn(func(int) error {
			close(done)
			return nil
		}),
	)
	s.waitDone(c, done)
}

func (s *ApplicationWorkerSuite) TestRefreshApplicationStatusNoOpsForDeadApplication(c *gc.C) {
	newAppWorker, tc, ctrl := s.getWorker(c, "test")
	defer ctrl.Finish()

	done := make(chan struct{})

	s.assertRefreshApplicationStatus(c, tc, newAppWorker,
		tc.facade.EXPECT().Life("test").DoAndReturn(func(string) (life.Value, error) {
			return life.Dead, nil
		}),
		tc.brokerApp.EXPECT().Scale(0).DoAndReturn(func(int) error {
			return nil
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
		tc.facade.EXPECT().ClearApplicationResources("test").Return(nil),
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
	s.waitDone(c, done)
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
	brokerApp := caasmocks.NewMockApplication(ctrl)
	facade := mocks.NewMockCAASProvisionerFacade(ctrl)
	done := make(chan struct{})

	gomock.InOrder(
		broker.EXPECT().Application("test", caas.DeploymentStateful).Return(brokerApp),
		facade.EXPECT().Life("test").Return(life.Alive, nil),

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
			close(done)
			return errors.New("exit early error")
		}),
	)

	appWorker := s.startAppWorker(c, nil, facade, broker, nil)

	s.waitDone(c, done)
	workertest.DirtyKill(c, appWorker)
}

func (s *ApplicationWorkerSuite) startAppWorker(
	c *gc.C,
	clk clock.Clock,
	facade caasapplicationprovisioner.CAASProvisionerFacade,
	broker caasapplicationprovisioner.CAASBroker,
	unitFacade caasapplicationprovisioner.CAASUnitProvisionerFacade,
) worker.Worker {
	config := caasapplicationprovisioner.AppWorkerConfig{
		Name:       "test",
		Facade:     facade,
		Broker:     broker,
		ModelTag:   s.modelTag,
		Clock:      clk,
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

func (s *ApplicationWorkerSuite) TestLifeNotFound(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	broker := mocks.NewMockCAASBroker(ctrl)
	brokerApp := caasmocks.NewMockApplication(ctrl)
	facade := mocks.NewMockCAASProvisionerFacade(ctrl)
	done := make(chan struct{})

	gomock.InOrder(
		broker.EXPECT().Application("test", caas.DeploymentStateful).Return(brokerApp),
		facade.EXPECT().Life("test").DoAndReturn(func(appName string) (life.Value, error) {
			close(done)
			return "", errors.NotFoundf("test charm")
		}),
	)
	appWorker := s.startAppWorker(c, nil, facade, broker, nil)

	s.waitDone(c, done)
	workertest.CleanKill(c, appWorker)
}

func (s *ApplicationWorkerSuite) TestUpgradeInfoNotFound(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	appStateChan := make(chan struct{}, 1)
	appStateWatcher := watchertest.NewMockNotifyWatcher(appStateChan)
	broker := mocks.NewMockCAASBroker(ctrl)
	brokerApp := caasmocks.NewMockApplication(ctrl)
	facade := mocks.NewMockCAASProvisionerFacade(ctrl)
	done := make(chan struct{})

	gomock.InOrder(
		broker.EXPECT().Application("test", caas.DeploymentStateful).Return(brokerApp),
		facade.EXPECT().Life("test").Return(life.Alive, nil),

		// Wait till charm is v2
		facade.EXPECT().WatchApplication("test").Return(appStateWatcher, nil),
		facade.EXPECT().ApplicationCharmInfo("test").DoAndReturn(func(appName string) (*charmscommon.CharmInfo, error) {
			close(done)
			return nil, errors.NotFoundf("test charm")
		}),
	)
	appWorker := s.startAppWorker(c, nil, facade, broker, nil)

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
	brokerApp := caasmocks.NewMockApplication(ctrl)
	facade := mocks.NewMockCAASProvisionerFacade(ctrl)
	done := make(chan struct{})

	gomock.InOrder(
		broker.EXPECT().Application("test", caas.DeploymentStateful).Return(brokerApp),
		facade.EXPECT().Life("test").Return(life.Alive, nil),

		// Wait till charm is v2
		facade.EXPECT().WatchApplication("test").Return(appStateWatcher, nil),
		facade.EXPECT().ApplicationCharmInfo("test").Return(charmInfoV1, nil),
		facade.EXPECT().Life("test").DoAndReturn(func(appName string) (life.Value, error) {
			close(done)
			return "", errors.NotFoundf("test charm")
		}),
	)

	appWorker := s.startAppWorker(c, nil, facade, broker, nil)

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
	brokerApp := caasmocks.NewMockApplication(ctrl)
	facade := mocks.NewMockCAASProvisionerFacade(ctrl)
	done := make(chan struct{})

	gomock.InOrder(
		broker.EXPECT().Application("test", caas.DeploymentStateful).Return(brokerApp),
		facade.EXPECT().Life("test").Return(life.Alive, nil),

		// Wait till charm is v2
		facade.EXPECT().WatchApplication("test").Return(appStateWatcher, nil),
		facade.EXPECT().ApplicationCharmInfo("test").Return(charmInfoV1, nil),
		facade.EXPECT().Life("test").DoAndReturn(func(appName string) (life.Value, error) {
			close(done)
			return life.Dead, nil
		}),
	)

	appWorker := s.startAppWorker(c, nil, facade, broker, nil)

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

	appStateChan := make(chan struct{}, 1)
	appStateWatcher := watchertest.NewMockNotifyWatcher(appStateChan)
	broker := mocks.NewMockCAASBroker(ctrl)
	brokerApp := caasmocks.NewMockApplication(ctrl)
	facade := mocks.NewMockCAASProvisionerFacade(ctrl)
	done := make(chan struct{})

	clk := testclock.NewClock(time.Time{})
	gomock.InOrder(
		broker.EXPECT().Application("test", caas.DeploymentStateful).Return(brokerApp),
		facade.EXPECT().Life("test").Return(life.Alive, nil),

		// Verify charm is v2
		facade.EXPECT().WatchApplication("test").Return(appStateWatcher, nil),
		facade.EXPECT().ApplicationCharmInfo("test").Return(charmInfo, nil),

		// Operator delete loop (with a retry)
		broker.EXPECT().OperatorExists("test").Return(caas.DeploymentState{Exists: true}, nil),
		broker.EXPECT().DeleteService("test").Return(nil),
		broker.EXPECT().Units("test", caas.ModeWorkload).Return([]caas.Unit{}, nil),
		broker.EXPECT().DeleteOperator("test").DoAndReturn(func(appName string) error {
			go func() {
				err := clk.WaitAdvance(3*time.Second, coretesting.ShortWait, 1)
				c.Assert(err, jc.ErrorIsNil)
			}()
			return nil
		}),
		broker.EXPECT().OperatorExists("test").Return(caas.DeploymentState{Exists: false}, nil),

		// Make SetPassword return an error to exit early (we've tested what
		// we want to above).
		facade.EXPECT().SetPassword("test", gomock.Any()).DoAndReturn(func(appName, password string) error {
			close(done)
			return errors.New("exit early error")
		}),
	)

	appWorker := s.startAppWorker(c, clk, facade, broker, nil)

	s.waitDone(c, done)
	workertest.DirtyKill(c, appWorker)
}
