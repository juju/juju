// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package utils

import (
	"github.com/juju/juju/caas/kubernetes/provider/constants"
	"github.com/juju/juju/core/annotations"
	"github.com/juju/juju/environs/tags"
)

type AnnotationKeySupplier func() string

// AnnotationsForStorage provides the annotations that should be placed on a
// storage object. The annotations returned by this function are storage
// specific only and should be combined with other annotations where
// appropriate.
func AnnotationsForStorage(name string, legacy bool) annotations.Annotation {
	if legacy {
		return annotations.Annotation{
			constants.LegacyAnnotationStorageName: name,
		}
	}
	return annotations.Annotation{
		constants.AnnotationJujuStorageName: name,
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
		return constants.LegacyAnnotationVersion
	}
	return constants.AnnotationJujuVersion
}

func ResourceTagsToAnnotations(in map[string]string) annotations.Annotation {
	tagsAnnotationsMap := map[string]string{
		tags.JujuController: constants.AnnotationControllerUUIDKey,
		tags.JujuModel:      constants.AnnotationModelUUIDKey,
	}

	out := annotations.New(nil)
	for k, v := range in {
		if annotationKey, ok := tagsAnnotationsMap[k]; ok {
			k = annotationKey
		}
		out.Add(k, v)
	}
	return out
}
