// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider

import (
	"github.com/juju/names/v5"
	"github.com/juju/version/v2"
	"k8s.io/client-go/kubernetes"

	"github.com/juju/juju/caas/kubernetes/provider/constants"
)

type upgradeCAASModelOperatorBridge struct {
	clientFn       func() kubernetes.Interface
	namespaceFn    func() string
	labelVersionFn func() constants.LabelVersion
}

type UpgradeCAASModelOperatorBroker interface {
	// Client returns a Kubernetes client associated with the current broker's
	// cluster
	Client() kubernetes.Interface

	// LabelVersion returns the detected label version for k8s resources created
	// for this model.
	LabelVersion() constants.LabelVersion

	// Namespace returns the targeted Kubernetes namespace for this broker
	Namespace() string
}

func (u *upgradeCAASModelOperatorBridge) Client() kubernetes.Interface {
	return u.clientFn()
}

func (u *upgradeCAASModelOperatorBridge) LabelVersion() constants.LabelVersion {
	return u.labelVersionFn()
}

func modelOperatorUpgrade(
	operatorName string,
	vers version.Number,
	broker UpgradeCAASModelOperatorBroker) error {
	return upgradeDeployment(
		operatorName,
		"",
		vers,
		broker.LabelVersion(),
		broker.Client().AppsV1().Deployments(broker.Namespace()))
}

func (u *upgradeCAASModelOperatorBridge) Namespace() string {
	return u.namespaceFn()
}

func (k *kubernetesClient) upgradeModelOperator(agentTag names.Tag, vers version.Number) error {
	broker := &upgradeCAASModelOperatorBridge{
		clientFn:       k.client,
		namespaceFn:    k.Namespace,
		labelVersionFn: k.LabelVersion,
	}
	return modelOperatorUpgrade(modelOperatorName, vers, broker)
}
