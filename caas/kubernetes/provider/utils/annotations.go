// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package utils

import (
	"crypto/rand"
	"fmt"
	"io"

	"github.com/juju/errors"

	"github.com/juju/juju/caas/kubernetes/provider/constants"
	"github.com/juju/juju/core/annotations"
	"github.com/juju/juju/environs/tags"
)

// ResourceTagsToAnnotations merges resources tags with Juju specific annotations.
func ResourceTagsToAnnotations(in map[string]string) annotations.Annotation {
	tagsAnnotationsMap := map[string]string{
		tags.JujuController: constants.AnnotationControllerUUIDKey(),
		tags.JujuModel:      constants.AnnotationModelUUIDKey(),
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
