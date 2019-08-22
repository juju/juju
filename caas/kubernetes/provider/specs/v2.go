// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package specs

import (
	// "strings"

	// "github.com/juju/errors"
	// "gopkg.in/yaml.v2"
	core "k8s.io/api/core/v1"
	apiextensionsv1beta1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	// k8syaml "k8s.io/apimachinery/pkg/util/yaml"

	// "github.com/juju/juju/caas"
	"github.com/juju/juju/caas/specs"
)

// K8sPodSpecV2 is a subset of v1.PodSpec which defines
// attributes we expose for charms to set.
type K8sPodSpecV2 struct {
	RestartPolicy                 core.RestartPolicy       `json:"restartPolicy,omitempty"`
	TerminationGracePeriodSeconds *int64                   `json:"terminationGracePeriodSeconds,omitempty"`
	ActiveDeadlineSeconds         *int64                   `json:"activeDeadlineSeconds,omitempty"`
	DNSPolicy                     core.DNSPolicy           `json:"dnsPolicy,omitempty"`
	SecurityContext               *core.PodSecurityContext `json:"securityContext,omitempty"`
	Hostname                      string                   `json:"hostname,omitempty"`
	Subdomain                     string                   `json:"subdomain,omitempty"`
	PriorityClassName             string                   `json:"priorityClassName,omitempty"`
	Priority                      *int32                   `json:"priority,omitempty"`
	DNSConfig                     *core.PodDNSConfig       `json:"dnsConfig,omitempty"`
	ReadinessGates                []core.PodReadinessGate  `json:"readinessGates,omitempty"`
	Service                       *K8sServiceSpec          `json:"service,omitempty"`

	CustomResourceDefinitions map[string]apiextensionsv1beta1.CustomResourceDefinitionSpec `json:"customResourceDefinitions,omitempty" yaml:"customResourceDefinitions,omitempty"`
	ServiceAccount            *ServiceAccountSpec                                          `yaml:"serviceAccount,omitempty"`
}

// Validate is defined on ProviderPod.
func (*K8sPodSpecV2) Validate() error {
	return nil
}

func parsePodSpecV2(in string) (*specs.PodSpec, error) {
	return nil, nil
}
