// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasadmission

import (
	"bytes"
	"context"
	"fmt"

	"github.com/juju/errors"
	admission "k8s.io/api/admissionregistration/v1"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/juju/juju/internal/pki"
	k8sconstants "github.com/juju/juju/internal/provider/kubernetes/constants"
	k8sutils "github.com/juju/juju/internal/provider/kubernetes/utils"
)

// AdmissionCreator represents a creator of mutating webhooks that is context aware of the
// current controller.
type AdmissionCreator interface {
	EnsureMutatingWebhookConfiguration(context.Context) (func(), error)
}

// AdmissionCreatorFunc is the func type of AdmissionCreator.
type AdmissionCreatorFunc func(context.Context) (func(), error)

const (
	// Component describes a sub zone to use on the juju tld for unique resource
	// ids. For example using this component "admission" with "juju.io" would
	// yield admission.juju.io
	Component = "admission"

	// we still accept v1beta1 AdmissionReview only.
	reviewVersionV1beta1 = "v1beta1"
)

var (
	anyMatch = []string{"*"}
)

// EnsureMutatingWebhookConfiguration implements AdmissionCreator interface for
// func type.
func (a AdmissionCreatorFunc) EnsureMutatingWebhookConfiguration(ctx context.Context) (func(), error) {
	return a(ctx)
}

// NewAdmissionCreator instantiates a new AdmissionCreator for the supplied
// context arguments.
func NewAdmissionCreator(
	authority pki.Authority,
	namespace string,
	modelName string,
	modelUUID string,
	controllerUUID string,
	labelVersion k8sconstants.LabelVersion,
	ensureConfig func(context.Context, *admission.MutatingWebhookConfiguration) (func(), error),
	service *admission.ServiceReference) (AdmissionCreator, error) {

	caPemBuffer := bytes.Buffer{}
	if err := pki.CertificateToPemWriter(&caPemBuffer, map[string]string{},
		authority.Certificate()); err != nil {
		return nil, errors.Trace(err)
	}

	// TODO change to fail
	failurePolicy := admission.Ignore
	matchPolicy := admission.Equivalent
	ruleScope := admission.AllScopes
	sideEffects := admission.SideEffectClassNone

	// MutatingWebhook Obj
	obj := admission.MutatingWebhookConfiguration{
		ObjectMeta: meta.ObjectMeta{
			Labels:    k8sutils.LabelsForModel(modelName, modelUUID, controllerUUID, labelVersion),
			Name:      fmt.Sprintf("juju-model-admission-%s", namespace),
			Namespace: namespace,
		},
		Webhooks: []admission.MutatingWebhook{
			{
				SideEffects: &sideEffects,
				ClientConfig: admission.WebhookClientConfig{
					CABundle: caPemBuffer.Bytes(),
					Service:  service,
				},
				AdmissionReviewVersions: []string{reviewVersionV1beta1},
				FailurePolicy:           &failurePolicy,
				MatchPolicy:             &matchPolicy,
				Name:                    k8sutils.MakeK8sDomain(Component),
				NamespaceSelector: &meta.LabelSelector{
					MatchLabels: k8sutils.LabelsForModel(modelName, modelUUID, controllerUUID, labelVersion),
				},
				ObjectSelector: &meta.LabelSelector{
					MatchExpressions: []meta.LabelSelectorRequirement{
						{
							Key:      k8sconstants.LabelJujuModelOperatorDisableWebhook,
							Operator: meta.LabelSelectorOpDoesNotExist,
						},
					},
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

	return AdmissionCreatorFunc(func(ctx context.Context) (func(), error) {
		leafGroup := fmt.Sprintf("k8sadmission-%s", modelName)
		_, err := authority.LeafRequestForGroup(leafGroup).
			AddDNSNames(fmt.Sprintf("%s.%s.svc", service.Name, service.Namespace)).
			Commit()
		if err != nil {
			return nil, errors.Trace(err)
		}

		configCleanup, err := ensureConfig(ctx, &obj)
		if err != nil {
			return nil, errors.Trace(err)
		}

		return func() {
			configCleanup()
		}, nil
	}), nil
}
