// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package useragent

import (
	"github.com/Azure/go-autorest/autorest"

	"github.com/juju/juju/version"
)

// JujuPrefix returns the User-Agent prefix set by Juju.
func JujuPrefix() string {
	return "Juju/" + version.Current.String()
}

// UpdateClient updates the UserAgent field of the given autorest.Client.
func UpdateClient(client *autorest.Client) {
	if client.UserAgent == "" {
		client.UserAgent = JujuPrefix()
	} else {
		client.UserAgent = JujuPrefix() + " " + client.UserAgent
	}
}
