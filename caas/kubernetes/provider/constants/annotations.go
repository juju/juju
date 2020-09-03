// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package constants

// AnnotationKey returns a key for annotations.
func AnnotationKey(name string) string {
	return AnnotationPrefix + "/" + name
}

const (
	// AnnotationPrefix of juju annotations
	AnnotationPrefix = "juju.io"
)

// AnnotationModelUUIDKey returns the annotation key for model UUID.
func AnnotationModelUUIDKey() string {
	return AnnotationKey("model")
}

// AnnotationControllerUUIDKey returns the annotation key for controller UUID.
func AnnotationControllerUUIDKey() string {
	return AnnotationKey("controller")
}

// AnnotationControllerIsControllerKey returns the annotation key for `is-controller`.
func AnnotationControllerIsControllerKey() string {
	return AnnotationKey("is-controller")
}

// AnnotationUnit returns the annotation key for the unit.
func AnnotationUnit() string {
	return AnnotationKey("unit")
}

// AnnotationCharmModifiedVersionKey returns the annotation key for `charm-modified-version`.
func AnnotationCharmModifiedVersionKey() string {
	return AnnotationKey("charm-modified-version")
}

// AnnotationApplicationUUIDKey returns the annotation key for application UUID.
func AnnotationApplicationUUIDKey() string {
	return AnnotationKey("app-uuid")
}
