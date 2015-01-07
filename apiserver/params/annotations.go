// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package params

// GetEntitiesAnnotationsResult holds entity annotations or retrieval error.
type GetEntitiesAnnotationsResult struct {
	Entity      Entity
	Annotations map[string]string
	Error       ErrorResult
}

// GetEntitiesAnnotationsResults holds annotations associated with entities.
type GetEntitiesAnnotationsResults struct {
	Results []GetEntitiesAnnotationsResult
}

// SetEntitiesAnnotations stores parameters for making the SetEntitiesAnnotations call.
type SetEntitiesAnnotations struct {
	Collection  Entities
	Annotations map[string]string
}
