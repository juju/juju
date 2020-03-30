// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider

import (
	"encoding/base64"

	"github.com/juju/errors"
	core "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	k8sspecs "github.com/juju/juju/caas/kubernetes/provider/specs"
	"github.com/juju/juju/caas/specs"
	k8sannotations "github.com/juju/juju/core/annotations"
)

func (k *kubernetesClient) getSecretLabels(appName string) map[string]string {
	return map[string]string{
		labelApplication: appName,
	}
}

func processSecretData(in map[string]string) (_ map[string][]byte, err error) {
	out := make(map[string][]byte)
	for k, v := range in {
		if out[k], err = base64.StdEncoding.DecodeString(v); err != nil {
			return nil, errors.Trace(err)
		}
	}
	return out, nil
}

func (k *kubernetesClient) ensureSecrets(appName string, annotations k8sannotations.Annotation, secrets []k8sspecs.Secret) (cleanUps []func(), err error) {
	for _, v := range secrets {
		spec := &core.Secret{
			ObjectMeta: v1.ObjectMeta{
				Name:        v.Name,
				Namespace:   k.namespace,
				Labels:      k.getSecretLabels(appName),
				Annotations: annotations.Merge(v.Annotations),
			},
			Type:       v.Type,
			StringData: v.StringData,
		}
		if len(v.Data) > 0 {
			if spec.Data, err = processSecretData(v.Data); err != nil {
				return cleanUps, errors.Trace(err)
			}
		}
		secretCleanup, err := k.ensureSecret(spec)
		cleanUps = append(cleanUps, secretCleanup)
		if err != nil {
			return cleanUps, errors.Trace(err)
		}
	}
	return cleanUps, nil
}

// ensureOCIImageSecret ensures a secret exists for use with retrieving images from private registries
func (k *kubernetesClient) ensureOCIImageSecret(
	imageSecretName,
	appName string,
	imageDetails *specs.ImageDetails,
	annotations k8sannotations.Annotation,
) error {
	if imageDetails.Password == "" {
		return errors.New("attempting to create a secret with no password")
	}
	secretData, err := createDockerConfigJSON(imageDetails)
	if err != nil {
		return errors.Trace(err)
	}

	newSecret := &core.Secret{
		ObjectMeta: v1.ObjectMeta{
			Name:        imageSecretName,
			Namespace:   k.namespace,
			Labels:      k.getSecretLabels(appName),
			Annotations: annotations.ToMap()},
		Type: core.SecretTypeDockerConfigJson,
		Data: map[string][]byte{
			core.DockerConfigJsonKey: secretData,
		},
	}
	logger.Debugf("ensuring docker secret %q", imageSecretName)
	_, err = k.ensureSecret(newSecret)
	return errors.Trace(err)
}

func (k *kubernetesClient) ensureSecret(sec *core.Secret) (func(), error) {
	cleanUp := func() {}
	out, err := k.createSecret(sec)
	if err == nil {
		logger.Debugf("secret %q created", out.GetName())
		cleanUp = func() { _ = k.deleteSecret(out.GetName(), out.GetUID()) }
		return cleanUp, nil
	}
	if !errors.IsAlreadyExists(err) {
		return cleanUp, errors.Trace(err)
	}
	_, err = k.listSecrets(sec.GetLabels())
	if err != nil {
		if errors.IsNotFound(err) {
			// sec.Name is already used for an existing secret.
			return cleanUp, errors.AlreadyExistsf("secret %q", sec.GetName())
		}
		return cleanUp, errors.Trace(err)
	}
	err = k.updateSecret(sec)
	logger.Debugf("updating secret %q", sec.GetName())
	return cleanUp, errors.Trace(err)
}

// updateSecret updates a secret resource.
func (k *kubernetesClient) updateSecret(sec *core.Secret) error {
	_, err := k.client().CoreV1().Secrets(k.namespace).Update(sec)
	if k8serrors.IsNotFound(err) {
		return errors.NotFoundf("secret %q", sec.GetName())
	}
	return errors.Trace(err)
}

// getSecret return a secret resource.
func (k *kubernetesClient) getSecret(secretName string) (*core.Secret, error) {
	secret, err := k.client().CoreV1().Secrets(k.namespace).Get(secretName, v1.GetOptions{})
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return nil, errors.NotFoundf("secret %q", secretName)
		}
		return nil, errors.Trace(err)
	}
	return secret, nil
}

// createSecret creates a secret resource.
func (k *kubernetesClient) createSecret(secret *core.Secret) (*core.Secret, error) {
	purifyResource(secret)
	out, err := k.client().CoreV1().Secrets(k.namespace).Create(secret)
	if k8serrors.IsAlreadyExists(err) {
		return nil, errors.AlreadyExistsf("secret %q", secret.GetName())
	}
	return out, errors.Trace(err)
}

// deleteSecret deletes a secret resource.
func (k *kubernetesClient) deleteSecret(secretName string, uid types.UID) error {
	err := k.client().CoreV1().Secrets(k.namespace).Delete(secretName, newPreconditionDeleteOptions(uid))
	if k8serrors.IsNotFound(err) {
		return nil
	}
	return errors.Trace(err)
}

func (k *kubernetesClient) listSecrets(labels map[string]string) ([]core.Secret, error) {
	listOps := v1.ListOptions{
		LabelSelector: labelSetToSelector(labels).String(),
	}
	secList, err := k.client().CoreV1().Secrets(k.namespace).List(listOps)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if len(secList.Items) == 0 {
		return nil, errors.NotFoundf("secret with labels %v", labels)
	}
	return secList.Items, nil
}

func (k *kubernetesClient) deleteSecrets(appName string) error {
	err := k.client().CoreV1().Secrets(k.namespace).DeleteCollection(&v1.DeleteOptions{
		PropagationPolicy: &defaultPropagationPolicy,
	}, v1.ListOptions{
		LabelSelector: labelSetToSelector(k.getSecretLabels(appName)).String(),
	})
	if k8serrors.IsNotFound(err) {
		return nil
	}
	return errors.Trace(err)
}
