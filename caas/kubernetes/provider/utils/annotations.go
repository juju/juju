// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package utils

import (
	"fmt"
	"strings"

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

// AnnotationVersionKey returns the key used in annotations to describe the
// Juju version. Legacy controls if the key returns is a legacy annotation key
// or newer style.
func AnnotationVersionKey(legacy bool) string {
	if legacy {
		return constants.LegacyAnnotationVersion
	}
	return constants.AnnotationJujuVersion
}

// MakeK8sDomain builds and returns a Kubernetes resource domain for the
// provided components. Func is idempotent
func MakeK8sDomain(components ...string) string {
	return fmt.Sprintf("%s.%s", strings.Join(components, "."), constants.Domain)
}

func annotationKey(name, suffix string, legacy bool) string {
	if legacy {
		return constants.LegacyAnnotationPrefix + "/" + name
	}
	return MakeK8sDomain(name) + "/" + suffix
}

// AnnotationModelUUIDKey returns the key used in annotations
// to describe the model UUID.
func AnnotationModelUUIDKey(legacy bool) string {
	return annotationKey("model", "id", legacy)
}

// AnnotationControllerUUIDKey returns the key used in annotations
// to describe the controller UUID.
func AnnotationControllerUUIDKey(legacy bool) string {
	return annotationKey("controller", "id", legacy)
}

// AnnotationControllerIsControllerKey returns the key used in annotations
// to describe if this pod is a controller pod.
func AnnotationControllerIsControllerKey(legacy bool) string {
	return annotationKey("controller", "is-controller", legacy)
}

// AnnotationUnit returns the key used in annotations
// to describe the Juju unit.
func AnnotationUnit(legacy bool) string {
	return annotationKey("unit", "id", legacy)
}

// AnnotationCharmModifiedVersionKey returns the key used in annotations
// to describe the charm modified version.
func AnnotationCharmModifiedVersionKey(legacy bool) string {
	return annotationKey("charm", "modified-version", legacy)
}

// AnnotationDisableNameKey returns the key used in annotations
// to describe the disabled name prefix.
func AnnotationDisableNameKey(legacy bool) string {
	return annotationKey("model", "disable-prefix", legacy)
}

// AnnotationKeyApplicationUUID is the key of annotation for recording pvc unique ID.
func AnnotationKeyApplicationUUID(legacy bool) string {
	if legacy {
		return "juju-app-uuid"
	}
	return MakeK8sDomain("app") + "/uuid"
}

// ResourceTagsToAnnotations creates annotations from the resource tags.
func ResourceTagsToAnnotations(in map[string]string, legacy bool) annotations.Annotation {
	tagsAnnotationsMap := map[string]string{
		tags.JujuController: AnnotationControllerUUIDKey(legacy),
		tags.JujuModel:      AnnotationModelUUIDKey(legacy),
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
