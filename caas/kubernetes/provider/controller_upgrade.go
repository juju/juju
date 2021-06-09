// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/version/v2"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/juju/juju/caas/kubernetes/provider/resources"
	providerutils "github.com/juju/juju/caas/kubernetes/provider/utils"
	"github.com/juju/juju/environs/bootstrap"
)

type upgradeCAASControllerBridge struct {
	clientFn    func() kubernetes.Interface
	isLegacyFn  func() bool
	namespaceFn func() string
}

// UpgradeCAASControllerBroker describes the interface needed for upgrading
// Juju Kubernetes controllers
type UpgradeCAASControllerBroker interface {
	// Client returns a Kubernetes client associated with the current broker's
	// cluster
	Client() kubernetes.Interface

	// IsLegacyLabels indicates if this provider is operating on a legacy label schema
	IsLegacyLabels() bool

	// Namespace returns the targeted Kubernetes namespace for this broker
	Namespace() string
}

func (u *upgradeCAASControllerBridge) Client() kubernetes.Interface {
	return u.clientFn()
}

func (u *upgradeCAASControllerBridge) IsLegacyLabels() bool {
	return u.isLegacyFn()
}

func (u *upgradeCAASControllerBridge) Namespace() string {
	return u.namespaceFn()
}

func controllerUpgrade(appName string, vers version.Number, broker UpgradeCAASControllerBroker) error {
	return upgradeStatefulSet(
		appName,
		"",
		vers,
		broker.IsLegacyLabels(),
		broker.Client().AppsV1().StatefulSets(broker.Namespace()))
}

func (k *kubernetesClient) upgradeController(vers version.Number) error {
	broker := &upgradeCAASControllerBridge{
		clientFn:    k.client,
		namespaceFn: k.GetCurrentNamespace,
		isLegacyFn:  k.IsLegacyLabels,
	}
	return controllerUpgrade(bootstrap.ControllerModelName, vers, broker)
}

// InClusterCredentialUpgrade implements upgrades.upgradeKubernetesClusterCredential
// used in the Juju 2.9.6 upgrade step
func (k *kubernetesClient) InClusterCredentialUpgrade() error {
	return inClusterCredentialUpgrade(
		k.client(),
		k.IsLegacyLabels(),
		k.GetCurrentNamespace(),
	)
}

func inClusterCredentialUpgrade(
	client kubernetes.Interface,
	legacyLabels bool,
	namespace string,
) error {
	ctx := context.TODO()
	labels := providerutils.LabelsForApp("controller", legacyLabels)

	saName, cleanUps, err := ensureControllerServiceAccount(
		ctx,
		client,
		namespace,
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
