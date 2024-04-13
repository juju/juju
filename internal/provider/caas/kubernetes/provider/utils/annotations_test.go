// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package utils_test

import (
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/internal/provider/caas/kubernetes/provider/utils"
	"github.com/juju/juju/testing"
)

type annotationSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&annotationSuite{})

func (s *annotationSuite) TestAnnotations(c *gc.C) {
	c.Assert(utils.AnnotationJujuStorageKey(true), gc.DeepEquals, "juju-storage")
	c.Assert(utils.AnnotationJujuStorageKey(false), gc.DeepEquals, "storage.juju.is/name")

	c.Assert(utils.AnnotationVersionKey(true), gc.DeepEquals, "juju-version")
	c.Assert(utils.AnnotationVersionKey(false), gc.DeepEquals, "juju.is/version")

	c.Assert(utils.AnnotationModelUUIDKey(true), gc.DeepEquals, "juju-model")
	c.Assert(utils.AnnotationModelUUIDKey(false), gc.DeepEquals, "model.juju.is/id")

	c.Assert(utils.AnnotationControllerUUIDKey(true), gc.DeepEquals, "juju.io/controller")
	c.Assert(utils.AnnotationControllerUUIDKey(false), gc.DeepEquals, "controller.juju.is/id")

	c.Assert(utils.AnnotationControllerIsControllerKey(true), gc.DeepEquals, "juju.io/is-controller")
	c.Assert(utils.AnnotationControllerIsControllerKey(false), gc.DeepEquals, "controller.juju.is/is-controller")

	c.Assert(utils.AnnotationUnitKey(true), gc.DeepEquals, "juju.io/unit")
	c.Assert(utils.AnnotationUnitKey(false), gc.DeepEquals, "unit.juju.is/id")

	c.Assert(utils.AnnotationCharmModifiedVersionKey(true), gc.DeepEquals, "juju.io/charm-modified-version")
	c.Assert(utils.AnnotationCharmModifiedVersionKey(false), gc.DeepEquals, "charm.juju.is/modified-version")

	c.Assert(utils.AnnotationDisableNameKey(true), gc.DeepEquals, "juju.io/disable-name-prefix")
	c.Assert(utils.AnnotationDisableNameKey(false), gc.DeepEquals, "model.juju.is/disable-prefix")

	c.Assert(utils.AnnotationKeyApplicationUUID(true), gc.DeepEquals, "juju-app-uuid")
	c.Assert(utils.AnnotationKeyApplicationUUID(false), gc.DeepEquals, "app.juju.is/uuid")
}
