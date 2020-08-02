// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider

import (
	"github.com/juju/juju/core/annotations"
)

const (
	// AnnotationJujuStorageName is the Juju annotation that represents a
	// storage objects associated Juju name.
	AnnotationJujuStorageName = "storage.juju.is/name"

	// AnnotationJujuVersion is the version annotation used on operator
	// deployments.
	AnnotationJujuVersion = "juju.is/version"

	// legacyAnnotationStorageName is the legacy annotation used by Juju for
	// dictating storage name on k8s storage objects.
	legacyAnnotationStorageName = "juju-storage"

	// legacyAnnotationVersion is the legacy annotation used by Juju for
	// dictating juju agent version on operators.
	legacyAnnotationVersion = "juju-version"
)

type AnnotationKeySupplier func() string

// AnnotationsForStorage provides the annotations that should be placed on a
// storage object. The annotations returned by this function are storage
// specific only and should be combined with other annotations where
// appropriate.
func AnnotationsForStorage(name string, legacy bool) annotations.Annotation {
	if legacy {
		return annotations.Annotation{
			legacyAnnotationStorageName: name,
		}
	}
	return annotations.Annotation{
		AnnotationJujuStorageName: name,
	}
}

// AnnotationsForVersion provides the annotations that should be placed on an
// object that requires juju version information. The annotations returned by
// this function are version specific and may need to be combined with other
// annotations for a complete set.
func AnnotationsForVersion(vers string, legacy bool) annotations.Annotation {
	return annotations.Annotation{
		AnnotationVersionKey(legacy): vers,
	}
}

// AnnotationVersionKey returns the key used un in annotations to describe the
// Juju version. Legacy controls if the key returns is a legacy annotation key
// or newer style.
func AnnotationVersionKey(legacy bool) string {
	if legacy {
		return legacyAnnotationVersion
	}
	return AnnotationJujuVersion
}
