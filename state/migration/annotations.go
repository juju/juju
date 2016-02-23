// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migration

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
