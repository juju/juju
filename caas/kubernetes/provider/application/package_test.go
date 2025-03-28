// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"testing"

	"github.com/juju/clock"
	gc "gopkg.in/check.v1"
	"k8s.io/client-go/kubernetes"

	"github.com/juju/juju/caas"
	"github.com/juju/juju/caas/kubernetes/provider/resources"
	k8sutils "github.com/juju/juju/caas/kubernetes/provider/utils"
	k8swatcher "github.com/juju/juju/caas/kubernetes/provider/watcher"
	"github.com/juju/juju/core/k8s"
)

func Test(t *testing.T) {
	gc.TestingT(t)
}

type (
	AnnotationUpdater = annotationUpdater
)

func (a *app) IsLegacyLabels() bool {
	return a.legacyLabels
}

type ApplicationInterfaceForTest interface {
	caas.Application
	IsLegacyLabels() bool
}

func NewApplicationForTest(
	name string,
	namespace string,
	modelUUID string,
	modelName string,
	legacyLabels bool,
	deploymentType k8s.WorkloadType,
	client kubernetes.Interface,
	newWatcher k8swatcher.NewK8sWatcherFunc,
	clock clock.Clock,
	randomPrefix k8sutils.RandomPrefixFunc,
	newApplier func() resources.Applier,
) ApplicationInterfaceForTest {
	return newApplication(
		name, namespace, modelUUID, modelName, legacyLabels, deploymentType,
		client, newWatcher, clock, randomPrefix, newApplier,
	)
}

func PVCNames(client kubernetes.Interface, namespace, appName, storagePrefix string) (map[string]string, error) {
	a := &app{
		name:      appName,
		namespace: namespace,
		client:    client,
	}
	return a.pvcNames(storagePrefix)
}
