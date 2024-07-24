// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import "context"

// GetSecretsForExport returns a result containing all the information needed to
// export secrets to a model description.
func (s *SecretService) GetSecretsForExport(ctx context.Context) (*SecretExport, error) {
	// TODO(secrets)
	return &SecretExport{}, nil
}

// ImportSecrets saves the supplied secret details to the model.
func (s *SecretService) ImportSecrets(context.Context, *SecretExport) error {
	// TODO(secrets)
	return nil
}
