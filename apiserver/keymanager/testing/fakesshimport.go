// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"strings"

	sshtesting "github.com/juju/juju/utils/ssh/testing"
)

var multiOneDup = sshtesting.ValidKeyFour.Key + "\n" + sshtesting.ValidKeyTwo.Key

var importResponses = map[string]string{
	"lp:validuser":    sshtesting.ValidKeyThree.Key,
	"lp:existing":     sshtesting.ValidKeyTwo.Key,
	"lp:multi":        sshtesting.ValidKeyMulti,
	"lp:multipartial": sshtesting.PartValidKeyMulti,
	"lp:multiempty":   sshtesting.EmptyKeyMulti,
	"lp:multiinvalid": sshtesting.MultiInvalid,
	"lp:multionedup":  multiOneDup,
}

var FakeImport = func(keyId string) (string, error) {
	response, ok := importResponses[keyId]
	if ok {
		return strings.Join([]string{"INFO: line1", response, "INFO: line3"}, "\n"), nil
	}
	return "INFO: line", nil
}
