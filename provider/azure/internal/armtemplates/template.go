// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package armtemplates

import "github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/compute/armcompute/v6"

const (
	// Schema defines a resource group schema.
	Schema = "https://schema.management.azure.com/schemas/2015-01-01/deploymentTemplate.json#"
	// SubscriptionSchema defines a subscription schema.
	SubscriptionSchema = "https://schema.management.azure.com/schemas/2018-05-01/subscriptionDeploymentTemplate.json#"

	contentVersion = "1.0.0.0"
)

// Template represents an Azure Resource Manager (ARM) Template.
// See: https://azure.microsoft.com/en-us/documentation/articles/resource-group-authoring-templates/
type Template struct {
	// Schema defines a subscription schema or resource group schema.
	Schema string
	// Resources contains the definitions of resources that will
	// be created by the template.
	Resources []Resource `json:"resources"`
	// Parameters contains the values of whatever parameters
	// are used by the template.
	Parameters any `json:"parameters,omitempty"`
}

// Map returns the template as a map, suitable for use in
// azure-sdk-for-go/arm/resources/resources/DeploymentProperties.Template.
func (t *Template) Map() (map[string]any, error) {
	schema := t.Schema
	if schema == "" {
		schema = Schema
	}
	m := map[string]any{
		"$schema":        schema,
		"contentVersion": contentVersion,
		"resources":      t.Resources,
	}
	if t.Parameters != nil {
		m["parameters"] = t.Parameters
	}
	return m, nil
}

// Sku represents an Azure SKU. Each API (compute/networking/storage)
// defines its own SKU types, but we use a common type because we
// don't require many fields.
type Sku struct {
	Name string `json:"name,omitempty"`
}

// Resource describes a template resource. For information on the
// individual fields, see https://azure.microsoft.com/en-us/documentation/articles/resource-group-authoring-templates/.
type Resource struct {
	APIVersion string                             `json:"apiVersion"`
	Type       string                             `json:"type"`
	Name       string                             `json:"name"`
	Location   string                             `json:"location,omitempty"`
	Tags       map[string]string                  `json:"tags,omitempty"`
	Comments   string                             `json:"comments,omitempty"`
	DependsOn  []string                           `json:"dependsOn,omitempty"`
	Properties any                                `json:"properties,omitempty"`
	Identity   *armcompute.VirtualMachineIdentity `json:"identity,omitempty"`
	Resources  []Resource                         `json:"resources,omitempty"`

	// Non-uniform attributes.
	Sku *Sku `json:"sku,omitempty"`
}
