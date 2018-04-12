// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package azure

import (
	"math/rand"
	"net/http"
	"time"

	"github.com/Azure/azure-sdk-for-go/arm/resources/resources"
	"github.com/Azure/go-autorest/autorest"
	"github.com/Azure/go-autorest/autorest/to"
	"github.com/juju/errors"
	"github.com/juju/utils"
)

const (
	retryDelay       = 5 * time.Second
	maxRetryDelay    = 1 * time.Minute
	maxRetryDuration = 5 * time.Minute
)

func toTags(tags *map[string]*string) map[string]string {
	if tags == nil {
		return nil
	}
	return to.StringMap(*tags)
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

func isNotFoundResponse(resp autorest.Response) bool {
	if resp.Response != nil && resp.StatusCode == http.StatusNotFound {
		return true
	}
	return false
}

// collectAPIVersions returns a map of the latest API version for each
// possible resource type. This is needed to use the Azure Resource
// Management API, because the API version requested must match the
// type of the resource being manipulated through the API, rather than
// the API version specified statically in the resource client code.
func collectAPIVersions(client resources.ProvidersClient) (map[string]string, error) {
	result := make(map[string]string)

	var res resources.ProviderListResult
	res, err := client.List(nil, "")
	if err != nil {
		return result, errors.Trace(err)
	}
	for res.Value != nil {
		for _, provider := range *res.Value {
			if provider.ResourceTypes == nil {
				continue
			}
			for _, resourceType := range *provider.ResourceTypes {
				key := to.String(provider.Namespace) + "/" + to.String(resourceType.ResourceType)
				versions := to.StringSlice(resourceType.APIVersions)
				if len(versions) == 0 {
					continue
				}
				// The versions are newest-first.
				result[key] = versions[0]
			}
		}
		res, err = client.ListNextResults(res)
		if err != nil {
			return map[string]string{}, errors.Trace(err)
		}
	}
	return result, nil
}
