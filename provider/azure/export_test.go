// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package azure

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/Azure/go-autorest/autorest/adal"

	"github.com/juju/juju/environs"
	"github.com/juju/juju/provider/azure/internal/azureauth"
	"github.com/juju/juju/storage"
)

func ForceVolumeSourceTokenRefresh(vs storage.VolumeSource) error {
	return ForceTokenRefresh(vs.(*azureVolumeSource).env)
}

func ForceTokenRefresh(env environs.Environ) error {
	auth := env.(*azureEnviron).authorizer
	resourceID, _ := azureauth.ResourceManagerResourceId(auth.cloud.StorageEndpoint)
	token, ok := auth.tokens[resourceID]
	if ok {
		token.SetCustomRefreshFunc(func(ctx context.Context, resource string) (*adal.Token, error) {
			return &adal.Token{
				AccessToken: "access-token",
				ExpiresOn:   json.Number(fmt.Sprint(time.Now().Add(time.Hour).Unix())),
				Type:        "Bearer",
			}, nil
		})
		auth.tokens[resourceID] = token
	}
	return env.(*azureEnviron).authorizer.refreshToken("")
}

func SetRetries(env environs.Environ) {
	azureEnv := env.(*azureEnviron)
	azureEnv.setClientRetries(1, 0)
}
