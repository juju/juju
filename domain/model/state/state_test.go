// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cloud"
	dbcloud "github.com/juju/juju/domain/cloud/state"
	"github.com/juju/juju/domain/credential"
	credentialstate "github.com/juju/juju/domain/credential/state"
	modelerrors "github.com/juju/juju/domain/model/errors"
	"github.com/juju/juju/domain/modelmanager/service"
	modelmanagerstate "github.com/juju/juju/domain/modelmanager/state"
	schematesting "github.com/juju/juju/domain/schema/testing"
)

type modelSuite struct {
	schematesting.ControllerSuite
}

const (
	modelUUID = service.UUID("12345")
)

var _ = gc.Suite(&modelSuite{})

func (m *modelSuite) SetUpTest(c *gc.C) {
	m.ControllerSuite.SetUpTest(c)

	cloudSt := dbcloud.NewState(m.TxnRunnerFactory())
	err := cloudSt.UpsertCloud(context.Background(), cloud.Cloud{
		Name:      "testmctestface",
		Type:      "ec2",
		AuthTypes: cloud.AuthTypes{cloud.AccessKeyAuthType, cloud.UserPassAuthType},
	})

	c.Assert(err, jc.ErrorIsNil)

	cred := cloud.NewNamedCredential(
		"foobar",
		cloud.AccessKeyAuthType,
		map[string]string{
			"foo": "foo val",
			"bar": "bar val",
		},
		false)

	credSt := credentialstate.NewState(m.TxnRunnerFactory())
	err = credSt.UpsertCloudCredential(
		context.Background(),
		"foobar",
		"testmctestface",
		"bob",
		cred,
	)
	c.Assert(err, jc.ErrorIsNil)

	mmSt := modelmanagerstate.NewState(m.TxnRunnerFactory())
	err = mmSt.Create(context.Background(), modelUUID)
	c.Assert(err, jc.ErrorIsNil)
}

func (m *modelSuite) TestModelSetCredential(c *gc.C) {
	st := NewState(m.TxnRunnerFactory())
	err := st.SetCloudCredential(
		context.Background(),
		modelUUID,
		credential.ID{
			Cloud: "testmctestface",
			Owner: "bob",
			Name:  "foobar",
		},
	)
	c.Assert(err, jc.ErrorIsNil)
}

func (m *modelSuite) TestModelSetNonExistentCredential(c *gc.C) {
	st := NewState(m.TxnRunnerFactory())
	err := st.SetCloudCredential(
		context.Background(),
		modelUUID,
		credential.ID{
			Cloud: "testmctestface",
			Owner: "bob",
			Name:  "noexist",
		},
	)
	c.Assert(err, jc.ErrorIs, errors.NotFound)
}

func (m *modelSuite) TestModelSetCredentialNoModel(c *gc.C) {
	st := NewState(m.TxnRunnerFactory())
	err := st.SetCloudCredential(
		context.Background(),
		"noexist",
		credential.ID{
			Cloud: "testmctestface",
			Owner: "bob",
			Name:  "foobar",
		},
	)
	c.Assert(err, jc.ErrorIs, modelerrors.NotFound)
}
