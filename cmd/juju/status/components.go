// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package status

// RegisterUnitStatusFormatter registers a function that returns a
// value that will be used to deserialize the status value for the given
// component.
func RegisterUnitStatusFormatter(component string, fn func([]byte) interface{}) {
	if _, ok := unitStatusFormatters[component]; ok {
		panic("Component " + component + " already registered!")
	}
	unitStatusFormatters[component] = fn
}

var unitStatusFormatters = map[string]func([]byte) interface{}{}
