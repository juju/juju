// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package azure

import (
	"math/rand"

	"github.com/Azure/azure-sdk-for-go/Godeps/_workspace/src/github.com/Azure/go-autorest/autorest/to"
	"github.com/juju/utils"
)

func toTagsPtr(tags map[string]string) *map[string]*string {
	stringPtrMap := to.StringMapPtr(tags)
	return &stringPtrMap
}

func toTags(tags *map[string]*string) map[string]string {
	if tags == nil {
		return nil
	}
	return to.StringMap(*tags)
}

func toStringSlicePtr(s ...string) *[]string {
	return &s
}

// randomAdminPassword returns a random administrator password for
// Windows machines.
func randomAdminPassword() string {
	// We want at least one each of lower-alpha, upper-alpha, and digit.
	// Allocate 16 of each (randomly), and then the remaining characters
	// will be randomly chosen from the full set.
	validRunes := append(utils.LowerAlpha, utils.Digits...)
	validRunes = append(validRunes, utils.UpperAlpha...)

	lowerAlpha := utils.RandomString(16, utils.LowerAlpha)
	upperAlpha := utils.RandomString(16, utils.UpperAlpha)
	digits := utils.RandomString(16, utils.Digits)
	mixed := utils.RandomString(16, validRunes)
	password := []rune(lowerAlpha + upperAlpha + digits + mixed)
	for i := len(password) - 1; i >= 1; i-- {
		j := rand.Intn(i + 1)
		password[i], password[j] = password[j], password[i]
	}
	return string(password)
}
