// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package description

import (
	"github.com/juju/schema"
)

// Instead of copy / pasting the Annotations, SetAnnotations, and the import
// three lines into every entity that has annotations, the Annotations_ helper
// type is provided for use in composition. This type is composed without a
// name so the methods get promoted so they satisfy the HasAnnotations
// interface.
//
// NOTE(mjs) - The type is exported due to a limitation with go-yaml under
// 1.6. Once that's fixed it should be possible to make it private again.
//
// NOTE(mjs) - The trailing underscore on the type name is to avoid collisions
// between the type name and the Annotations method. The underscore can go once
// the type becomes private again (revert to "annotations").
type Annotations_ map[string]string

// Annotations implements HasAnnotations.
func (a *Annotations_) Annotations() map[string]string {
	if a == nil {
		return nil
	}
	return *a
}

// SetAnnotations implements HasAnnotations.
func (a *Annotations_) SetAnnotations(annotations map[string]string) {
	*a = annotations
}

func (a *Annotations_) importAnnotations(valid map[string]interface{}) {
	if annotations := convertToStringMap(valid["annotations"]); annotations != nil {
		a.SetAnnotations(annotations)
	}
}

func addAnnotationSchema(fields schema.Fields, defaults schema.Defaults) {
	fields["annotations"] = schema.StringMap(schema.String())
	defaults["annotations"] = schema.Omit
}
