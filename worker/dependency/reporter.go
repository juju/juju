// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package dependency

// Reporter defines an interface that can be used to get reports
// from types that implement this interface.
// A report is just a map of values that might be of interest.
// The primary use case is status reports
type Reporter interface {
	Report() map[string]interface{}
}
