// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package utils

import (
	"crypto/rand"
	"fmt"
	"io"
	"strings"

	"github.com/juju/errors"

	"github.com/juju/juju/core/annotations"
	"github.com/juju/juju/environs/tags"
	"github.com/juju/juju/internal/provider/caas/kubernetes/provider/constants"
)

type AnnotationKeySupplier func() string

// AnnotationsForStorage provides the annotations that should be placed on a
// storage object. The annotations returned by this function are storage
// specific only and should be combined with other annotations where
// appropriate.
func AnnotationsForStorage(name string, legacy bool) annotations.Annotation {
	return annotations.Annotation{
		AnnotationJujuStorageKey(legacy): name,
	}
}

// AnnotationJujuStorageKey returns the key used in annotations
// to describe the storage UUID.
func AnnotationJujuStorageKey(legacy bool) string {
	if legacy {
		return "juju-storage"
	}
	return annotationKey("storage", "name", false)
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
		return "juju-version"
	}
	return annotationKey("", "version", false)
}

// MakeK8sDomain builds and returns a Kubernetes resource domain for the
// provided components. Func is idempotent
func MakeK8sDomain(components ...string) string {
	var parts []string
	for _, v := range components {
		if v != "" {
			parts = append(parts, v)
		}
	}
	return strings.Join(append(parts, constants.Domain), ".")
}

func annotationKey(name, suffix string, legacy bool) string {
	if legacy {
		return constants.LegacyDomain + "/" + name
	}
	return MakeK8sDomain(name) + "/" + suffix
}

// AnnotationModelUUIDKey returns the key used in annotations
// to describe the model UUID.
func AnnotationModelUUIDKey(legacy bool) string {
	if legacy {
		return "juju-model"
	}
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
	if legacy {
		return annotationKey("is-controller", "", true)
	}
	return annotationKey("controller", "is-controller", false)
}

// AnnotationUnitKey returns the key used in annotations
// to describe the Juju unit.
func AnnotationUnitKey(legacy bool) string {
	return annotationKey("unit", "id", legacy)
}

// AnnotationCharmModifiedVersionKey returns the key used in annotations
// to describe the charm modified version.
func AnnotationCharmModifiedVersionKey(legacy bool) string {
	if legacy {
		return annotationKey("charm-modified-version", "", true)
	}
	return annotationKey("charm", "modified-version", false)
}

// AnnotationDisableNameKey returns the key used in annotations
// to describe the disabled name prefix.
func AnnotationDisableNameKey(legacy bool) string {
	if legacy {
		return annotationKey("disable-name-prefix", "", true)
	}
	return annotationKey("model", "disable-prefix", false)
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

// RandomPrefixFunc defines a function used to generate a random hex string.
type RandomPrefixFunc func() (string, error)

// RandomPrefix returns a random string for storage related annotations.
func RandomPrefix() (string, error) {
	var randPrefixBytes [4]byte
	if _, err := io.ReadFull(rand.Reader, randPrefixBytes[0:4]); err != nil {
		return "", errors.Trace(err)
	}
	return fmt.Sprintf("%x", randPrefixBytes), nil
}
