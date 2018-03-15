// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider

import (
	"k8s.io/client-go/pkg/api/v1"

	"github.com/juju/juju/caas"
)

var (
	MakeUnitSpec = makeUnitSpec
	OperatorPod  = operatorPod
)

func PodSpec(u *unitSpec) v1.PodSpec {
	return u.Pod
}

func NewProvider() caas.ContainerEnvironProvider {
	return kubernetesEnvironProvider{}
}
