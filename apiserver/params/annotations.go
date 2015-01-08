// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package params

// AnnotationsGetResult holds entity annotations or retrieval error.
type AnnotationsGetResult struct {
	Entity      Entity
	Annotations map[string]string
	Error       ErrorResult
}

// AnnotationsGetResults holds annotations associated with entities.
type AnnotationsGetResults struct {
	Results []AnnotationsGetResult
}

// AnnotationsSet stores parameters for making the SetEntitiesAnnotations call.
type AnnotationsSet struct {
	Annotations []EntityAnnotations
}

// EntityAnnotations stores annotations for entities.
type EntityAnnotations struct {
	Entities    Entities
	Annotations map[string]string
}
