// Copyright 2012-2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package environ

var LoadedInvalid = make(chan struct{})

func init() {
	loadedInvalid = func() {
		LoadedInvalid <- struct{}{}
	}
}
