// Copyright 2024 Ca
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	_ "github.com/juju/juju/provider/all" // Import all the providers
)

// providerNames maps the internal Juju provider name to a "nice"
// human-readable title.
var providerNames = map[string]string{
	"azure":      "Microsoft Azure",
	"ec2":        "Amazon EC2",
	"equinix":    "Equinix Metal",
	"gce":        "Google GCE",
	"kubernetes": "Kubernetes",
	"lxd":        "LXD",
	"maas":       "MAAS",
	"manual":     "Manual",
	"oci":        "Oracle OCI",
	"openstack":  "OpenStack",
	"vsphere":    "VMware vSphere",
}

func main() {
	docsDir := mustEnv("DOCS_DIR")
	cloudDocsDir := filepath.Join(docsDir, "cloud")
	// Create dir if it doesn't exist
	check(os.MkdirAll(cloudDocsDir, 0777))

	for _, providerName := range environs.RegisteredProviders() {
		file, err := os.Create(filepath.Join(cloudDocsDir, providerName+".md"))
		check(err)
		(&providerInfo{name: providerName}).print(file)
		check(file.Close())
	}
}

// providerInfo stores all the fields needed to print the cloud information
// document.
type providerInfo struct {
	name     string
	niceName string
	provider environs.EnvironProvider
}

// print prints the information doc about the given provider to the given
// io.Writer.
func (p *providerInfo) print(w io.Writer) {
	p.niceName = p.name
	if niceName, ok := providerNames[p.name]; ok {
		p.niceName = niceName
	}

	var err error
	p.provider, err = environs.Provider(p.name)
	check(err)

	// Print supported authentication types
	fprintf(w, "## Supported authentication types\n\n")
	for authType, schema := range p.provider.CredentialSchemas() {
		fprintf(w, "### %s\n", authType)
		fprintf(w, "Attributes:\n")
		for _, attr := range schema {
			requiredStr := "required"
			if attr.Optional {
				requiredStr = "optional"
			}
			fprintf(w, "- %s: %s (%s)\n", attr.Name, attr.Description, requiredStr)
		}
		fprintf(w, "\n")
	}

	p.maybePrintProviderSpecificConfig(w)
}

// maybePrintProviderSpecificConfig attempts to print information about the
// model config keys specific to this provider. If the provider doesn't support
// the required interfaces, it will return without printing anything.
func (p *providerInfo) maybePrintProviderSpecificConfig(w io.Writer) {
	schemaGetter, ok := p.provider.(environs.ProviderSchema)
	if !ok {
		return
	}
	// fullSchema includes all the model config keys, including generic ones
	fullSchema := schemaGetter.Schema()

	configSchemaGetter, ok := p.provider.(config.ConfigSchemaSource)
	if !ok {
		return
	}
	// providerSchema lists only the keys that are specific to this provider.
	// However, it does not include any information about the keys - we have to
	// get this from fullSchema.
	providerSchema := configSchemaGetter.ConfigSchema()
	defaultValues := configSchemaGetter.ConfigDefaults()

	fprintf(w, "## Model config keys specific to the %s cloud\n\n", p.niceName)

	for key := range providerSchema {
		info := fullSchema[key]

		fprintf(w, "### %s\n", key)
		fprintf(w, "%s\n\n", info.Description)
		if info.Example != nil {
			fprintf(w, "Example: %v\n", info.Example)
		}

		fprintf(w, "| | |\n|-|-|\n")
		fprintf(w, "| type | %s |\n", info.Type)
		if defVal, ok := defaultValues[key]; ok {
			fprintf(w, "| default value | %#v |\n", defVal)
		}
		fprintf(w, "| immutable | %t |\n", info.Immutable)
		fprintf(w, "| mandatory | %t |\n", info.Mandatory)
		fprintf(w, "\n")
	}
}

// UTILITY FUNCTIONS

// check panics if the provided error is not nil.
func check(err error) {
	if err != nil {
		panic(err)
	}
}

func fprintf(w io.Writer, format string, a ...any) {
	_, err := fmt.Fprintf(w, format, a...)
	check(err)
}

// Returns the value of the given environment variable, panicking if the var
// is not set.
func mustEnv(key string) string {
	val, ok := os.LookupEnv(key)
	if !ok {
		panic(fmt.Sprintf("env var %q not set", key))
	}
	return val
}
