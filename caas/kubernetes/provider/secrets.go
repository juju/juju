// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider

import (
	"context"
	"encoding/base64"
	"time"

	"github.com/juju/errors"
	core "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/juju/juju/caas/kubernetes/provider/constants"
	"github.com/juju/juju/caas/kubernetes/provider/proxy"
	k8sspecs "github.com/juju/juju/caas/kubernetes/provider/specs"
	"github.com/juju/juju/caas/kubernetes/provider/utils"
	"github.com/juju/juju/caas/specs"
	k8sannotations "github.com/juju/juju/core/annotations"
)

func getSecretLabels(appName string, labelVersion constants.LabelVersion) map[string]string {
	return utils.LabelsForApp(appName, labelVersion)
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

func (k *kubernetesClient) ensureSecrets(appName string, annotations k8sannotations.Annotation, secrets []k8sspecs.K8sSecret) (cleanUps []func(), err error) {
	for _, v := range secrets {
		spec := &core.Secret{
			ObjectMeta: v1.ObjectMeta{
				Name:        v.Name,
				Namespace:   k.namespace,
				Labels:      getSecretLabels(appName, k.LabelVersion()),
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

func (k *kubernetesClient) ensureOCIImageSecretForApp(
	imageSecretName,
	appName string,
	imageDetails *specs.ImageDetails,
	annotations k8sannotations.Annotation,
) error {
	if imageDetails.Password == "" {
		return errors.New("attempting to create a secret with no password")
	}
	secretData, err := utils.CreateDockerConfigJSON(imageDetails.Username, imageDetails.Password, imageDetails.ImagePath)
	if err != nil {
		return errors.Trace(err)
	}

	logger.Debugf("ensuring docker secret %q", imageSecretName)
	_, err = k.ensureOCIImageSecret(
		imageSecretName,
		getSecretLabels(appName, k.LabelVersion()),
		secretData, annotations.ToMap(),
	)
	return errors.Trace(err)
}

// ensureOCIImageSecret ensures a secret exists for use with retrieving images from private registries.
func (k *kubernetesClient) ensureOCIImageSecret(
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
	logger.Debugf("ensuring docker secret %q", name)
	return k.ensureSecret(newSecret)
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
	if k.namespace == "" {
		return errNoNamespace
	}
	_, err := k.client().CoreV1().Secrets(k.namespace).Update(context.TODO(), sec, v1.UpdateOptions{})
	if k8serrors.IsNotFound(err) {
		return errors.NotFoundf("secret %q", sec.GetName())
	}
	return errors.Trace(err)
}

// getSecret return a secret resource.
func (k *kubernetesClient) getSecret(secretName string) (*core.Secret, error) {
	if k.namespace == "" {
		return nil, errNoNamespace
	}
	secret, err := k.client().CoreV1().Secrets(k.namespace).Get(context.TODO(), secretName, v1.GetOptions{})
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
	if k.namespace == "" {
		return nil, errNoNamespace
	}
	utils.PurifyResource(secret)
	out, err := k.client().CoreV1().Secrets(k.namespace).Create(context.TODO(), secret, v1.CreateOptions{})
	if k8serrors.IsAlreadyExists(err) {
		return nil, errors.AlreadyExistsf("secret %q", secret.GetName())
	}
	return out, errors.Trace(err)
}

// deleteSecret deletes a secret resource.
func (k *kubernetesClient) deleteSecret(secretName string, uid types.UID) error {
	if k.namespace == "" {
		return errNoNamespace
	}
	err := k.client().CoreV1().Secrets(k.namespace).Delete(context.TODO(), secretName, utils.NewPreconditionDeleteOptions(uid))
	if k8serrors.IsNotFound(err) {
		return nil
	}
	return errors.Trace(err)
}

func (k *kubernetesClient) listSecrets(labels map[string]string) ([]core.Secret, error) {
	if k.namespace == "" {
		return nil, errNoNamespace
	}
	listOps := v1.ListOptions{
		LabelSelector: utils.LabelsToSelector(labels).String(),
	}
	secList, err := k.client().CoreV1().Secrets(k.namespace).List(context.TODO(), listOps)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if len(secList.Items) == 0 {
		return nil, errors.NotFoundf("secret with labels %v", labels)
	}
	return secList.Items, nil
}

func (k *kubernetesClient) deleteSecrets(appName string) error {
	if k.namespace == "" {
		return errNoNamespace
	}
	err := k.client().CoreV1().Secrets(k.namespace).DeleteCollection(context.TODO(), v1.DeleteOptions{
		PropagationPolicy: constants.DefaultPropagationPolicy(),
	}, v1.ListOptions{
		LabelSelector: utils.LabelsToSelector(
			getSecretLabels(appName, k.LabelVersion())).String(),
	})
	if k8serrors.IsNotFound(err) {
		return nil
	}
	return errors.Trace(err)
}

var timeoutForSecretTokenGet = 10 * time.Second

// GetSecretToken returns the token content for the specified secret name.
func (k *kubernetesClient) GetSecretToken(name string) (string, error) {
	ctx, cancel := context.WithTimeout(context.TODO(), timeoutForSecretTokenGet)
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
