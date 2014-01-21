// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import "reflect"

var (
	RootType          = reflect.TypeOf(&srvRoot{})
	NewPingTimeout    = newPingTimeout
	GetEnvironStorage = getEnvironStorage
	MaxPingInterval   = &maxPingInterval
)
