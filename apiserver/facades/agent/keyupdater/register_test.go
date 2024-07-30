// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package keyupdater

import (
	"github.com/juju/names/v5"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	apiservertesting "github.com/juju/juju/apiserver/testing"
)

type registerSuite struct {
	authorizer   apiservertesting.FakeAuthorizer
	modelContext *MockModelContext
	machineTag   names.MachineTag
}

var _ = gc.Suite(&registerSuite{})

func (r *registerSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	r.modelContext = NewMockModelContext(ctrl)
	return ctrl
}

func (r *registerSuite) SetUpTest(c *gc.C) {
	r.machineTag = names.NewMachineTag("0")

	// The default auth is as a controller
	r.authorizer = apiservertesting.FakeAuthorizer{
		Tag: r.machineTag,
	}
}

// TestMakeKeyUpdaterAPIRefusesNonMachineAgent is checking that if we try and
// make the facade with a non machine entity the facade fails to construct with
// [apiservererrors.ErrPerm] error.
func (r *registerSuite) TestMakeKeyUpdaterAPIRefusesNonMachineAgent(c *gc.C) {
	defer r.setupMocks(c).Finish()

	r.authorizer.Tag = names.NewUnitTag("ubuntu/1")
	r.modelContext.EXPECT().Auth().Return(r.authorizer)

	_, err := makeKeyUpdaterAPI(r.modelContext)
	c.Check(err, gc.ErrorMatches, "permission denied")
	c.Check(err, jc.ErrorIs, apiservererrors.ErrPerm)
}
