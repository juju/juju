// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package utils

import (
	"github.com/juju/juju/caas/kubernetes/provider/constants"
	"github.com/juju/juju/core/annotations"
	"github.com/juju/juju/environs/tags"
)

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
