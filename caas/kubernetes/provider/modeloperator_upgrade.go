// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider

import (
	"context"

	"k8s.io/client-go/kubernetes"

	"github.com/juju/juju/core/semversion"
)

type upgradeCAASModelOperatorBridge struct {
	clientFn    func() kubernetes.Interface
	namespaceFn func() string
	isLegacyFn  func() bool
}

type UpgradeCAASModelOperatorBroker interface {
	// Client returns a Kubernetes client associated with the current broker's
	// cluster
	Client() kubernetes.Interface

	// IsLegacyLabels indicates if this provider is operating on a legacy label schema
	IsLegacyLabels() bool

	// Namespace returns the targeted Kubernetes namespace for this broker
	Namespace() string
}

func (u *upgradeCAASModelOperatorBridge) Client() kubernetes.Interface {
	return u.clientFn()
}

func (u *upgradeCAASModelOperatorBridge) IsLegacyLabels() bool {
	return u.isLegacyFn()
}

func modelOperatorUpgrade(
	ctx context.Context,
	operatorName string,
	vers semversion.Number,
	broker UpgradeCAASModelOperatorBroker) error {
	return upgradeDeployment(
		ctx,
		operatorName,
		"",
		vers,
		broker.IsLegacyLabels(),
		broker.Client().AppsV1().Deployments(broker.Namespace()))
}

func (u *upgradeCAASModelOperatorBridge) Namespace() string {
	return u.namespaceFn()
}

func (k *kubernetesClient) upgradeModelOperator(ctx context.Context, vers semversion.Number) error {
	broker := &upgradeCAASModelOperatorBridge{
		clientFn:    k.client,
		namespaceFn: k.GetCurrentNamespace,
		isLegacyFn:  k.IsLegacyLabels,
	}
	return modelOperatorUpgrade(ctx, modelOperatorName, vers, broker)
}
