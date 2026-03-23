// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package kubernetes

import (
	"context"
	"encoding/base64"
	"math"
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/retry"
	core "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	coresecrets "github.com/juju/juju/core/secrets"
	"github.com/juju/juju/internal/provider/kubernetes/resources"
)

type k8sBackend struct {
	serviceAccount string
	namespace      string
	modelName      string
	modelUUID      string

	client kubernetes.Interface
	clock  clock.Clock
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
func (k *k8sBackend) getSecret(ctx context.Context, secretName string) (*resources.Secret, error) {
	if k.namespace == "" {
		return nil, errNoNamespace
	}
	secret := resources.NewSecret(k.client.CoreV1().Secrets(k.namespace), k.namespace, secretName, nil)
	err := retry.Call(retry.CallArgs{
		Func: func() error {
			return secret.Get(ctx)
		},
		IsFatalError: isFatalError,
		NotifyFunc: func(lastError error, attempt int) {
			logFn := logger.Warningf
			if attempt <= 1 {
				// It is expected that this may fail at least once due to etcd
				// eventual consistency, silence the first warning.
				logFn = logger.Debugf
			}
			logFn(
				"error getting secret (attempt %d): %s",
				attempt, lastError.Error(),
			)
		},
		Attempts:    10,
		Delay:       time.Second,
		BackoffFunc: retry.ExpBackoff(time.Second, time.Minute, math.Phi, true),
		Clock:       k.clock,
		Stop:        ctx.Done(),
	})
	if k8serrors.IsNotFound(err) {
		return nil, errors.NotFoundf("secret %q", secretName)
	} else if err != nil {
		return nil, errors.Trace(err)
	}
	return secret, nil
}

// GetContent implements SecretBackend.
func (k *k8sBackend) GetContent(ctx context.Context, revisionId string) (_ coresecrets.SecretValue, err error) {
	defer func() {
		err = maybePermissionDenied(err)
	}()

	// revisionId is the secret name.
	secret, err := k.getSecret(ctx, revisionId)
	if err != nil {
		logger.Tracef("getting secret %q: %v", revisionId, err)
		return nil, errors.Trace(err)
	}
	data := map[string]string{}
	for k, v := range secret.Data {
		data[k] = base64.StdEncoding.EncodeToString(v)
	}
	return coresecrets.NewSecretValue(data), nil
}

// SaveContent implements SecretBackend.
func (k *k8sBackend) SaveContent(ctx context.Context, uri *coresecrets.URI, revision int, value coresecrets.SecretValue) (_ string, err error) {
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
	secret := resources.NewSecret(k.client.CoreV1().Secrets(k.namespace), k.namespace, name, in)
	err = retry.Call(retry.CallArgs{
		Func: func() error {
			return secret.Apply(ctx)
		},
		IsFatalError: isFatalError,
		NotifyFunc: func(lastError error, attempt int) {
			logFn := logger.Warningf
			if attempt <= 1 {
				// It is expected that this may fail at least once due to etcd
				// eventual consistency, silence the first warning.
				logFn = logger.Debugf
			}
			logFn(
				"error saving secret content (attempt %d): %s",
				attempt, lastError.Error(),
			)
		},
		Attempts:    10,
		Delay:       time.Second,
		BackoffFunc: retry.ExpBackoff(time.Second, time.Minute, math.Phi, true),
		Clock:       k.clock,
		Stop:        ctx.Done(),
	})
	if err != nil {
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
		logger.Tracef("deleting secret %q: %v", revisionId, err)
		return errors.Trace(err)
	}

	err = retry.Call(retry.CallArgs{
		Func: func() error {
			return secret.Delete(ctx)
		},
		IsFatalError: isFatalError,
		NotifyFunc: func(lastError error, attempt int) {
			logFn := logger.Warningf
			if attempt <= 1 {
				// It is expected that this may fail at least once due to etcd
				// eventual consistency, silence the first warning.
				logFn = logger.Debugf
			}
			logFn(
				"error deleting secret content (attempt %d): %s",
				attempt, lastError.Error(),
			)
		},
		Attempts:    10,
		Delay:       time.Second,
		BackoffFunc: retry.ExpBackoff(time.Second, time.Minute, math.Phi, true),
		Clock:       k.clock,
		Stop:        ctx.Done(),
	})
	if err != nil {
		return errors.Trace(err)
	}

	return nil
}

func isFatalError(err error) bool {
	temporary := k8serrors.IsForbidden(err) ||
		k8serrors.IsInternalError(err) ||
		k8serrors.IsServiceUnavailable(err) ||
		k8serrors.IsConflict(err) ||
		k8serrors.IsTooManyRequests(err) ||
		k8serrors.IsServerTimeout(err)
	return !temporary
}
