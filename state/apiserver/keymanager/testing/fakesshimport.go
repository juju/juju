// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	sshtesting "launchpad.net/juju-core/utils/ssh/testing"
	"strings"
)

var importResponses = map[string]string{
	"lp:validuser": sshtesting.ValidKeyThree.Key,
	"lp:existing":  sshtesting.ValidKeyTwo.Key,
}

var FakeImport = func(keyId string) (string, error) {
	response, ok := importResponses[keyId]
	if ok {
		return strings.Join([]string{"INFO: line1", response, "INFO: line3"}, "\n"), nil
	}
	return "INFO: line", nil
}
