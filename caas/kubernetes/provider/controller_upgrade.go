// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider

import (
	"github.com/juju/version"
	"k8s.io/client-go/kubernetes"

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
