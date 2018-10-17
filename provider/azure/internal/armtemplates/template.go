// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package armtemplates

import "github.com/Azure/azure-sdk-for-go/arm/storage"

const (
	schema         = "http://schema.management.azure.com/schemas/2015-01-01/deploymentTemplate.json#"
	contentVersion = "1.0.0.0"
)

// Template represents an Azure Resource Manager (ARM) Template.
// See: https://azure.microsoft.com/en-us/documentation/articles/resource-group-authoring-templates/
type Template struct {
	// Resources contains the definitions of resources that will
	// be created by the template.
	Resources []Resource `json:"resources"`
}

// Map returns the template as a map, suitable for use in
// azure-sdk-for-go/arm/resources/resources/DeploymentProperties.Template.
func (t *Template) Map() (map[string]interface{}, error) {
	m := map[string]interface{}{
		"$schema":        schema,
		"contentVersion": contentVersion,
		"resources":      t.Resources,
	}
	return m, nil
}

// Resource describes a template resource. For information on the
// individual fields, see https://azure.microsoft.com/en-us/documentation/articles/resource-group-authoring-templates/.
type Resource struct {
	APIVersion string            `json:"apiVersion"`
	Type       string            `json:"type"`
	Name       string            `json:"name"`
	Location   string            `json:"location,omitempty"`
	Tags       map[string]string `json:"tags,omitempty"`
	Comments   string            `json:"comments,omitempty"`
	DependsOn  []string          `json:"dependsOn,omitempty"`
	Properties interface{}       `json:"properties,omitempty"`
	Resources  []Resource        `json:"resources,omitempty"`

	// Non-uniform attributes.
	StorageSku *storage.Sku `json:"sku,omitempty"`
}
