// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import "os"

// Signals is the list of operating system signals this suite will capture
var Signals = []os.Signal{
	os.Interrupt,
	os.Kill,
}
