// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"gopkg.in/juju/charmrepo.v3/csclient"
	"gopkg.in/juju/charmrepo.v3/csclient/params"
)

type charmstoreClientShim struct {
	*csclient.Client
}

func (c charmstoreClientShim) WithChannel(channel params.Channel) charmstoreForDeploy {
	client := c.Client.WithChannel(channel)
	return charmstoreClientShim{client}
}
