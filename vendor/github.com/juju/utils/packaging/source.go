// Copyright 2015 Canonical Ltd.
// Copyright 2015 Cloudbase Solutions SRL
// Licensed under the LGPLv3, see LICENCE file for details.

// Package packaging abstracts away differences between package managers like
// apt and yum and allows for easy extension for other package managers/distros.
package packaging

import (
	"text/template"
)

// PackageSource contains all the data required for a package source.
type PackageSource struct {
	Name string `yaml:"-"`
	URL  string `yaml:"source"`
	Key  string `yaml:"key,omitempty"`
}

// KeyFileName returns the name of this source's keyfile.
func (s *PackageSource) KeyFileName() string {
	return s.Name + ".key"
}

// RenderSourceFile renders the current source based on a template it recieves.
func (s *PackageSource) RenderSourceFile(fileTemplate *template.Template) (string, error) {
	return renderTemplate(fileTemplate, s)
}
