// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider

import (
	"context"

	"github.com/juju/errors"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/juju/juju/caas/kubernetes/provider/constants"
	"github.com/juju/juju/caas/kubernetes/provider/resources"
	providerutils "github.com/juju/juju/caas/kubernetes/provider/utils"
	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/environs/bootstrap"
)

type upgradeCAASControllerBridge struct {
	clientFn       func() kubernetes.Interface
	labelVersionFn func() constants.LabelVersion
	namespaceFn    func() string
}

// UpgradeCAASControllerBroker describes the interface needed for upgrading
// Juju Kubernetes controllers
type UpgradeCAASControllerBroker interface {
	// Client returns a Kubernetes client associated with the current broker's
	// cluster
	Client() kubernetes.Interface

	// LabelVersion returns the detected label version for k8s resources created
	// for this model.
	LabelVersion() constants.LabelVersion

	// Namespace returns the targeted Kubernetes namespace for this broker
	Namespace() string
}

func (u *upgradeCAASControllerBridge) Client() kubernetes.Interface {
	return u.clientFn()
}

func (u *upgradeCAASControllerBridge) LabelVersion() constants.LabelVersion {
	return u.labelVersionFn()
}

func (u *upgradeCAASControllerBridge) Namespace() string {
	return u.namespaceFn()
}

func controllerUpgrade(ctx context.Context, appName string, vers semversion.Number, broker UpgradeCAASControllerBroker) error {
	return upgradeOperatorOrControllerStatefulSet(
		ctx,
		appName,
		"",
		vers,
		broker.LabelVersion(),
		broker.Client().AppsV1().StatefulSets(broker.Namespace()))
}

func (k *kubernetesClient) upgradeController(ctx context.Context, vers semversion.Number) error {
	broker := &upgradeCAASControllerBridge{
		clientFn:       k.client,
		namespaceFn:    k.Namespace,
		labelVersionFn: k.LabelVersion,
	}
	return controllerUpgrade(ctx, bootstrap.ControllerModelName, vers, broker)
}

// InClusterCredentialUpgrade implements upgrades.upgradeKubernetesClusterCredential
// used in the Juju 2.9.6 upgrade step
func (k *kubernetesClient) InClusterCredentialUpgrade(ctx context.Context) error {
	return inClusterCredentialUpgrade(
		ctx,
		k.client(),
		k.LabelVersion(),
		k.Namespace(),
		k.ControllerUUID(),
	)
}

func inClusterCredentialUpgrade(
	ctx context.Context,
	client kubernetes.Interface,
	labelVersion constants.LabelVersion,
	namespace string,
	controllerUUID string,
) error {
	labels := providerutils.LabelsForApp("controller", labelVersion)

	saName, cleanUps, err := ensureControllerServiceAccount(
		ctx,
		client,
		namespace,
		controllerUUID,
		labels,
		map[string]string{},
	)

	runCleanups := func() {
		for _, v := range cleanUps {
			v()
		}
	}

	if err != nil {
		runCleanups()
		return errors.Trace(err)
	}

	ss := resources.NewStatefulSet("controller", namespace, &appsv1.StatefulSet{})
	if err := ss.Get(ctx, client); err != nil {
		runCleanups()
		return errors.Annotate(err, "updating controller for in cluster credentials")
	}

	ss.Spec.Template.Spec.ServiceAccountName = saName
	ss.Spec.Template.Spec.AutomountServiceAccountToken = boolPtr(true)
	if err := ss.Apply(ctx, client); err != nil {
		runCleanups()
		return errors.Annotate(err, "updating controller for in cluster credentials")
	}

	return nil
}
