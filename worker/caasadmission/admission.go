// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasadmission

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"fmt"

	"github.com/juju/errors"
	admission "k8s.io/api/admissionregistration/v1beta1"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/juju/juju/caas/kubernetes/provider"
)

type CertificateBroker interface {
	// RegisterDomainLeases registers a new domain name lease on the broker's
	// certificate. Returns a cleanup lease function on success.
	RegisterDomainLeases(...string) (func(), error)

	// RootCertificates set of root certificates in pem format for the broker.
	RootCertificates() ([]byte, error)
}

// TODO this is a tmp struct and won't make it to final
type CertWatcherBroker struct {
	CertWatcher func() *tls.Certificate
}

// Represents a creator of mutating webhooks that is context aware of the
// current controller.
type AdmissionCreator interface {
	EnsureMutatingWebhookConfiguration() (func(), error)
}

// Func type of AdmissionCreator
type AdmissionCreatorFunc func() (func(), error)

const (
	// Component describes a sub zone to use on the juju tld for unique resource
	// ids. For example using this component "admission" with "juju.io" would
	// yeild admission.juju.io
	Component = "admission"
)

var (
	anyMatch = []string{"*"}
)

// EnsureMutatingWebhookConfiguration implements AdmissionCreator interface for
// func type
func (a AdmissionCreatorFunc) EnsureMutatingWebhookConfiguration() (func(), error) {
	return a()
}

// NewAdmissionCreator instantiates a new AdmissionCreator for the supplied
// context arguments.
func NewAdmissionCreator(
	certBroker CertificateBroker,
	namespace, modelName string,
	ensureConfig func(*admission.MutatingWebhookConfiguration) (func(), error),
	service *admission.ServiceReference) (AdmissionCreator, error) {

	caPems, err := certBroker.RootCertificates()
	if err != nil {
		return nil, errors.Trace(err)
	}

	// TODO change to fail
	failurePolicy := admission.Ignore
	matchPolicy := admission.Equivalent
	ruleScope := admission.AllScopes

	// MutatingWebjook Obj
	obj := admission.MutatingWebhookConfiguration{
		ObjectMeta: meta.ObjectMeta{
			Labels:    provider.LabelsForModel(modelName),
			Name:      fmt.Sprintf("%s-model-admission", modelName),
			Namespace: namespace,
		},
		Webhooks: []admission.MutatingWebhook{
			{
				ClientConfig: admission.WebhookClientConfig{
					CABundle: caPems,
					Service:  service,
				},
				FailurePolicy: &failurePolicy,
				MatchPolicy:   &matchPolicy,
				Name:          provider.MakeK8sDomain(Component),
				NamespaceSelector: &meta.LabelSelector{
					MatchLabels: provider.LabelsForModel(modelName),
				},
				Rules: []admission.RuleWithOperations{
					{
						Operations: []admission.OperationType{
							admission.Create,
							admission.Update,
						},
						Rule: admission.Rule{
							APIGroups:   anyMatch,
							APIVersions: anyMatch,
							Resources:   anyMatch,
							Scope:       &ruleScope,
						},
					},
				},
			},
		},
	}

	return AdmissionCreatorFunc(func() (func(), error) {
		certRevoke, err := certBroker.RegisterDomainLeases(
			fmt.Sprintf("%s.%s.svc", service.Name, service.Namespace))
		if err != nil {
			return nil, errors.Trace(err)
		}

		configCleanup, err := ensureConfig(&obj)
		if err != nil {
			certRevoke()
			return nil, errors.Trace(err)
		}

		return func() {
			configCleanup()
			certRevoke()
		}, nil
	}), nil
}

func (c *CertWatcherBroker) RegisterDomainLeases(_ ...string) (func(), error) {
	return func() {}, nil
}

func (c *CertWatcherBroker) RootCertificates() ([]byte, error) {
	if c.CertWatcher == nil {
		return nil, errors.NewNotValid(nil, "not cert watcher fn defined")
	}

	caPems := &bytes.Buffer{}
	for _, derCert := range c.CertWatcher().Certificate {
		cert, err := x509.ParseCertificate(derCert)
		if err != nil {
			return nil, errors.Trace(err)
		}
		if !cert.IsCA {
			continue
		}
		if err := pem.Encode(caPems, &pem.Block{
			Type:  "CERTIFICATE",
			Bytes: derCert}); err != nil {
			return nil, errors.Trace(err)
		}
	}
	return caPems.Bytes(), nil
}
