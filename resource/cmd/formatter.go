// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cmd

import (
	"github.com/juju/juju/resource"
)

type specListFormatter struct {
	specs []resource.Spec
}

func newSpecListFormatter(specs []resource.Spec) *specListFormatter {
	// Note that unlike the "juju status" code, we don't worry
	// about "compatVersion".
	lf := specListFormatter{
		specs: specs,
	}
	return &lf
}

func (lf *specListFormatter) format() []FormattedSpec {
	if lf.specs == nil {
		return nil
	}

	var formatted []FormattedSpec
	for _, spec := range lf.specs {
		formatted = append(formatted, FormatSpec(spec))
	}
	return formatted
}

// FormatSpec converts the resource spec into a FormattedSpec.
func FormatSpec(spec resource.Spec) FormattedSpec {
	info := spec.Definition
	return FormattedSpec{
		Name:     info.Name,
		Type:     info.Type.String(),
		Path:     info.Path,
		Comment:  info.Comment,
		Origin:   spec.Origin.String(),
		Revision: spec.Revision,
	}
}
