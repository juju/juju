// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package utils_test

import (
	stdtesting "testing"

	"github.com/juju/tc"

	"github.com/juju/juju/internal/provider/kubernetes/constants"
	"github.com/juju/juju/internal/provider/kubernetes/utils"
	"github.com/juju/juju/internal/testing"
)

type annotationSuite struct {
	testing.BaseSuite
}

func TestAnnotationSuite(t *stdtesting.T) {
	tc.Run(t, &annotationSuite{})
}

func (s *annotationSuite) TestAnnotations(c *tc.C) {
	c.Assert(utils.AnnotationJujuStorageKey(constants.LegacyLabelVersion), tc.DeepEquals, "juju-storage")
	c.Assert(utils.AnnotationJujuStorageKey(constants.LabelVersion1), tc.DeepEquals, "storage.juju.is/name")

	c.Assert(utils.AnnotationVersionKey(constants.LegacyLabelVersion), tc.DeepEquals, "juju-version")
	c.Assert(utils.AnnotationVersionKey(constants.LabelVersion1), tc.DeepEquals, "juju.is/version")

	c.Assert(utils.AnnotationModelUUIDKey(constants.LegacyLabelVersion), tc.DeepEquals, "juju-model")
	c.Assert(utils.AnnotationModelUUIDKey(constants.LabelVersion1), tc.DeepEquals, "model.juju.is/id")

	c.Assert(utils.AnnotationControllerUUIDKey(constants.LegacyLabelVersion), tc.DeepEquals, "juju.io/controller")
	c.Assert(utils.AnnotationControllerUUIDKey(constants.LabelVersion1), tc.DeepEquals, "controller.juju.is/id")

	c.Assert(utils.AnnotationControllerIsControllerKey(constants.LegacyLabelVersion), tc.DeepEquals, "juju.io/is-controller")
	c.Assert(utils.AnnotationControllerIsControllerKey(constants.LabelVersion1), tc.DeepEquals, "controller.juju.is/is-controller")

	c.Assert(utils.AnnotationUnitKey(constants.LegacyLabelVersion), tc.DeepEquals, "juju.io/unit")
	c.Assert(utils.AnnotationUnitKey(constants.LabelVersion1), tc.DeepEquals, "unit.juju.is/id")

	c.Assert(utils.AnnotationCharmModifiedVersionKey(constants.LegacyLabelVersion), tc.DeepEquals, "juju.io/charm-modified-version")
	c.Assert(utils.AnnotationCharmModifiedVersionKey(constants.LabelVersion1), tc.DeepEquals, "charm.juju.is/modified-version")

	c.Assert(utils.AnnotationDisableNameKey(constants.LegacyLabelVersion), tc.DeepEquals, "juju.io/disable-name-prefix")
	c.Assert(utils.AnnotationDisableNameKey(constants.LabelVersion1), tc.DeepEquals, "model.juju.is/disable-prefix")

	c.Assert(utils.AnnotationKeyApplicationUUID(constants.LegacyLabelVersion), tc.DeepEquals, "juju-app-uuid")
	c.Assert(utils.AnnotationKeyApplicationUUID(constants.LabelVersion1), tc.DeepEquals, "app.juju.is/uuid")
	c.Assert(utils.AnnotationKeyApplicationUUID(constants.LabelVersion2), tc.DeepEquals, "app.juju.is/uuid")
}
