// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider

import (
	"github.com/juju/juju/environs/config"
	"github.com/juju/schema"
	"gopkg.in/juju/environschema.v1"
	core "k8s.io/api/core/v1"
)

const (
	defaultServiceType           = string(core.ServiceTypeClusterIP)
	defaultIngressClass          = "nginx"
	defaultIngressSSLRedirect    = false
	defaultIngressSSLPassthrough = false
	defaultIngressAllowHTTPKey   = false

	serviceTypeConfigKey               = "kubernetes-service-type"
	serviceExternalIPsConfigKey        = "kubernetes-service-external-ips"
	serviceTargetPortConfigKey         = "kubernetes-service-target-port"
	serviceLoadBalancerIPKey           = "kubernetes-service-loadbalancer-ip"
	serviceLoadBalancerSourceRangesKey = "kubernetes-service-loadbalancer-sourceranges"
	serviceExternalNameKey             = "kubernetes-service-externalname"

	ingressClassKey          = "kubernetes-ingress-class"
	ingressSSLRedirectKey    = "kubernetes-ingress-ssl-redirect"
	ingressSSLPassthroughKey = "kubernetes-ingress-ssl-passthrough"
	ingressAllowHTTPKey      = "kubernetes-ingress-allow-http"
)

var configFields = environschema.Fields{
	serviceTypeConfigKey: {
		Description: "determines how the Service is exposed",
		Type:        environschema.Tstring,
		Group:       environschema.ProviderGroup,
	},
	serviceExternalIPsConfigKey: {
		Description: "list of IP addresses for which nodes in the cluster will also accept traffic",
		Type:        environschema.Tstring,
		Group:       environschema.ProviderGroup,
	},
	serviceTargetPortConfigKey: {
		Description: "name or number of the port to access on the pods targeted by the service",
		Type:        environschema.Tstring,
		Group:       environschema.ProviderGroup,
	},
	serviceLoadBalancerIPKey: {
		Description: "LoadBalancer will get created with the IP specified in this field",
		Type:        environschema.Tstring,
		Group:       environschema.ProviderGroup,
	},
	serviceLoadBalancerSourceRangesKey: {
		Description: "traffic through the load-balancer will be restricted to the specified client IPs",
		Type:        environschema.Tstring,
		Group:       environschema.ProviderGroup,
	},
	serviceExternalNameKey: {
		Description: "external reference that kubedns or equivalent will return as a CNAME record",
		Type:        environschema.Tstring,
		Group:       environschema.ProviderGroup,
	},
	ingressClassKey: {
		Description: "the class of the ingress controller to be used by the ingress resource",
		Type:        environschema.Tstring,
		Group:       environschema.ProviderGroup,
	},
	ingressSSLRedirectKey: {
		Description: "whether to redirect SSL traffic to the ingress controller",
		Type:        environschema.Tbool,
		Group:       environschema.ProviderGroup,
	},
	ingressSSLPassthroughKey: {
		Description: "whether to passthrough SSL traffic to the ingress controller",
		Type:        environschema.Tbool,
		Group:       environschema.ProviderGroup,
	},
	ingressAllowHTTPKey: {
		Description: "whether to allow HTTP traffic to the ingress controller",
		Type:        environschema.Tbool,
		Group:       environschema.ProviderGroup,
	},
}

var schemaDefaults = schema.Defaults{
	serviceTypeConfigKey:     defaultServiceType,
	ingressClassKey:          defaultIngressClass,
	ingressSSLRedirectKey:    defaultIngressSSLRedirect,
	ingressSSLPassthroughKey: defaultIngressSSLPassthrough,
	ingressAllowHTTPKey:      defaultIngressAllowHTTPKey,
}

// ConfigSchema returns the configuration schema for
// a kubernetes provider config.
func ConfigSchema() environschema.Fields {
	return configFields
}

// ConfigDefaults returns the default values for
// a kubernetes configuration.
func ConfigDefaults() schema.Defaults {
	return schemaDefaults
}

type environConfig struct {
	*config.Config
	attrs map[string]interface{}
}
