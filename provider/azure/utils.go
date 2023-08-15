// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package azure

import (
	"math/rand"

	azurecloud "github.com/Azure/azure-sdk-for-go/sdk/azcore/cloud"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armresources"
	"github.com/juju/errors"
	"github.com/juju/utils/v3"

	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/provider/azure/internal/errorutils"
)

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

// collectAPIVersions returns a map of the latest API version for each
// possible resource type. This is needed to use the Azure Resource
// Management API, because the API version requested must match the
// type of the resource being manipulated through the API, rather than
// the API version specified statically in the resource client code.
func collectAPIVersions(ctx context.ProviderCallContext, client *armresources.ProvidersClient) (map[string]string, error) {
	result := make(map[string]string)

	res := client.NewListPager(nil)
	for res.More() {
		p, err := res.NextPage(ctx)
		if err != nil {
			return map[string]string{}, errorutils.HandleCredentialError(errors.Trace(err), ctx)
		}

		providers := p.ProviderListResult
		for _, provider := range providers.Value {
			for _, resourceType := range provider.ResourceTypes {
				key := toValue(provider.Namespace) + "/" + toValue(resourceType.ResourceType)
				if len(resourceType.APIVersions) == 0 {
					continue
				}
				// The versions are newest-first.
				result[key] = toValue(resourceType.APIVersions[0])
			}
		}
	}
	return result, nil
}

func azureCloud(cloudName, apiEndpoint, identityEndpoint string) azurecloud.Configuration {
	// Use well known cloud definitions from the SDk if possible.
	switch cloudName {
	case "azure":
		return azurecloud.AzurePublic
	case "azure-china":
		return azurecloud.AzureChina
	case "azure-gov":
		return azurecloud.AzureGovernment
	}
	return azurecloud.Configuration{
		ActiveDirectoryAuthorityHost: identityEndpoint,
		Services: map[azurecloud.ServiceName]azurecloud.ServiceConfiguration{
			azurecloud.ResourceManager: {
				Audience: "https://management.core.windows.net/",
				Endpoint: apiEndpoint,
			},
		},
	}
}

func toValue[T any](v *T) T {
	if v == nil {
		return *new(T)
	}
	return *v
}

func toMap(in map[string]*string) map[string]string {
	result := make(map[string]string)
	for k, v := range in {
		result[k] = toValue(v)
	}
	return result
}

func toMapPtr(in map[string]string) map[string]*string {
	result := make(map[string]*string)
	for k, v := range in {
		result[k] = to.Ptr(v)
	}
	return result
}
