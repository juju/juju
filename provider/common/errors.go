// Copyright 2012, 2013, 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import "errors"

// An error indicating that the provider can't allocate another IP address to
// an instance.
var IPAddressesExhausted = errors.New("Can't allocate a new IP address")
