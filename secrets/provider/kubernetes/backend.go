// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package kubernetes

import (
	"context"
	"encoding/base64"

	"github.com/juju/errors"
	core "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/juju/juju/caas/kubernetes/provider/resources"
	"github.com/juju/juju/caas/kubernetes/provider/utils"
	"github.com/juju/juju/core/secrets"
)

type k8sBackend struct {
	namespace string
	model     string

	client kubernetes.Interface
}

// Ping implements SecretsBackend.
func (k *k8sBackend) Ping() error {
	_, err := k.client.Discovery().ServerVersion()
	if err == nil {
		_, err = k.client.CoreV1().Namespaces().Get(context.Background(), k.namespace, v1.GetOptions{})
	}
	return errors.Annotate(err, "backend not reachable")
}

// getSecret return a secret resource.
func (k *k8sBackend) getSecret(secretName string) (*core.Secret, error) {
	if k.namespace == "" {
		return nil, errNoNamespace
	}
	secret, err := k.client.CoreV1().Secrets(k.namespace).Get(context.TODO(), secretName, v1.GetOptions{})
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return nil, errors.NotFoundf("secret %q", secretName)
		}
		return nil, errors.Trace(err)
	}
	return secret, nil
}

// GetContent implements SecretBackend.
func (k *k8sBackend) GetContent(ctx context.Context, revisionId string) (secrets.SecretValue, error) {
	// revisionId is the secret name.
	secret, err := k.getSecret(revisionId)
	if k8serrors.IsForbidden(err) {
		logger.Tracef("getting secret %q: %v", revisionId, err)
		return nil, errors.Unauthorizedf("cannot access %q", revisionId)
	}
	if err != nil {
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
	name := uri.Name(revision)
	labels := utils.LabelsMerge(
		utils.LabelsForModel(k.model, false),
		utils.LabelsJuju)
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
func (k *k8sBackend) DeleteContent(ctx context.Context, revisionId string) error {
	// revisionId is the secret name.
	secret, err := k.getSecret(revisionId)
	if k8serrors.IsForbidden(err) {
		logger.Tracef("deleting secret %q: %v", revisionId, err)
		return errors.Unauthorizedf("cannot access %q", revisionId)
	}
	if err != nil {
		return errors.Trace(err)
	}
	return resources.NewSecret(secret.Name, k.namespace, secret).Delete(ctx, k.client)
}
