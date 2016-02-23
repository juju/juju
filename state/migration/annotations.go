// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migration

// Instead of copy / pasting the Annotations, SetAnnotations, and the import
// three lines into every entity that has annotations, we provide a helper
// struct used in composition. Each entity still needs to define the
// annotations map to be serialized. For the two locations where entities are
// created (the new<entity> function, and the import functions), the pointer
// to the annotations needs to be set. This allows the accessor and setter
// methods to work. This type is composed without a name so the methods get
// promoted so they satisfy the HasAnnotations interface.
type hasAnnotations struct {
	annotations *map[string]interface{}
}

// Annotations implements HasAnnotations.
func (a *hasAnnotations) Annotations() map[string]interface{} {
	return *a.annotations
}

// SetAnnotations implements HasAnnotations.
func (a *hasAnnotations) SetAnnotations(annotations map[string]interface{}) {
	*a.annotations = annotations
}

func (a *hasAnnotations) importAnnotations(valid map[string]interface{}) {
	if annotations, ok := valid["annotations"]; ok {
		a.SetAnnotations(annotations.(map[string]interface{}))
	}
}
