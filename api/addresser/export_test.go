// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package addresser

import (
	"github.com/juju/names"

	"github.com/juju/juju/apiserver/params"
)

func NewIPAddress(api *API, tag names.IPAddressTag, life params.Life) *IPAddress {
	return &IPAddress{api.facade, tag, life}
}

var NewEntityWatcher = &newEntityWatcher
