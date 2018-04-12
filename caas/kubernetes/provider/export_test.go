// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider

import (
	core "k8s.io/api/core/v1"

	"github.com/juju/juju/caas"
)

var (
	MakeUnitSpec    = makeUnitSpec
	ParseK8sPodSpec = parseK8sPodSpec
	OperatorPod     = operatorPod
)

func PodSpec(u *unitSpec) core.PodSpec {
	return u.Pod
}

func NewProvider() caas.ContainerEnvironProvider {
	return kubernetesEnvironProvider{}
}
