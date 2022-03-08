// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package params

// AnnotationsGetResult holds entity annotations or retrieval error.
type AnnotationsGetResult struct {
	EntityTag   string            `json:"entity"`
	Annotations map[string]string `json:"annotations"`
	Error       ErrorResult       `json:"error,omitempty"`
}

// AnnotationsGetResults holds annotations associated with entities.
type AnnotationsGetResults struct {
	Results []AnnotationsGetResult `json:"results"`
}

// AnnotationsSet stores parameters for making Set call on Annotations client.
type AnnotationsSet struct {
	Annotations []EntityAnnotations `json:"annotations"`
}

// EntityAnnotations stores annotations for an entity.
type EntityAnnotations struct {
	EntityTag   string            `json:"entity"`
	Annotations map[string]string `json:"annotations"`
}
