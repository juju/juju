// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmstore

import (
	"gopkg.in/juju/charm.v6"
	csparams "gopkg.in/juju/charmrepo.v4/csclient/params"
)

// CharmID encapsulates data for identifying a
// unique charm from the charm store.
type CharmID struct {
	// URL is the url of the charm.
	URL *charm.URL

	// Channel is the channel in which the charm was published.
	Channel csparams.Channel

	// Metadata is optional extra information about a particular model's
	// "in-theatre" use use of the charm.
	Metadata map[string]string
}
