// Copyright 2012, 2013, 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import "errors"

// Errors indicating that the provider can't allocate an IP address to an
// instance.
var ErrIPAddressesExhausted = errors.New("can't allocate a new IP address")
var ErrIPAddressUnvailable = errors.New("the requested IP address is unavailable")
