// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package exec

import (
	typedcorev1 "k8s.io/client-go/kubernetes/typed/core/v1"
)

var (
	ProcessEnv = processEnv
)

func (ep *ExecParams) Validate(podGetter typedcorev1.PodInterface) error {
	return ep.validate(podGetter)
}
