// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package description

import (
	"github.com/juju/schema"
)

// Instead of copy / pasting the Annotations, SetAnnotations, and the import
// three lines into every entity that has annotations, we provide a helper
// struct used in composition. This allows the accessor and setter methods to
// work. This type is composed without a name so the methods get promoted so
// they satisfy the HasAnnotations interface, but it does require that the
// name is serialized as "annotations".
type annotations map[string]string

// Annotations implements HasAnnotations.
func (a *annotations) Annotations() map[string]string {
	if a == nil {
		return nil
	}
	return *a
}

// SetAnnotations implements HasAnnotations.
func (a *annotations) SetAnnotations(annotations map[string]string) {
	*a = annotations
}

func (a *annotations) importAnnotations(valid map[string]interface{}) {
	if annotations := convertToStringMap(valid["annotations"]); annotations != nil {
		a.SetAnnotations(annotations)
	}
}

func addAnnotationSchema(fields schema.Fields, defaults schema.Defaults) {
	fields["annotations"] = schema.StringMap(schema.String())
	defaults["annotations"] = schema.Omit
}
