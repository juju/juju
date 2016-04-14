package charmstore

import (
	"gopkg.in/juju/charm.v6-unstable"
	csparams "gopkg.in/juju/charmrepo.v2-unstable/csclient/params"
)

// CharmID is a type that encapsulates all the data required to interact with a
// unique charm from the charmstore.
type CharmID struct {
	// URL is the url of the charm.
	URL *charm.URL

	// Channel is the channel in which the charm was published.
	Channel csparams.Channel
}
