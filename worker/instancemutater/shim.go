// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package instancemutater

import (
	"github.com/juju/juju/api/base"
	apiinstancemutater "github.com/juju/juju/api/agent/instancemutater"
)

func NewClient(apiCaller base.APICaller) InstanceMutaterAPI {
	facade := apiinstancemutater.NewClient(apiCaller)
	return facade
}
