// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodel

import (
	"github.com/juju/names"
)

// ExportOffer prepares service endpoints for consumption.
func ExportOffer(offer Offer) error {
	// TODO(anastasiamac 2015-11-02) needs the actual implementation - this is a placeholder.
	// actual implementation will coordinate the work:
	// validate entities exist, access the service directory, write to state etc.
	TempPlaceholder[offer.Service] = offer
	return nil
}

// START TEMP IN-MEMORY PLACEHOLDER ///////////////

var TempPlaceholder = make(map[names.ServiceTag]Offer)

// END TEMP IN-MEMORY PLACEHOLDER ///////////////
