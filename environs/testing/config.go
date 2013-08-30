// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/testing"
)

// FakeConfig holds a set of attributes just sufficient to create a
// new config.Config without defaults, but not corresponding with any
// actual provider.
var FakeConfig = testing.Attrs{
	"type":                      "someprovider",
	"name":                      "testenv",
	"authorized-keys":           "my-keys",
	"firewall-mode":             config.FwInstance,
	"admin-secret":              "fish",
	"ca-cert":                   testing.CACert,
	"ca-private-key":	testing.CAKey,
	"ssl-hostname-verification": true,
	"development":               false,
	"state-port":                1234,
	"api-port":                  4321,
	"default-series":            "precise",
}
