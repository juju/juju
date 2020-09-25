// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasapplicationprovisioner_test

import (
	"time"

	"github.com/golang/mock/gomock"
	"github.com/juju/charm/v8"
	charmresource "github.com/juju/charm/v8/resource"
	"github.com/juju/clock/testclock"
	"github.com/juju/loggo"
	"github.com/juju/names/v4"
	"github.com/juju/systems"
	"github.com/juju/systems/channel"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v2"
	"github.com/juju/worker/v2/workertest"
	gc "gopkg.in/check.v1"

	api "github.com/juju/juju/api/caasapplicationprovisioner"
	charmscommon "github.com/juju/juju/api/common/charms"
	"github.com/juju/juju/caas"
	caasmocks "github.com/juju/juju/caas/mocks"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/resources"
	"github.com/juju/juju/core/status"
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
	var err error
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	appWorker := worker.Worker(nil)

	appUnits := []names.Tag{
		names.NewUnitTag("test/0"),
	}
	appLife := life.Alive
	appCharmURL := &charm.URL{
		Schema:   "cs",
		Name:     "test",
		Revision: -1,
	}
	appCharmInfo := &charmscommon.CharmInfo{
		Meta: &charm.Meta{
			Name: "test",
			Platforms: []charm.Platform{
				charm.PlatformKubernetes,
			},
			Systems: []systems.System{{
				OS:      systems.Ubuntu,
				Channel: channel.MustParse("20.04/stable"),
			}},
			Containers: map[string]charm.Container{
				"test": charm.Container{
					Systems: []systems.System{{
						Resource: "test-oci",
					}},
				},
			},
			Resources: map[string]charmresource.Meta{
				"test-oci": charmresource.Meta{
					Type: charmresource.TypeContainerImage,
				},
			},
		},
	}
	appProvisioningInfo := api.ProvisioningInfo{
		Series: "focal",
	}
	ociResources := map[string]resources.DockerImageDetails{
		"test-oci": resources.DockerImageDetails{
			RegistryPath: "some/test:img",
		},
	}

	facade := mocks.NewMockCAASProvisionerFacade(ctrl)
	facade.EXPECT().Units("application-test").AnyTimes().DoAndReturn(func(string) ([]names.Tag, error) {
		return appUnits, nil
	})
	facade.EXPECT().Life("application-test").AnyTimes().DoAndReturn(func(string) (life.Value, error) {
		return appLife, nil
	})
	facade.EXPECT().ApplicationCharmURL("application-test").AnyTimes().DoAndReturn(func(string) (*charm.URL, error) {
		return appCharmURL, nil
	})
	facade.EXPECT().CharmInfo("cs:test").AnyTimes().DoAndReturn(func(string) (*charmscommon.CharmInfo, error) {
		return appCharmInfo, nil
	})
	facade.EXPECT().ProvisioningInfo("application-test").AnyTimes().DoAndReturn(func(string) (api.ProvisioningInfo, error) {
		return appProvisioningInfo, nil
	})
	facade.EXPECT().ApplicationOCIResources("application-test").AnyTimes().DoAndReturn(func(string) (map[string]resources.DockerImageDetails, error) {
		return ociResources, nil
	})

	appChan := make(chan struct{}, 1)
	appWatcher := watchertest.NewMockNotifyWatcher(appChan)
	appReplicasChan := make(chan struct{}, 1)
	appReplicasWatcher := watchertest.NewMockNotifyWatcher(appReplicasChan)
	appDeploymentState := caas.DeploymentState{}
	appState := caas.ApplicationState{}

	brokerApp := caasmocks.NewMockApplication(ctrl)
	broker := mocks.NewMockCAASBroker(ctrl)
	broker.EXPECT().Application("application-test", caas.DeploymentStateful).AnyTimes().Return(brokerApp)

	brokerApp.EXPECT().Watch().Return(appWatcher, nil)
	brokerApp.EXPECT().WatchReplicas().Return(appReplicasWatcher, nil)
	brokerApp.EXPECT().Exists().AnyTimes().DoAndReturn(func() (caas.DeploymentState, error) {
		return appDeploymentState, nil
	})
	brokerApp.EXPECT().State().AnyTimes().DoAndReturn(func() (caas.ApplicationState, error) {
		return appState, nil
	})

	done := make(chan struct{})
	gomock.InOrder(
		facade.EXPECT().SetPassword("application-test", gomock.Any()).Return(nil),
		brokerApp.EXPECT().Ensure(gomock.Any()).DoAndReturn(func(config caas.ApplicationConfig) error {
			mc := jc.NewMultiChecker()
			mc.AddExpr(`_.IntroductionSecret`, gc.HasLen, 24)
			mc.AddExpr(`_.Charm`, gc.NotNil)
			c.Check(config, mc, caas.ApplicationConfig{
				CharmBaseImage: resources.DockerImageDetails{
					RegistryPath: "jujusolutions/ubuntu:20.04",
				},
				Containers: map[string]caas.ContainerConfig{
					"test": caas.ContainerConfig{
						Name: "test",
						Image: resources.DockerImageDetails{
							RegistryPath: "some/test:img",
						},
					},
				},
			})
			appDeploymentState.Exists = true
			appState.DesiredReplicas = 1
			appState.Replicas = []string{"test-0"}
			appReplicasChan <- struct{}{}
			return nil
		}),
		facade.EXPECT().SetOperatorStatus("application-test", status.Active, "deployed", nil).Return(nil),
		facade.EXPECT().GarbageCollect("application-test", []names.Tag{names.NewUnitTag("test/0")}, 1, []string{"test-0"}, false).DoAndReturn(func(appName string, observedUnits []names.Tag, desiredReplicas int, activePodNames []string, force bool) error {
			appState.DesiredReplicas = 0
			appState.Replicas = []string(nil)
			appChan <- struct{}{}
			return nil
		}),
		facade.EXPECT().GarbageCollect("application-test", []names.Tag{names.NewUnitTag("test/0")}, 0, []string(nil), false).DoAndReturn(func(appName string, observedUnits []names.Tag, desiredReplicas int, activePodNames []string, force bool) error {
			appUnits = nil
			appLife = life.Dying
			appWorker.(appNotifyWorker).Notify()
			return nil
		}),
		brokerApp.EXPECT().Delete().DoAndReturn(func() error {
			appLife = life.Dead
			appDeploymentState.Terminating = true
			appWorker.(appNotifyWorker).Notify()
			return nil
		}),
		brokerApp.EXPECT().Delete().DoAndReturn(func() error {
			appLife = life.Dead
			appDeploymentState.Exists = false
			appDeploymentState.Terminating = false
			appWorker.(appNotifyWorker).Notify()
			return nil
		}),
		facade.EXPECT().GarbageCollect("application-test", []names.Tag(nil), 0, []string(nil), true).DoAndReturn(func(appName string, observedUnits []names.Tag, desiredReplicas int, activePodNames []string, force bool) error {
			close(done)
			return nil
		}),
	)

	config := caasapplicationprovisioner.AppWorkerConfig{
		Name:     "application-test",
		Facade:   facade,
		Broker:   broker,
		ModelTag: s.modelTag,
		Clock:    s.clock,
		Logger:   s.logger,
	}
	startFunc := caasapplicationprovisioner.NewAppWorker(config)
	c.Assert(startFunc, gc.NotNil)
	appWorker, err = startFunc()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(appWorker, gc.NotNil)

	select {
	case <-done:
	case <-time.After(coretesting.ShortWait):
		c.Errorf("timed out waiting for worker")
	}

	workertest.CleanKill(c, appWorker)
}

type appNotifyWorker interface {
	worker.Worker
	Notify()
}
