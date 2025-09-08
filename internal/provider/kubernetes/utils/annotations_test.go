// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package utils_test

import (
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/caas/kubernetes/provider/constants"
	"github.com/juju/juju/caas/kubernetes/provider/utils"
	"github.com/juju/juju/testing"
)

type annotationSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&annotationSuite{})

func (s *annotationSuite) TestAnnotations(c *gc.C) {
	c.Assert(utils.AnnotationJujuStorageKey(constants.LegacyLabelVersion), gc.DeepEquals, "juju-storage")
	c.Assert(utils.AnnotationJujuStorageKey(constants.LabelVersion1), gc.DeepEquals, "storage.juju.is/name")

	c.Assert(utils.AnnotationVersionKey(constants.LegacyLabelVersion), gc.DeepEquals, "juju-version")
	c.Assert(utils.AnnotationVersionKey(constants.LabelVersion1), gc.DeepEquals, "juju.is/version")

	c.Assert(utils.AnnotationModelUUIDKey(constants.LegacyLabelVersion), gc.DeepEquals, "juju-model")
	c.Assert(utils.AnnotationModelUUIDKey(constants.LabelVersion1), gc.DeepEquals, "model.juju.is/id")

	c.Assert(utils.AnnotationControllerUUIDKey(constants.LegacyLabelVersion), gc.DeepEquals, "juju.io/controller")
	c.Assert(utils.AnnotationControllerUUIDKey(constants.LabelVersion1), gc.DeepEquals, "controller.juju.is/id")

	c.Assert(utils.AnnotationControllerIsControllerKey(constants.LegacyLabelVersion), gc.DeepEquals, "juju.io/is-controller")
	c.Assert(utils.AnnotationControllerIsControllerKey(constants.LabelVersion1), gc.DeepEquals, "controller.juju.is/is-controller")

	c.Assert(utils.AnnotationUnitKey(constants.LegacyLabelVersion), gc.DeepEquals, "juju.io/unit")
	c.Assert(utils.AnnotationUnitKey(constants.LabelVersion1), gc.DeepEquals, "unit.juju.is/id")

	c.Assert(utils.AnnotationCharmModifiedVersionKey(constants.LegacyLabelVersion), gc.DeepEquals, "juju.io/charm-modified-version")
	c.Assert(utils.AnnotationCharmModifiedVersionKey(constants.LabelVersion1), gc.DeepEquals, "charm.juju.is/modified-version")

	c.Assert(utils.AnnotationDisableNameKey(constants.LegacyLabelVersion), gc.DeepEquals, "juju.io/disable-name-prefix")
	c.Assert(utils.AnnotationDisableNameKey(constants.LabelVersion1), gc.DeepEquals, "model.juju.is/disable-prefix")

	c.Assert(utils.AnnotationKeyApplicationUUID(constants.LegacyLabelVersion), gc.DeepEquals, "juju-app-uuid")
	c.Assert(utils.AnnotationKeyApplicationUUID(constants.LabelVersion1), gc.DeepEquals, "app.juju.is/uuid")
	c.Assert(utils.AnnotationKeyApplicationUUID(constants.LabelVersion2), gc.DeepEquals, "app.juju.is/uuid")
}
