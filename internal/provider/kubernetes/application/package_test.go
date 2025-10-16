// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"testing"

	"github.com/juju/clock"
	gc "gopkg.in/check.v1"
	"k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"

	"github.com/juju/juju/caas"
	"github.com/juju/juju/internal/provider/kubernetes/constants"
	"github.com/juju/juju/internal/provider/kubernetes/resources"
	k8swatcher "github.com/juju/juju/internal/provider/kubernetes/watcher"
)

func Test(t *testing.T) {
	gc.TestingT(t)
}

type (
	AnnotationUpdater = annotationUpdater
)

func (a *app) LabelVersion() constants.LabelVersion {
	return a.labelVersion
}

type ApplicationInterfaceForTest interface {
	caas.Application
	LabelVersion() constants.LabelVersion
}

func NewApplicationForTest(
	name string,
	namespace string,
	modelUUID string,
	modelName string,
	labelVersion constants.LabelVersion,
	deploymentType caas.DeploymentType,
	client kubernetes.Interface,
	extendedClient clientset.Interface,
	dynamicClient dynamic.Interface,
	newWatcher k8swatcher.NewK8sWatcherFunc,
	clock clock.Clock,
	newApplier func() resources.Applier,
	controllerUUID string,
) ApplicationInterfaceForTest {
	return newApplication(
		name, namespace, modelUUID, modelName, labelVersion, deploymentType,
		client, extendedClient, dynamicClient, newWatcher, clock, newApplier, controllerUUID,
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
