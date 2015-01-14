// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package params

// AnnotationsGetResult holds entity annotations or retrieval error.
type AnnotationsGetResult struct {
	EntityTag   string
	Annotations map[string]string
	Error       ErrorResult
}

// AnnotationsGetResults holds annotations associated with entities.
type AnnotationsGetResults struct {
	Results []AnnotationsGetResult
}

// AnnotationsSet stores parameters for making Set call on Annotations client.
type AnnotationsSet struct {
	Annotations []EntityAnnotations
}

// EntityAnnotations stores annotations for an entity.
type EntityAnnotations struct {
	EntityTag   string
	Annotations map[string]string
}
