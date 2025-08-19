// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"github.com/juju/clock"
	"k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	"k8s.io/client-go/kubernetes"

	"github.com/juju/juju/caas"
	"github.com/juju/juju/internal/provider/kubernetes/constants"
	"github.com/juju/juju/internal/provider/kubernetes/resources"
	k8sutils "github.com/juju/juju/internal/provider/kubernetes/utils"
	k8swatcher "github.com/juju/juju/internal/provider/kubernetes/watcher"
)

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
	newWatcher k8swatcher.NewK8sWatcherFunc,
	clock clock.Clock,
	randomPrefix k8sutils.RandomPrefixFunc,
	newApplier func() resources.Applier,
	controllerUUID string,
) ApplicationInterfaceForTest {
	return newApplication(
		name, namespace, modelUUID, modelName, labelVersion, deploymentType,
		client, extendedClient, newWatcher, clock, randomPrefix, newApplier, controllerUUID,
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
