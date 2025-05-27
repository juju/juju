// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package kubernetes

import (
	"context"
	"encoding/base64"
	"time"

	"github.com/juju/errors"
	core "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	k8sannotations "github.com/juju/juju/core/annotations"
	"github.com/juju/juju/internal/provider/kubernetes/proxy"
	"github.com/juju/juju/internal/provider/kubernetes/utils"
)

func processSecretData(in map[string]string) (_ map[string][]byte, err error) {
	out := make(map[string][]byte)
	for k, v := range in {
		if out[k], err = base64.StdEncoding.DecodeString(v); err != nil {
			return nil, errors.Trace(err)
		}
	}
	return out, nil
}

// ensureOCIImageSecret ensures a secret exists for use with retrieving images from private registries.
func (k *kubernetesClient) ensureOCIImageSecret(
	ctx context.Context,
	name string,
	labels map[string]string,
	secretData []byte,
	annotations k8sannotations.Annotation,
) (func(), error) {
	newSecret := &core.Secret{
		ObjectMeta: v1.ObjectMeta{
			Name:        name,
			Namespace:   k.namespace,
			Labels:      labels,
			Annotations: annotations.ToMap()},
		Type: core.SecretTypeDockerConfigJson,
		Data: map[string][]byte{
			core.DockerConfigJsonKey: secretData,
		},
	}
	logger.Debugf(context.TODO(), "ensuring docker secret %q", name)
	return k.ensureSecret(ctx, newSecret)
}

func (k *kubernetesClient) ensureSecret(ctx context.Context, sec *core.Secret) (func(), error) {
	cleanUp := func() {}
	out, err := k.createSecret(ctx, sec)
	if err == nil {
		logger.Debugf(context.TODO(), "secret %q created", out.GetName())
		cleanUp = func() { _ = k.deleteSecret(ctx, out.GetName(), out.GetUID()) }
		return cleanUp, nil
	}
	if !errors.Is(err, errors.AlreadyExists) {
		return cleanUp, errors.Trace(err)
	}
	_, err = k.listSecrets(ctx, sec.GetLabels())
	if err != nil {
		if errors.Is(err, errors.NotFound) {
			// sec.Name is already used for an existing secret.
			return cleanUp, errors.AlreadyExistsf("secret %q", sec.GetName())
		}
		return cleanUp, errors.Trace(err)
	}
	err = k.updateSecret(ctx, sec)
	logger.Debugf(context.TODO(), "updating secret %q", sec.GetName())
	return cleanUp, errors.Trace(err)
}

// updateSecret updates a secret resource.
func (k *kubernetesClient) updateSecret(ctx context.Context, sec *core.Secret) error {
	if k.namespace == "" {
		return errNoNamespace
	}
	_, err := k.client().CoreV1().Secrets(k.namespace).Update(ctx, sec, v1.UpdateOptions{})
	if k8serrors.IsNotFound(err) {
		return errors.NotFoundf("secret %q", sec.GetName())
	}
	return errors.Trace(err)
}

// getSecret return a secret resource.
func (k *kubernetesClient) getSecret(ctx context.Context, secretName string) (*core.Secret, error) {
	if k.namespace == "" {
		return nil, errNoNamespace
	}
	secret, err := k.client().CoreV1().Secrets(k.namespace).Get(ctx, secretName, v1.GetOptions{})
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return nil, errors.NotFoundf("secret %q", secretName)
		}
		return nil, errors.Trace(err)
	}
	return secret, nil
}

// createSecret creates a secret resource.
func (k *kubernetesClient) createSecret(ctx context.Context, secret *core.Secret) (*core.Secret, error) {
	if k.namespace == "" {
		return nil, errNoNamespace
	}
	utils.PurifyResource(secret)
	out, err := k.client().CoreV1().Secrets(k.namespace).Create(ctx, secret, v1.CreateOptions{})
	if k8serrors.IsAlreadyExists(err) {
		return nil, errors.AlreadyExistsf("secret %q", secret.GetName())
	}
	return out, errors.Trace(err)
}

// deleteSecret deletes a secret resource.
func (k *kubernetesClient) deleteSecret(ctx context.Context, secretName string, uid types.UID) error {
	if k.namespace == "" {
		return errNoNamespace
	}
	err := k.client().CoreV1().Secrets(k.namespace).Delete(ctx, secretName, utils.NewPreconditionDeleteOptions(uid))
	if k8serrors.IsNotFound(err) {
		return nil
	}
	return errors.Trace(err)
}

func (k *kubernetesClient) listSecrets(ctx context.Context, labels map[string]string) ([]core.Secret, error) {
	if k.namespace == "" {
		return nil, errNoNamespace
	}
	listOps := v1.ListOptions{
		LabelSelector: utils.LabelsToSelector(labels).String(),
	}
	secList, err := k.client().CoreV1().Secrets(k.namespace).List(ctx, listOps)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if len(secList.Items) == 0 {
		return nil, errors.NotFoundf("secret with labels %v", labels)
	}
	return secList.Items, nil
}

var timeoutForSecretTokenGet = 10 * time.Second

// GetSecretToken returns the token content for the specified secret name.
func (k *kubernetesClient) GetSecretToken(ctx context.Context, name string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, timeoutForSecretTokenGet)
	defer cancel()

	secret, err := proxy.FetchTokenReadySecret(
		ctx, name, k.client().CoreV1().Secrets(k.namespace), k.clock,
	)
	if k8serrors.IsNotFound(err) {
		return "", errors.NotFoundf("secret %q", name)
	}
	if err != nil {
		return "", errors.Trace(err)
	}
	return string(secret.Data[core.ServiceAccountTokenKey]), nil
}
