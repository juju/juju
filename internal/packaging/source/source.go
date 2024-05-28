// Copyright 2015 Canonical Ltd.
// Copyright 2015 Cloudbase Solutions SRL
// Licensed under the LGPLv3, see LICENCE file for details.

package source

import (
	"bytes"
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

// RenderSourceFile renders the current source based on a template it receives.
func (s *PackageSource) RenderSourceFile(fileTemplate *template.Template) (string, error) {
	return renderTemplate(fileTemplate, s)
}

// renderTemplate is a helper function which renders a given object to a given
// template and returns its output as a string.
func renderTemplate(t *template.Template, obj interface{}) (string, error) {
	var buf bytes.Buffer

	err := t.Execute(&buf, obj)
	if err != nil {
		return "", err
	}

	return buf.String(), nil
}
