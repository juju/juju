// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"github.com/juju/juju/apiserver/params"
)

// Settings is a test double for jujuc.Settings.
type Settings params.Settings

// Get implements jujuc.Settings.
func (s Settings) Get(k string) (interface{}, bool) {
	v, f := s[k]
	return v, f
}

// Set implements jujuc.Settings.
func (s Settings) Set(k, v string) {
	s[k] = v
}

// Delete implements jujuc.Settings.
func (s Settings) Delete(k string) {
	delete(s, k)
}

// Map implements jujuc.Settings.
func (s Settings) Map() params.Settings {
	r := params.Settings{}
	for k, v := range s {
		r[k] = v
	}
	return r
}
