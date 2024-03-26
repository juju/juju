// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/utils/v3/ssh"

	"github.com/juju/juju/environs/config"
)

// ControllerModelConfigService represents a slimmed down model config service
// for providing access to the controller models configuration.
type ControllerModelConfigService interface {
	ModelConfig(context.Context) (*config.Config, error)
}

// SystemSSHKeyProvider is responsible for providing the controllers system ssh key that
// gets added to every model as a default. If no system ssh key is found then a
// error satisfying [errors.NotFound] will be returned.
type SystemSSHKeyProvider = func(context.Context) (string, error)

// NewSystemSSHKeyProvider returns a [SystemSSHKeyProvider] that will return the
// set system ssh key for the controller. The provider finds the key in the
// controller mo
func NewSystemSSHKeyProvider(
	configService ControllerModelConfigService,
) SystemSSHKeyProvider {
	return func(ctx context.Context) (string, error) {
		conf, err := configService.ModelConfig(ctx)
		if err != nil {
			return "", fmt.Errorf("getting controller model configuration to extract system ssh keys from: %w", err)
		}

		for _, key := range ssh.SplitAuthorisedKeys(conf.AuthorizedKeys()) {
			parsedKey, err := ssh.ParseAuthorisedKey(key)
			if err != nil {
				return "", fmt.Errorf("parsing ssh keys from controller config, finding system ssh key: %w", err)
			}

			if parsedKey.Comment == config.JujuSystemKey {
				return key, nil
			}
		}

		return "", fmt.Errorf("system ssh key %w in controller model configuration", errors.NotFound)
	}
}
