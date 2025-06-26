// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodelsecrets

import (
	"context"
	"encoding/json"

	"github.com/juju/errors"

	k8scloud "github.com/juju/juju/caas/kubernetes/cloud"
	"github.com/juju/juju/cloud"
	"github.com/juju/juju/internal/secrets/provider/kubernetes"
	"github.com/juju/juju/rpc/params"
)

// CrossModelSecretsAPIV1 provides access to the CrossModelSecrets API V1 facade.
type CrossModelSecretsAPIV1 struct {
	*CrossModelSecretsAPI
}

// CrossModelSecretsAPI provides access to the CrossModelSecrets API facade.
type CrossModelSecretsAPI struct{}

// NewCrossModelSecretsAPI returns a new server-side CrossModelSecretsAPI facade.
func NewCrossModelSecretsAPI() (*CrossModelSecretsAPI, error) {
	return &CrossModelSecretsAPI{}, nil
}

// GetSecretAccessScope returns the tokens for the access scope of the specified secrets and consumers.
func (s *CrossModelSecretsAPI) GetSecretAccessScope(ctx context.Context, args params.GetRemoteSecretAccessArgs) (params.StringResults, error) {
	return params.StringResults{}, nil
}

// marshallLegacyBackendConfig converts the supplied backend config
// so it is suitable for older juju agents.
func marshallLegacyBackendConfig(cfg params.SecretBackendConfig) error {
	if cfg.BackendType != kubernetes.BackendType {
		return nil
	}
	if _, ok := cfg.Params["credential"]; ok {
		return nil
	}
	token, ok := cfg.Params["token"].(string)
	if !ok {
		return nil
	}
	delete(cfg.Params, "token")
	delete(cfg.Params, "namespace")
	delete(cfg.Params, "prefer-incluster-address")

	cred := cloud.NewCredential(cloud.OAuth2AuthType, map[string]string{k8scloud.CredAttrToken: token})
	credData, err := json.Marshal(cred)
	if err != nil {
		return errors.Annotatef(err, "error marshalling backend config")
	}
	cfg.Params["credential"] = string(credData)
	cfg.Params["is-controller-cloud"] = false
	return nil
}

// GetSecretContentInfo returns the secret values for the specified secrets.
func (s *CrossModelSecretsAPIV1) GetSecretContentInfo(ctx context.Context, args params.GetRemoteSecretContentArgs) (params.SecretContentResults, error) {
	results, err := s.CrossModelSecretsAPI.GetSecretContentInfo(ctx, args)
	if err != nil {
		return params.SecretContentResults{}, errors.Trace(err)
	}
	for i, cfg := range results.Results {
		if cfg.BackendConfig == nil {
			continue
		}
		if err := marshallLegacyBackendConfig(cfg.BackendConfig.Config); err != nil {
			return params.SecretContentResults{}, errors.Annotatef(err, "marshalling legacy backend config")
		}
		results.Results[i] = cfg
	}
	return results, nil
}

// GetSecretContentInfo returns the secret values for the specified secrets.
func (s *CrossModelSecretsAPI) GetSecretContentInfo(ctx context.Context, args params.GetRemoteSecretContentArgs) (params.SecretContentResults, error) {
	return params.SecretContentResults{}, nil
}
