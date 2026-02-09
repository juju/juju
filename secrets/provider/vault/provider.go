// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package vault

import (
	"context"
	"fmt"
	"math"
	"os"
	"strings"
	"time"

	"github.com/hashicorp/vault/api"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names/v5"
	vault "github.com/mittwald/vaultgo"

	coresecrets "github.com/juju/juju/core/secrets"
	"github.com/juju/juju/secrets/provider"
)

var logger = loggo.GetLogger("juju.secrets.vault")

const (
	// BackendType is the type of the Vault secrets backend.
	BackendType = "vault"
)

// NewProvider returns a Vault secrets provider.
func NewProvider() provider.SecretBackendProvider {
	return vaultProvider{}
}

type vaultProvider struct {
}

func (p vaultProvider) Type() string {
	return BackendType
}

func modelPathPrefix(name, modelUUID string) string {
	if name == "" || modelUUID == "" {
		return ""
	}
	suffix := modelUUID[len(modelUUID)-6:]
	return name + "-" + suffix
}

// Initialise sets up a kv store mounted on the model uuid.
func (p vaultProvider) Initialise(cfg *provider.ModelBackendConfig) error {
	modelUUID := cfg.ModelUUID
	modelPath := modelPathPrefix(cfg.ModelName, modelUUID)
	client, err := p.newBackend(modelPath, &cfg.BackendConfig)
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
	mountPath := client.mountPath
	if _, ok := mounts[mountPath+"/"]; ok {
		return nil
	}

	// Rename any legacy mounts which use the model uuid.
	if _, ok := mounts[modelUUID+"/"]; ok {
		err = sys.RemountWithContext(ctx, modelUUID, mountPath)
		if err != nil && !isMountNotFound(err) {
			return errors.Trace(err)
		}
	}
	err = sys.MountWithContext(ctx, mountPath, &api.MountInput{
		Type:    "kv",
		Options: map[string]string{"version": "1"},
	})
	if !isAlreadyExists(err, "path is already in use") {
		return errors.Trace(err)
	}
	return nil
}

// CleanupModel deletes all secrets and policies associated with the model.
func (p vaultProvider) CleanupModel(cfg *provider.ModelBackendConfig) (err error) {
	defer func() {
		if err == nil {
			return
		} else if strings.HasSuffix(err.Error(), "no route to host") ||
			strings.HasSuffix(err.Error(), "connection refused") {
			// There is nothing we can do now, so just log the error and continue.
			logger.Warningf("failed to cleanup secrets for model %q: %v", cfg.ModelUUID, err)
			err = nil
		}
	}()

	modelPath := modelPathPrefix(cfg.ModelName, cfg.ModelUUID)
	k, err := p.newBackend(modelPath, &cfg.BackendConfig)
	if err != nil {
		return errors.Trace(err)
	}
	sys := k.client.Sys()

	// First remove any policies.
	ctx := context.Background()
	policies, err := sys.ListPoliciesWithContext(ctx)
	if err != nil {
		return errors.Trace(err)
	}
	for _, p := range policies {
		// TODO(juju4) - remove legacy mount point
		if strings.HasPrefix(p, modelPath) || strings.HasPrefix(p, "model-"+cfg.ModelUUID) {
			if err := sys.DeletePolicyWithContext(ctx, p); err != nil {
				if isNotFound(err) {
					continue
				}
				return errors.Annotatef(err, "deleting policy %q", p)
			}
		}
	}

	// Now remove any secrets.
	s, err := k.client.Logical().ListWithContext(ctx, k.mountPath)
	if err != nil {
		return errors.Trace(err)
	}
	if s == nil || s.Data == nil {
		return nil
	}
	keys, ok := s.Data["keys"].([]interface{})
	if !ok {
		return nil
	}
	for _, id := range keys {
		err = k.client.KVv1(k.mountPath).Delete(ctx, fmt.Sprintf("%s", id))
		if err != nil && !isNotFound(err) {
			return errors.Annotatef(err, "deleting secret %q", id)
		}
	}
	return sys.UnmountWithContext(ctx, k.mountPath)
}

// CleanupSecrets removes policies associated with the removed secrets.
func (p vaultProvider) CleanupSecrets(cfg *provider.ModelBackendConfig, tag names.Tag, removed provider.SecretRevisions) error {
	modelPath := modelPathPrefix(cfg.ModelName, cfg.ModelUUID)
	client, err := p.newBackend(modelPath, &cfg.BackendConfig)
	if err != nil {
		return errors.Trace(err)
	}
	sys := client.client.Sys()

	isRelevantPolicy := func(p string) bool {
		for id := range removed {
			if strings.HasPrefix(p, fmt.Sprintf("%s-%s-", modelPath, id)) {
				return true
			}
			// TODO(juju4) - remove legacy mount point
			if strings.HasPrefix(p, fmt.Sprintf("model-%s-%s-", cfg.ModelUUID, id)) {
				return true
			}
		}
		return false
	}

	ctx := context.Background()
	policies, err := sys.ListPoliciesWithContext(ctx)
	if err != nil {
		return errors.Trace(err)
	}
	for _, p := range policies {
		if isRelevantPolicy(p) {
			if err := sys.DeletePolicyWithContext(ctx, p); err != nil {
				if isNotFound(err) {
					continue
				}
				return errors.Annotatef(err, "deleting policy %q", p)
			}
		}
	}
	return nil
}

// IssuesTokens returns true if this secret backend provider needs to issue
// a token to provide a restricted (delegated) config.
func (p vaultProvider) IssuesTokens() bool {
	return true
}

// CleanupIssuedTokens removes all ACLs/tokens related to the given issued
// token UUIDs. It returns, even during error, the list of tokens it revoked
// so far.
func (p vaultProvider) CleanupIssuedTokens(
	adminCfg *provider.ModelBackendConfig, issuedTokenUUIDs []string,
) ([]string, error) {
	// Get an admin backend client so we can set up the policies.
	mountPath := modelPathPrefix(adminCfg.ModelName, adminCfg.ModelUUID)
	backend, err := p.newBackend(mountPath, &adminCfg.BackendConfig)
	if err != nil {
		return nil, errors.Trace(err)
	}
	sys := backend.client.Sys()
	ctx := context.TODO()

	for i, issuedTokenUUID := range issuedTokenUUIDs {
		policyName := fmt.Sprintf("%s-%s", mountPath, issuedTokenUUID)
		err := sys.DeletePolicyWithContext(ctx, policyName)
		if err != nil && !isNotFound(err) {
			// return the tokens deleted so far.
			return issuedTokenUUIDs[:i], errors.New(
				"removing vault secret backend issued tokens",
			)
		}
	}

	return issuedTokenUUIDs, nil
}

// RestrictedConfig returns the config needed to create a
// secrets backend client restricted to manage the specified
// owned secrets and read shared secrets for the given entity tag.
func (p vaultProvider) RestrictedConfig(
	adminCfg *provider.ModelBackendConfig,
	sameController, forDrain bool,
	issuedTokenUUID string,
	consumer names.Tag,
	owned []string,
	ownedRevs provider.SecretRevisions,
	readRevs provider.SecretRevisions,
) (*provider.BackendConfig, error) {
	adminUser := consumer == nil
	// Get an admin backend client so we can set up the policies.
	modelPath := modelPathPrefix(adminCfg.ModelName, adminCfg.ModelUUID)
	backend, err := p.newBackend(modelPath, &adminCfg.BackendConfig)
	if err != nil {
		return nil, errors.Trace(err)
	}
	mountPath := backend.mountPath
	sys := backend.client.Sys()
	ctx := context.TODO()

	var rules []string
	if forDrain && (adminUser || consumer.Kind() == names.ModelTagKind) {
		// For controller drain worker, we need to be able to update a secret.
		// Because we may run into a situation that the worker creates a secret in the vault but gets killed/restarted
		// before it can update the secret to the new backend, we need to allow the worker to update the content
		// after it's coming up again.
		rule := fmt.Sprintf(`path "%s/*" {capabilities = ["update"]}`, mountPath)
		rules = append(rules, rule)
	}
	if adminUser {
		// For admin users, all secrets for the model can be read.
		rule := fmt.Sprintf(`path "%s/*" {capabilities = ["read"]}`, mountPath)
		rules = append(rules, rule)
	}

	// Any secrets owned by the agent can be updated/deleted etc.
	logger.Debugf("owned secrets: %#v", owned)
	for _, id := range owned {
		rule := fmt.Sprintf(`path "%s/%s-*" {capabilities = ["create", "read", "update", "delete", "list"]}`, mountPath, id)
		rules = append(rules, rule)
	}

	// Any secrets consumed by the agent can be read etc.
	logger.Debugf("consumed secret revisions: %#v", readRevs)
	for _, revs := range readRevs {
		for _, revId := range revs.Values() {
			rule := fmt.Sprintf(`path "%s/%s" {capabilities = ["read"]}`, mountPath, revId)
			rules = append(rules, rule)
		}
	}

	policyName := fmt.Sprintf("%s-%s", mountPath, issuedTokenUUID)
	err = sys.PutPolicyWithContext(ctx, policyName, strings.Join(rules, "\n"))
	if err != nil {
		return nil, errors.Annotatef(err, "creating policy %q", policyName)
	}
	logger.Tracef("policy rules for %q: %#v", policyName, rules)

	ttl := fmt.Sprintf(
		"%dm", int(math.Ceil(coresecrets.IssuedTokenValidity.Minutes())),
	)
	s, err := backend.client.Auth().Token().Create(&api.TokenCreateRequest{
		TTL:             ttl,
		NoDefaultPolicy: true,
		Policies:        []string{policyName},
		Metadata: map[string]string{
			"juju-issued-token-uuid": issuedTokenUUID,
		},
	})
	if err != nil {
		return nil, errors.Annotate(err, "creating secret access token")
	}

	cfg := adminCfg.BackendConfig
	cfg.Config[TokenKey] = s.Auth.ClientToken
	return &cfg, nil
}

// NewVaultClient is patched for testing.
var NewVaultClient = vault.NewClient

// NewBackend returns a vault backed secrets backend client.
func (p vaultProvider) NewBackend(cfg *provider.ModelBackendConfig) (provider.SecretsBackend, error) {
	return p.newBackend(modelPathPrefix(cfg.ModelName, cfg.ModelUUID), &cfg.BackendConfig)
}

func (p vaultProvider) newBackend(modelPathPrefix string, cfg *provider.BackendConfig) (*vaultBackend, error) {
	backend, err := p.newBackendNoMount(cfg)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if backend.mountPath != "" && !strings.HasSuffix(backend.mountPath, "/") {
		backend.mountPath += "/"
	}
	backend.mountPath += modelPathPrefix
	return backend, nil
}

func (p vaultProvider) newBackendNoMount(cfg *provider.BackendConfig) (*vaultBackend, error) {
	validCfg, err := newConfig(cfg.Config)
	if err != nil {
		return nil, errors.Annotatef(err, "invalid vault config")
	}

	var clientCertPath, clientKeyPath string
	clientCert := validCfg.clientCert()
	clientKey := validCfg.clientKey()
	if clientCert != "" {
		clientCertFile, err := os.CreateTemp("", "client-cert")
		if err != nil {
			return nil, errors.Annotate(err, "creating client cert file")
		}
		defer func() { _ = clientCertFile.Close() }()
		clientCertPath = clientCertFile.Name()
		if _, err := clientCertFile.Write([]byte(clientCert)); err != nil {
			return nil, errors.Annotate(err, "writing client cert file")
		}

		clientKeyFile, err := os.CreateTemp("", "client-key")
		if err != nil {
			return nil, errors.Annotate(err, "creating client key file")
		}
		defer func() { _ = clientKeyFile.Close() }()
		clientKeyPath = clientKeyFile.Name()
		if _, err := clientKeyFile.Write([]byte(clientKey)); err != nil {
			return nil, errors.Annotate(err, "writing client key file")
		}
	}

	tlsConfig := vault.TLSConfig{
		TLSConfig: &api.TLSConfig{
			CACertBytes:   []byte(validCfg.caCert()),
			ClientCert:    clientCertPath,
			ClientKey:     clientKeyPath,
			TLSServerName: validCfg.tlsServerName(),
		},
	}
	c, err := NewVaultClient(validCfg.endpoint(),
		&tlsConfig,
		vault.WithAuthToken(validCfg.token()),
	)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if ns := validCfg.namespace(); ns != "" {
		c.SetNamespace(ns)
	}
	return &vaultBackend{
		client:    c,
		mountPath: validCfg.mountPath(),
	}, nil
}

// RefreshAuth implements SupportAuthRefresh.
func (p vaultProvider) RefreshAuth(adminCfg *provider.ModelBackendConfig, validFor time.Duration) (_ *provider.BackendConfig, err error) {
	defer func() {
		err = maybePermissionDenied(err)
	}()

	backend, err := p.newBackendNoMount(&adminCfg.BackendConfig)
	if err != nil {
		return nil, errors.Trace(err)
	}
	validForSeconds := validFor.Truncate(time.Second).Seconds()
	s, err := backend.client.Auth().Token().Create(&api.TokenCreateRequest{
		TTL:      fmt.Sprintf("%ds", int(validForSeconds)),
		NoParent: true,
	})
	if err != nil {
		return nil, errors.Annotate(err, "creating new auth token")
	}

	tok, err := s.TokenID()
	if err != nil {
		return nil, errors.Annotate(err, "extracting new auth token")
	}
	backend.client.SetToken(tok)
	cfgCopy := adminCfg.BackendConfig
	cfgCopy.Config[TokenKey] = tok
	return &cfgCopy, nil
}
