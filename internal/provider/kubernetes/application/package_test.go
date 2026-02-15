// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"regexp"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"

	"github.com/juju/juju/caas"
	"github.com/juju/juju/internal/provider/kubernetes/constants"
	"github.com/juju/juju/internal/provider/kubernetes/resources"
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
	dynamicClient dynamic.Interface,
	newWatcher k8swatcher.NewK8sWatcherFunc,
	clock clock.Clock,
	newApplier func() resources.Applier,
	controllerUUID string,
) (ApplicationInterfaceForTest, error) {
	reg, err := regexp.Compile(`^(.+)-` + regexp.QuoteMeta(name) + `-\d+$`)
	if err != nil {
		return nil, errors.Annotatef(err, "compiling regex to get pvc template name")
	}
	return newApplication(
		name, namespace, modelUUID, modelName, labelVersion, deploymentType,
		client, extendedClient, dynamicClient, newWatcher, clock, newApplier,
		controllerUUID, reg,
	), nil
}
