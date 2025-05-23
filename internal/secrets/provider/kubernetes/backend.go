// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package kubernetes

import (
	"context"
	"encoding/base64"
	"fmt"

	"github.com/juju/errors"
	core "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/juju/juju/core/secrets"
	secreterrors "github.com/juju/juju/domain/secret/errors"
	"github.com/juju/juju/internal/provider/kubernetes/resources"
)

type k8sBackend struct {
	serviceAccount string
	namespace      string
	modelName      string
	modelUUID      string

	client kubernetes.Interface
}

// Ping implements SecretsBackend.
func (k *k8sBackend) Ping() (err error) {
	defer func() {
		err = errors.Annotatef(err, "backend not reachable")
	}()
	_, err = k.client.Discovery().ServerVersion()
	if err != nil {
		return errors.Trace(err)
	}
	_, err = k.client.CoreV1().Namespaces().Get(context.Background(), k.namespace, v1.GetOptions{})
	if err != nil {
		return errors.Annotatef(err, "checking secrets namespace")
	}
	if k.serviceAccount != "" {
		_, err = k.client.CoreV1().ServiceAccounts(k.namespace).Get(context.Background(), k.serviceAccount, v1.GetOptions{})
		if err != nil {
			return errors.Annotatef(err, "checking secrets service account")
		}
	}
	return nil
}

// getSecret returns a secret resource.
func (k *k8sBackend) getSecret(ctx context.Context, secretName string) (*core.Secret, error) {
	if k.namespace == "" {
		return nil, errNoNamespace
	}
	secret, err := k.client.CoreV1().Secrets(k.namespace).Get(ctx, secretName, v1.GetOptions{})
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return nil, fmt.Errorf("secret %q not found%w", secretName, errors.Hide(secreterrors.SecretRevisionNotFound))
		} else if k8serrors.IsForbidden(err) {
			return nil, errors.Unauthorizedf("cannot access %q", secretName)
		}
		return nil, errors.Trace(err)
	}
	return secret, nil
}

// GetContent implements SecretBackend.
func (k *k8sBackend) GetContent(ctx context.Context, revisionId string) (_ secrets.SecretValue, err error) {
	defer func() {
		err = maybePermissionDenied(err)
	}()

	// revisionId is the secret name.
	secret, err := k.getSecret(ctx, revisionId)
	if err != nil {
		logger.Tracef(context.TODO(), "getting secret %q: %v", revisionId, err)
		return nil, errors.Trace(err)
	}
	data := map[string]string{}
	for k, v := range secret.Data {
		data[k] = base64.StdEncoding.EncodeToString(v)
	}
	return secrets.NewSecretValue(data), nil
}

// SaveContent implements SecretBackend.
func (k *k8sBackend) SaveContent(ctx context.Context, uri *secrets.URI, revision int, value secrets.SecretValue) (_ string, err error) {
	defer func() {
		err = maybePermissionDenied(err)
	}()

	name := uri.Name(revision)
	labels := labelsForSecretRevision(k.modelName, k.modelUUID)
	in := &core.Secret{
		ObjectMeta: v1.ObjectMeta{
			Labels: labels,
		},
		Type: core.SecretTypeOpaque,
	}
	if in.StringData, err = value.Values(); err != nil {
		return "", errors.Trace(err)
	}
	secret := resources.NewSecret(name, k.namespace, in)
	if err = secret.Apply(ctx, k.client); err != nil {
		return "", errors.Trace(err)
	}
	return name, nil
}

// DeleteContent implements SecretBackend.
func (k *k8sBackend) DeleteContent(ctx context.Context, revisionId string) (err error) {
	defer func() {
		err = maybePermissionDenied(err)
	}()

	// revisionId is the secret name.
	secret, err := k.getSecret(ctx, revisionId)
	if err != nil {
		logger.Tracef(context.TODO(), "deleting secret %q: %v", revisionId, err)
		return errors.Trace(err)
	}
	return resources.NewSecret(secret.Name, k.namespace, secret).Delete(ctx, k.client)
}
