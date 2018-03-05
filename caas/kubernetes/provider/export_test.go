// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider

import "k8s.io/client-go/pkg/api/v1"

var (
	MakeUnitSpec = makeUnitSpec
)

func PodSpec(u *unitSpec) v1.PodSpec {
	return u.Pod
}
