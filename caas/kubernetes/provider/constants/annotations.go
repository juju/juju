// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package k8sconstants

func AnnotationKey(name string) string {
	return AnnotationPrefix + "/" + name
}

const (
	// AnnotationPrefix of juju annotations
	AnnotationPrefix = "juju.io"
)

func AnnotationModelUUIDKey() string {
	return AnnotationKey("model")
}

func AnnotationControllerUUIDKey() string {
	return AnnotationKey("controller")
}

func AnnotationControllerIsControllerKey() string {
	return AnnotationKey("is-controller")
}

func AnnotationUnit() string {
	return AnnotationKey("unit")
}

func AnnotationCharmModifiedVersionKey() string {
	return AnnotationKey("charm-modified-version")
}
