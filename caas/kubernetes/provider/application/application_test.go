// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application_test

import (
	"time"

	"github.com/juju/charm/v7"
	jujuclock "github.com/juju/clock"
	"github.com/juju/clock/testclock"
	"github.com/juju/errors"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	// core "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/tools/cache"

	// apicommoncharms "github.com/juju/juju/api/common/charms"
	"github.com/juju/juju/caas"
	"github.com/juju/juju/caas/kubernetes/provider/application"
	k8swatcher "github.com/juju/juju/caas/kubernetes/provider/watcher"
	"github.com/juju/juju/testing"
)

type applicationSuite struct {
	testing.BaseSuite
	client *fake.Clientset

	namespace    string
	appName      string
	clock        *testclock.Clock
	k8sWatcherFn k8swatcher.NewK8sWatcherFunc
	watchers     []k8swatcher.KubernetesNotifyWatcher
}

var _ = gc.Suite(&applicationSuite{})

func (s *applicationSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)

	s.namespace = "test"
	s.appName = "gitlab"
	s.client = fake.NewSimpleClientset()
	s.clock = testclock.NewClock(time.Time{})
}

func (s *applicationSuite) TearDownTest(c *gc.C) {
	s.client = nil
	s.clock = nil
	s.watchers = nil

	s.BaseSuite.TearDownTest(c)
}

func (s *applicationSuite) getApp(deploymentType caas.DeploymentType) caas.Application {
	watcherFn := k8swatcher.NewK8sWatcherFunc(func(i cache.SharedIndexInformer, n string, c jujuclock.Clock) (k8swatcher.KubernetesNotifyWatcher, error) {
		if s.k8sWatcherFn == nil {
			return nil, errors.NewNotFound(nil, "undefined k8sWatcherFn for base test")
		}

		w, err := s.k8sWatcherFn(i, n, c)
		if err == nil {
			s.watchers = append(s.watchers, w)
		}
		return w, err
	})

	return application.NewApplication(
		s.appName, s.namespace, "test",
		deploymentType,
		s.client,
		watcherFn,
		s.clock,
	)
}

func (s *applicationSuite) getCharm(deployment *charm.Deployment) charm.Charm {
	return &fakeCharm{deployment}
}

func (s *applicationSuite) TestEnsureFailed(c *gc.C) {
	app := s.getApp(caas.DeploymentType("notsupported"))
	c.Assert(app.Ensure(
		caas.ApplicationConfig{
			Charm: s.getCharm(&charm.Deployment{
				DeploymentType:     charm.DeploymentType("notsupported"),
				ContainerImageName: "gitlab:latest",
			}),
		},
	), gc.ErrorMatches, `unknown deployment type not supported`)

	app = s.getApp(caas.DeploymentStateless)
	c.Assert(app.Ensure(
		caas.ApplicationConfig{},
	), gc.ErrorMatches, `charm was missing for gitlab application not valid`)

	c.Assert(app.Ensure(
		caas.ApplicationConfig{
			Charm: s.getCharm(&charm.Deployment{
				DeploymentType: charm.DeploymentStateful,
			}),
		},
	), gc.ErrorMatches, `charm deployment type mismatch with application not valid`)

	c.Assert(app.Ensure(
		caas.ApplicationConfig{
			Charm: s.getCharm(&charm.Deployment{
				DeploymentType: charm.DeploymentStateless,
			}),
		},
	), gc.ErrorMatches, `generating application podspec: charm missing container-image-name not valid`)

}

func (s *applicationSuite) TestEnsure(c *gc.C) {
	app := s.getApp(caas.DeploymentStateless)
	c.Assert(app.Ensure(
		caas.ApplicationConfig{},
	), gc.ErrorMatches, `charm was missing for gitlab application not valid`)

	c.Assert(app.Ensure(
		caas.ApplicationConfig{
			Charm: s.getCharm(&charm.Deployment{
				DeploymentType:     charm.DeploymentStateless,
				ContainerImageName: "gitlab:latest",
			}),
		},
	), jc.ErrorIsNil)
}

type fakeCharm struct {
	// TODO: remove this once `api/common/charms.CharmInfo` has upgraded to use the new charm.Charm.
	deployment *charm.Deployment
}

func (c *fakeCharm) Meta() *charm.Meta {
	return &charm.Meta{Deployment: c.deployment}
}

func (c *fakeCharm) Config() *charm.Config {
	return nil
}

func (c *fakeCharm) Metrics() *charm.Metrics {
	return nil
}

func (c *fakeCharm) Actions() *charm.Actions {
	return nil
}

func (c *fakeCharm) Revision() int {
	return 0
}
