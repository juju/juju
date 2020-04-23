// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package specs

import (
	"fmt"
	"strings"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	admissionregistration "k8s.io/api/admissionregistration/v1beta1"
	core "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apiextensionsv1beta1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/juju/juju/caas/specs"
)

type caaSSpecV3 = specs.PodSpecV3

type podSpecV3 struct {
	caaSSpecV3    `json:",inline" yaml:",inline"`
	K8sPodSpecV3  `json:",inline" yaml:",inline"`
	k8sContainers `json:",inline" yaml:",inline"`
}

// Validate is defined on ProviderPod.
func (p podSpecV3) Validate() error {
	if err := p.K8sPodSpecV3.Validate(); err != nil {
		return errors.Trace(err)
	}
	if err := p.k8sContainers.Validate(); err != nil {
		return errors.Trace(err)
	}
	return nil
}

func (p podSpecV3) ToLatest() *specs.PodSpec {
	pSpec := &specs.PodSpec{}
	pSpec.Version = specs.CurrentVersion
	for _, c := range p.Containers {
		pSpec.Containers = append(pSpec.Containers, c.ToContainerSpec())
	}
	pSpec.Service = p.caaSSpecV3.Service
	pSpec.ConfigMaps = p.caaSSpecV3.ConfigMaps
	pSpec.ServiceAccount = p.caaSSpecV3.ServiceAccount
	pSpec.ProviderPod = &p.K8sPodSpecV3
	return pSpec
}

// K8sServiceAccountSpec defines spec for referencing or creating additional RBAC resources.
type K8sServiceAccountSpec struct {
	Name                       string `json:"name" yaml:"name"`
	specs.ServiceAccountSpecV3 `json:",inline" yaml:",inline"`
}

// Validate returns an error if the spec is not valid.
func (sa K8sServiceAccountSpec) Validate() error {
	if sa.Name == "" {
		return errors.New("name is missing")
	}
	return errors.Trace(sa.ServiceAccountSpecV3.Validate())
}

// K8sRBACResources defines a spec for creating RBAC resources.
type K8sRBACResources struct {
	K8sRBACSpecConverter
	ServiceAccounts []K8sServiceAccountSpec `json:"serviceAccounts,omitempty" yaml:"serviceAccounts,omitempty"`
}

// K8sRBACSpecConverter has a method to convert modelled RBAC spec to k8s spec.
type K8sRBACSpecConverter interface {
	ToK8s(
		getSaMeta ServiceAccountMetaGetter,
		getRoleMeta, getClusterRoleMeta RoleMetaGetter,
		getBindingMeta, getClusterBindingMeta BindingMetaGetter,
	) (
		[]core.ServiceAccount,
		[]rbacv1.Role,
		[]rbacv1.ClusterRole,
		[]rbacv1.RoleBinding,
		[]rbacv1.ClusterRoleBinding,
	)
}

// Validate validates the spec.
func (ks K8sRBACResources) Validate() error {
	saNames := set.NewStrings()
	for _, sa := range ks.ServiceAccounts {
		if err := sa.Validate(); err != nil {
			return errors.Trace(err)
		}
		if saNames.Contains(sa.Name) {
			return errors.NotValidf("duplicated service account name %q", sa.Name)
		}
		saNames.Add(sa.Name)

		roleNames := set.NewStrings()
		for _, r := range sa.Roles {
			if r.Name == "" {
				continue
			}
			if roleNames.Contains(r.Name) {
				return errors.NotValidf("duplicated role name %q", r.Name)
			}
			roleNames.Add(r.Name)
		}
		if roleNames.Size() == 0 || len(sa.Roles) == roleNames.Size() {
			// All good.
			continue
		}
		return errors.NewNotValid(nil, fmt.Sprintf("either all or none of the roles of the service account %q should have a name set", sa.Name))
	}
	return nil
}

// NameGetter defines method to get the name from the resource.
type NameGetter interface {
	GetName() string
}

// ServiceAccountMetaGetter generates ObjectMeta for service accounts.
type ServiceAccountMetaGetter func(rawName string) v1.ObjectMeta

// RoleMetaGetter generates ObjectMeta for roles, cluster roles.
type RoleMetaGetter func(roleName, serviceAccountName string, index int) v1.ObjectMeta

// BindingMetaGetter generates ObjectMeta for role bindings, cluster role bindings.
type BindingMetaGetter func(sa, roleOrClusterRole NameGetter) v1.ObjectMeta

func toK8sRules(rules []specs.PolicyRule) (out []rbacv1.PolicyRule) {
	for _, r := range rules {
		out = append(out, rbacv1.PolicyRule{
			Verbs:           r.Verbs,
			APIGroups:       r.APIGroups,
			Resources:       r.Resources,
			ResourceNames:   r.ResourceNames,
			NonResourceURLs: r.NonResourceURLs,
		})
	}
	return out
}

// ToK8s converts modelled RBAC specs to k8s specs.
func (ks K8sRBACResources) ToK8s(
	getSaMeta ServiceAccountMetaGetter,
	getRoleMeta, getClusterRoleMeta RoleMetaGetter,
	getBindingMeta, getClusterBindingMeta BindingMetaGetter,
) (
	serviceAccounts []core.ServiceAccount,
	roles []rbacv1.Role,
	clusterroles []rbacv1.ClusterRole,
	roleBindings []rbacv1.RoleBinding,
	clusterRoleBindings []rbacv1.ClusterRoleBinding,
) {
	for _, spec := range ks.ServiceAccounts {
		sa := core.ServiceAccount{
			ObjectMeta:                   getSaMeta(spec.Name),
			AutomountServiceAccountToken: spec.AutomountServiceAccountToken,
		}
		serviceAccounts = append(serviceAccounts, sa)
		for i, r := range spec.Roles {
			if r.Global {
				cR := rbacv1.ClusterRole{
					ObjectMeta: getClusterRoleMeta(r.Name, sa.GetName(), i),
					Rules:      toK8sRules(r.Rules),
				}
				clusterroles = append(clusterroles, cR)
				clusterRoleBindings = append(clusterRoleBindings, rbacv1.ClusterRoleBinding{
					ObjectMeta: getClusterBindingMeta(&sa, &cR),
					RoleRef: rbacv1.RoleRef{
						Name: cR.GetName(),
						Kind: "ClusterRole",
					},
					Subjects: []rbacv1.Subject{
						{
							Kind:      rbacv1.ServiceAccountKind,
							Name:      sa.GetName(),
							Namespace: sa.GetNamespace(),
						},
					},
				})
			} else {
				r := rbacv1.Role{
					ObjectMeta: getRoleMeta(r.Name, sa.GetName(), i),
					Rules:      toK8sRules(r.Rules),
				}
				roles = append(roles, r)
				roleBindings = append(roleBindings, rbacv1.RoleBinding{
					ObjectMeta: getBindingMeta(&sa, &r),
					RoleRef: rbacv1.RoleRef{
						Name: r.GetName(),
						Kind: "Role",
					},
					Subjects: []rbacv1.Subject{
						{
							Kind:      rbacv1.ServiceAccountKind,
							Name:      sa.GetName(),
							Namespace: sa.GetNamespace(),
						},
					},
				})
			}
		}
	}
	return
}

// PrimeServiceAccountToK8sRBACResources converts PrimeServiceAccount to K8sRBACResources.
func PrimeServiceAccountToK8sRBACResources(spec specs.PrimeServiceAccountSpecV3) (*K8sRBACResources, error) {
	out := &K8sRBACResources{
		ServiceAccounts: []K8sServiceAccountSpec{
			{
				Name:                 spec.GetName(),
				ServiceAccountSpecV3: spec.ServiceAccountSpecV3,
			},
		},
	}
	if err := out.Validate(); err != nil {
		return nil, errors.Trace(err)
	}
	return out, nil
}

// Meta defines fields for constructing k8s spec ObjectMeta field.
type Meta struct {
	Name        string            `json:"name" yaml:"name"`
	Labels      map[string]string `json:"labels,omitempty" yaml:"labels,omitempty"`
	Annotations map[string]string `json:"annotations,omitempty" yaml:"annotations,omitempty"`
}

// Validate validates the spec.
func (m Meta) Validate() error {
	if len(m.Name) == 0 {
		return errors.New("name is missing")
	}
	return nil
}

// K8sCustomResourceDefinitionSpec defines spec for creating or updating an CustomResourceDefinition resource.
type K8sCustomResourceDefinitionSpec struct {
	Meta `json:",inline" yaml:",inline"`
	Spec apiextensionsv1beta1.CustomResourceDefinitionSpec `json:"spec" yaml:"spec"`
}

// Validate validates the spec.
func (crd K8sCustomResourceDefinitionSpec) Validate() error {
	if err := crd.Meta.Validate(); err != nil {
		return errors.Trace(err)
	}

	if crd.Spec.Scope != apiextensionsv1beta1.NamespaceScoped && crd.Spec.Scope != apiextensionsv1beta1.ClusterScoped {
		return errors.NewNotSupported(nil,
			fmt.Sprintf("custom resource definition %q scope %q is not supported, please use %q or %q scope",
				crd.Name, crd.Spec.Scope, apiextensionsv1beta1.NamespaceScoped, apiextensionsv1beta1.ClusterScoped),
		)
	}
	if err := validateLabels(crd.Labels); err != nil {
		return errors.Trace(err)
	}
	return nil
}

// K8sMutatingWebhookSpec defines spec for creating or updating an MutatingWebhook resource.
type K8sMutatingWebhookSpec struct {
	Meta     `json:",inline" yaml:",inline"`
	Webhooks []admissionregistration.MutatingWebhook `json:"webhooks" yaml:"webhooks"`
}

func (w K8sMutatingWebhookSpec) Validate() error {
	if err := w.Meta.Validate(); err != nil {
		return errors.Trace(err)
	}
	if len(w.Webhooks) == 0 {
		return errors.NotValidf("empty webhooks %q", w.Name)
	}
	return nil
}

// K8sValidatingWebhookSpec defines spec for creating or updating an ValidatingWebhook resource.
type K8sValidatingWebhookSpec struct {
	Meta     `json:",inline" yaml:",inline"`
	Webhooks []admissionregistration.ValidatingWebhook `json:"webhooks" yaml:"webhooks"`
}

func (w K8sValidatingWebhookSpec) Validate() error {
	if err := w.Meta.Validate(); err != nil {
		return errors.Trace(err)
	}
	if len(w.Webhooks) == 0 {
		return errors.NotValidf("empty webhooks %q", w.Name)
	}
	return nil
}

// KubernetesResources is the k8s related resources.
type KubernetesResources struct {
	Pod *PodSpec `json:"pod,omitempty" yaml:"pod,omitempty"`

	Secrets                   []Secret                               `json:"secrets" yaml:"secrets"`
	CustomResourceDefinitions []K8sCustomResourceDefinitionSpec      `json:"customResourceDefinitions" yaml:"customResourceDefinitions"`
	CustomResources           map[string][]unstructured.Unstructured `json:"customResources,omitempty" yaml:"customResources,omitempty"`

	MutatingWebhookConfigurations   []K8sMutatingWebhookSpec   `json:"mutatingWebhookConfigurations,omitempty" yaml:"mutatingWebhookConfigurations,omitempty"`
	ValidatingWebhookConfigurations []K8sValidatingWebhookSpec `json:"validatingWebhookConfigurations,omitempty" yaml:"validatingWebhookConfigurations,omitempty"`

	K8sRBACResources `json:",inline" yaml:",inline"`

	IngressResources []K8sIngressSpec `json:"ingressResources,omitempty" yaml:"ingressResources,omitempty"`
}

// Validate is defined on ProviderPod.
func (krs *KubernetesResources) Validate() error {
	for _, crd := range krs.CustomResourceDefinitions {
		if err := crd.Validate(); err != nil {
			return errors.Trace(err)
		}
	}
	for k, crs := range krs.CustomResources {
		if len(crs) == 0 {
			return errors.NotValidf("empty custom resources %q", k)
		}
	}

	for _, webhook := range krs.MutatingWebhookConfigurations {
		if err := webhook.Validate(); err != nil {
			return errors.Trace(err)
		}
	}
	for _, webhook := range krs.ValidatingWebhookConfigurations {
		if err := webhook.Validate(); err != nil {
			return errors.Trace(err)
		}
	}

	if err := krs.K8sRBACResources.Validate(); err != nil {
		return errors.Trace(err)
	}

	for _, ing := range krs.IngressResources {
		if err := ing.Validate(); err != nil {
			return errors.Trace(err)
		}
	}
	return nil
}

// K8sPodSpecV3 is a subset of v1.PodSpec which defines
// attributes we expose for charms to set.
type K8sPodSpecV3 struct {
	// k8s resources.
	KubernetesResources *KubernetesResources `json:"kubernetesResources,omitempty" yaml:"kubernetesResources,omitempty"`
}

// Validate is defined on ProviderPod.
func (p *K8sPodSpecV3) Validate() error {
	if p.KubernetesResources != nil {
		if err := p.KubernetesResources.Validate(); err != nil {
			return errors.Trace(err)
		}
	}
	return nil
}

func parsePodSpecV3(in string) (_ PodSpecConverter, err error) {
	var spec podSpecV3
	decoder := newStrictYAMLOrJSONDecoder(strings.NewReader(in), len(in))
	if err = decoder.Decode(&spec); err != nil {
		return nil, errors.Trace(err)
	}
	return &spec, nil
}
