// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package vault

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/hashicorp/vault/api"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	vault "github.com/mittwald/vaultgo"
	"gopkg.in/yaml.v2"

	"github.com/juju/juju/core/secrets"
	"github.com/juju/juju/secrets/provider"
)

const (
	// Store is the name of the Kubernetes secrets store.
	Store = "vault"
)

// NewProvider returns a Kubernetes secrets provider.
func NewProvider() provider.SecretStoreProvider {
	return vaultProvider{}
}

type vaultProvider struct {
}

var logger = loggo.GetLogger("juju.secrets.vault")

// Initialise sets up a kv store mounted on the model uuid.
func (p vaultProvider) Initialise(m provider.Model) error {
	cfg, err := p.adminConfig(m)
	if err != nil {
		return errors.Trace(err)
	}
	client, err := p.newStore(cfg)
	if err != nil {
		return errors.Trace(err)
	}
	sys := client.client.Sys()
	ctx := context.Background()

	mounts, err := sys.ListMountsWithContext(ctx)
	if err != nil {
		return errors.Trace(err)
	}
	logger.Debugf("kv mounts: %v", mounts)
	modelUUID := cfg.Params["model-uuid"].(string)
	if _, ok := mounts[modelUUID]; !ok {
		err = sys.MountWithContext(ctx, modelUUID, &api.MountInput{
			Type:    "kv",
			Options: map[string]string{"version": "1"},
		})
		if err != nil {
			var apiErr *api.ResponseError
			if errors.As(err, &apiErr) {
				message := strings.Join(apiErr.Errors, ",")
				if apiErr.StatusCode != 400 || !strings.Contains(message, "path is already in use") {
					return errors.Trace(err)
				}
			}
		}
	}
	return nil
}

// CleanupSecrets removes policies associated with the removed secrets..
func (p vaultProvider) CleanupSecrets(m provider.Model, removed []*secrets.URI) error {
	cfg, err := p.adminConfig(m)
	if err != nil {
		return errors.Trace(err)
	}
	client, err := p.newStore(cfg)
	if err != nil {
		return errors.Trace(err)
	}
	sys := client.client.Sys()

	isRelevantPolicy := func(p string) bool {
		for _, r := range removed {
			if strings.HasPrefix(p, fmt.Sprintf("model-%s-%s-", m.UUID(), r.ID)) {
				return true
			}
		}
		return false
	}

	policies, err := sys.ListPolicies()
	if err != nil {
		return errors.Trace(err)
	}
	for _, p := range policies {
		if isRelevantPolicy(p) {
			if err := sys.DeletePolicy(p); err != nil {
				var apiErr *api.ResponseError
				if errors.As(err, &apiErr) {
					if apiErr.StatusCode == 404 {
						continue
					}
				}
				return errors.Annotatef(err, "deleting policy %q", p)
			}
		}
	}
	return nil
}

type vaultConfig struct {
	Endpoint      string   `yaml:"endpoint" json:"endpoint"`
	Namespace     string   `yaml:"namespace" json:"namespace"`
	Token         string   `yaml:"token" json:"token"`
	CACert        string   `yaml:"ca-cert" json:"ca-cert"`
	TLSServerName string   `yaml:"tls-server-name" json:"tls-server-name"`
	Keys          []string `yaml:"keys" json:"keys"`
}

// adminConfig returns the config needed to create a vault secrets store client
// with full admin rights.
func (p vaultProvider) adminConfig(m provider.Model) (*provider.StoreConfig, error) {
	cfg, err := m.Config()
	if err != nil {
		return nil, errors.Trace(err)
	}
	vaultCfgStr := cfg.SecretStoreConfig()
	if vaultCfgStr == "" {
		return nil, errors.NotValidf("empty vault config")
	}
	var vaultCfg vaultConfig
	if errJ := json.Unmarshal([]byte(vaultCfgStr), &vaultCfg); errJ != nil {
		if errY := yaml.Unmarshal([]byte(vaultCfgStr), &vaultConfig{}); errY != nil {
			return nil, errors.NewNotValid(errY, "invalid vault config")
		}
	}
	modelUUID := cfg.UUID()
	storeCfg := &provider.StoreConfig{
		StoreType: Store,
		Params: map[string]interface{}{
			"controller-uuid": m.ControllerUUID(),
			"model-uuid":      modelUUID,
			"endpoint":        vaultCfg.Endpoint,
			"namespace":       vaultCfg.Namespace,
			"token":           vaultCfg.Token,
			"ca-cert":         vaultCfg.CACert,
			"tls-server-name": vaultCfg.TLSServerName,
		},
	}
	// If keys are provided, we need to unseal the vault.
	// (If not, the vault needs to be unsealed already).
	if len(vaultCfg.Keys) == 0 {
		return storeCfg, nil
	}

	vaultClient, err := p.newStore(storeCfg)
	if err != nil {
		return nil, errors.Trace(err)
	}
	sys := vaultClient.client.Sys()
	for _, key := range vaultCfg.Keys {
		_, err := sys.Unseal(key)
		if err != nil {
			return nil, errors.Trace(err)
		}
	}

	return storeCfg, nil
}

// StoreConfig returns the config needed to create a vault secrets store client.
func (p vaultProvider) StoreConfig(m provider.Model, adminUser bool, owned []*secrets.URI, read []*secrets.URI) (*provider.StoreConfig, error) {
	// Get an admin store client so we can set up the policies.
	storeCfg, err := p.adminConfig(m)
	if err != nil {
		return nil, errors.Trace(err)
	}
	store, err := p.newStore(storeCfg)
	if err != nil {
		return nil, errors.Trace(err)
	}
	sys := store.client.Sys()

	modelUUID := m.UUID()
	var policies []string
	if adminUser {
		// For admin users, add secrets for the model can be read.
		rule := fmt.Sprintf(`path "%s/*" {capabilities = ["read"]}`, modelUUID)
		policyName := fmt.Sprintf("model-%s-read", modelUUID)
		err = sys.PutPolicy(policyName, rule)
		if err != nil {
			return nil, errors.Annotatef(err, "creating read policy for model %q", modelUUID)
		}
		policies = append(policies, policyName)
	} else {
		// Agents can create new secrets in the model.
		rule := fmt.Sprintf(`path "%s/*" {capabilities = ["create"]}`, modelUUID)
		policyName := fmt.Sprintf("model-%s-create", modelUUID)
		err = sys.PutPolicy(policyName, rule)
		if err != nil {
			return nil, errors.Annotatef(err, "creating create policy for model %q", modelUUID)
		}
		policies = append(policies, policyName)
	}
	// Any secrets owned by the agent can be updated/deleted etc.
	logger.Debugf("owned secrets: %#v", owned)
	for _, uri := range owned {
		rule := fmt.Sprintf(`path "%s/%s-*" {capabilities = ["create", "read", "update", "delete", "list"]}`, modelUUID, uri.ID)
		policyName := fmt.Sprintf("model-%s-%s-owner", modelUUID, uri.ID)
		err = sys.PutPolicy(policyName, rule)
		if err != nil {
			return nil, errors.Annotatef(err, "creating owner policy for %q", uri.ID)
		}
		policies = append(policies, policyName)
	}

	// Any secrets consumed by the agent can be read etc.
	logger.Debugf("consumed secrets: %#v", read)
	for _, uri := range read {
		rule := fmt.Sprintf(`path "%s/%s-*" {capabilities = ["read"]}`, modelUUID, uri.ID)
		policyName := fmt.Sprintf("model-%s-%s-read", modelUUID, uri.ID)
		err = sys.PutPolicy(policyName, rule)
		if err != nil {
			return nil, errors.Annotatef(err, "creating read policy for %q", uri.ID)
		}
		policies = append(policies, policyName)
	}
	s, err := store.client.Auth().Token().Create(&api.TokenCreateRequest{
		TTL:             "10m", // 10 minutes for now, can configure later.
		NoDefaultPolicy: true,
		Policies:        policies,
	})
	if err != nil {
		return nil, errors.Annotate(err, "creating secret access token")
	}
	storeCfg.Params["token"] = s.Auth.ClientToken

	return storeCfg, nil
}

// NewVaultClient is patched for testing.
var NewVaultClient = vault.NewClient

// NewStore returns a vault backed secrets store client.
func (p vaultProvider) NewStore(cfg *provider.StoreConfig) (provider.SecretsStore, error) {
	return p.newStore(cfg)
}

func (p vaultProvider) newStore(cfg *provider.StoreConfig) (*vaultStore, error) {
	modelUUID := cfg.Params["model-uuid"].(string)
	address := cfg.Params["endpoint"].(string)
	tlsConfig := vault.TLSConfig{
		TLSConfig: &api.TLSConfig{
			CACertBytes:   []byte(cfg.Params["ca-cert"].(string)),
			TLSServerName: cfg.Params["tls-server-name"].(string),
		},
	}
	c, err := NewVaultClient(address,
		&tlsConfig,
		vault.WithAuthToken(cfg.Params["token"].(string)),
	)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if ns := cfg.Params["namespace"].(string); ns != "" {
		c.SetNamespace(ns)
	}
	return &vaultStore{modelUUID: modelUUID, client: c}, nil
}

type vaultStore struct {
	modelUUID string
	client    *vault.Client
}

func secretPath(uri *secrets.URI, revision int) string {
	return fmt.Sprintf("%s-%d", uri.ID, revision)
}

// GetContent implements SecretsStore.
func (k vaultStore) GetContent(ctx context.Context, providerId string) (secrets.SecretValue, error) {
	s, err := k.client.KVv1(k.modelUUID).Get(ctx, providerId)
	if err != nil {
		return nil, errors.Annotatef(err, "getting secret %q", providerId)
	}
	val := make(map[string]string)
	for k, v := range s.Data {
		val[k] = fmt.Sprintf("%s", v)
	}
	return secrets.NewSecretValue(val), nil
}

// DeleteContent implements SecretsStore.
func (k vaultStore) DeleteContent(ctx context.Context, providerId string) error {
	return k.client.KVv1(k.modelUUID).Delete(ctx, providerId)
}

// SaveContent implements SecretsStore.
func (k vaultStore) SaveContent(ctx context.Context, uri *secrets.URI, revision int, value secrets.SecretValue) (string, error) {
	path := secretPath(uri, revision)
	val := make(map[string]interface{})
	for k, v := range value.EncodedValues() {
		val[k] = v
	}
	err := k.client.KVv1(k.modelUUID).Put(ctx, path, val)
	return path, errors.Annotatef(err, "saving secret content for %q", uri)
}
