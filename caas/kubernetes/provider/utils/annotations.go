// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package utils

import (
	"crypto/rand"
	"fmt"
	"io"
	"strings"

	"github.com/juju/errors"

	"github.com/juju/juju/caas/kubernetes/provider/constants"
	"github.com/juju/juju/core/annotations"
	"github.com/juju/juju/environs/tags"
)

type AnnotationKeySupplier func() string

// AnnotationsForStorage provides the annotations that should be placed on a
// storage object. The annotations returned by this function are storage
// specific only and should be combined with other annotations where
// appropriate.
func AnnotationsForStorage(name string, labelVersion constants.LabelVersion) annotations.Annotation {
	return annotations.Annotation{
		AnnotationJujuStorageKey(labelVersion): name,
	}
}

// AnnotationJujuStorageKey returns the key used in annotations
// to describe the storage UUID.
func AnnotationJujuStorageKey(labelVersion constants.LabelVersion) string {
	if labelVersion == constants.LegacyLabelVersion {
		return "juju-storage"
	}
	return annotationKey("storage", "name", labelVersion)
}

// AnnotationsForVersion provides the annotations that should be placed on an
// object that requires juju version information. The annotations returned by
// this function are version specific and may need to be combined with other
// annotations for a complete set.
func AnnotationsForVersion(vers string, labelVersion constants.LabelVersion) annotations.Annotation {
	return annotations.Annotation{
		AnnotationVersionKey(labelVersion): vers,
	}
}

// AnnotationVersionKey returns the key used in annotations to describe the
// Juju version. Legacy controls if the key returns is a legacy annotation key
// or newer style.
func AnnotationVersionKey(labelVersion constants.LabelVersion) string {
	if labelVersion == constants.LegacyLabelVersion {
		return "juju-version"
	}
	return annotationKey("", "version", labelVersion)
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

func annotationKey(name, suffix string, labelVersion constants.LabelVersion) string {
	if labelVersion == constants.LegacyLabelVersion {
		return constants.LegacyDomain + "/" + name
	}
	return MakeK8sDomain(name) + "/" + suffix
}

// AnnotationModelUUIDKey returns the key used in annotations
// to describe the model UUID.
func AnnotationModelUUIDKey(labelVersion constants.LabelVersion) string {
	if labelVersion == constants.LegacyLabelVersion {
		return "juju-model"
	}
	return annotationKey("model", "id", labelVersion)
}

// AnnotationControllerUUIDKey returns the key used in annotations
// to describe the controller UUID.
func AnnotationControllerUUIDKey(labelVersion constants.LabelVersion) string {
	return annotationKey("controller", "id", labelVersion)
}

// AnnotationControllerIsControllerKey returns the key used in annotations
// to describe if this pod is a controller pod.
func AnnotationControllerIsControllerKey(labelVersion constants.LabelVersion) string {
	if labelVersion == constants.LegacyLabelVersion {
		return annotationKey("is-controller", "", labelVersion)
	}
	return annotationKey("controller", "is-controller", labelVersion)
}

// AnnotationUnitKey returns the key used in annotations
// to describe the Juju unit.
func AnnotationUnitKey(labelVersion constants.LabelVersion) string {
	return annotationKey("unit", "id", labelVersion)
}

// AnnotationCharmModifiedVersionKey returns the key used in annotations
// to describe the charm modified version.
func AnnotationCharmModifiedVersionKey(labelVersion constants.LabelVersion) string {
	if labelVersion == constants.LegacyLabelVersion {
		return annotationKey("charm-modified-version", "", labelVersion)
	}
	return annotationKey("charm", "modified-version", labelVersion)
}

// AnnotationDisableNameKey returns the key used in annotations
// to describe the disabled name prefix.
func AnnotationDisableNameKey(labelVersion constants.LabelVersion) string {
	if labelVersion == constants.LegacyLabelVersion {
		return annotationKey("disable-name-prefix", "", labelVersion)
	}
	return annotationKey("model", "disable-prefix", labelVersion)
}

// AnnotationKeyApplicationUUID is the key of annotation for recording pvc unique ID.
func AnnotationKeyApplicationUUID(labelVersion constants.LabelVersion) string {
	if labelVersion == constants.LegacyLabelVersion {
		return "juju-app-uuid"
	}
	return MakeK8sDomain("app") + "/uuid"
}

// ResourceTagsToAnnotations creates annotations from the resource tags.
func ResourceTagsToAnnotations(in map[string]string, labelVersion constants.LabelVersion) annotations.Annotation {
	tagsAnnotationsMap := map[string]string{
		tags.JujuController: AnnotationControllerUUIDKey(labelVersion),
		tags.JujuModel:      AnnotationModelUUIDKey(labelVersion),
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
