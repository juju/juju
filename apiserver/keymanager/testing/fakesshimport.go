// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"strings"

	"github.com/juju/utils/ssh/testing"
)

var multiOneDup = testing.ValidKeyFour.Key + "\n" + testing.ValidKeyTwo.Key

var importResponses = map[string]string{
	"lp:validuser":    testing.ValidKeyThree.Key,
	"lp:existing":     testing.ValidKeyTwo.Key,
	"lp:multi":        testing.ValidKeyMulti,
	"lp:multipartial": testing.PartValidKeyMulti,
	"lp:multiempty":   testing.EmptyKeyMulti,
	"lp:multiinvalid": testing.MultiInvalid,
	"lp:multionedup":  multiOneDup,
}

var FakeImport = func(keyId string) (string, error) {
	response, ok := importResponses[keyId]
	if ok {
		return strings.Join([]string{"INFO: line1", response, "INFO: line3"}, "\n"), nil
	}
	return "INFO: line", nil
}
