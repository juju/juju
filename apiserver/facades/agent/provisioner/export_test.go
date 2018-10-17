// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provisioner

import "github.com/juju/juju/apiserver/params"

func NewPrepareOrGetContext(result params.MachineNetworkConfigResults, maintain bool) *prepareOrGetContext {
	return &prepareOrGetContext{result: result, maintain: maintain}
}
