// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cloud"
	cloudtesting "github.com/juju/juju/core/cloud/testing"
	corecredential "github.com/juju/juju/core/credential"
	coremodel "github.com/juju/juju/core/model"
	modeltesting "github.com/juju/juju/core/model/testing"
	"github.com/juju/juju/core/user"
	usertesting "github.com/juju/juju/core/user/testing"
	userstate "github.com/juju/juju/domain/access/state"
	cloudstate "github.com/juju/juju/domain/cloud/state"
	"github.com/juju/juju/domain/credential"
	credentialstate "github.com/juju/juju/domain/credential/state"
	"github.com/juju/juju/domain/model"
	modelerrors "github.com/juju/juju/domain/model/errors"
	modelstate "github.com/juju/juju/domain/model/state"
	"github.com/juju/juju/domain/secretbackend/bootstrap"
	"github.com/juju/juju/internal/changestream/testing"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/secrets/provider/juju"
)

type stateSuite struct {
	testing.ControllerSuite
}

var _ = gc.Suite(&stateSuite{})

func (s *stateSuite) SetUpTest(c *gc.C) {
	s.ControllerSuite.SetUpTest(c)
}

var (
	testCloud = cloud.Cloud{
		Name:      "test",
		Type:      "ec2",
		AuthTypes: []cloud.AuthType{cloud.AccessKeyAuthType, cloud.UserPassAuthType},
		Endpoint:  "https://endpoint",
		Regions: []cloud.Region{{
			Name:             "test-region",
			Endpoint:         "http://region-endpoint1",
			IdentityEndpoint: "http://region-identity-endpoint1",
			StorageEndpoint:  "http://region-identity-endpoint1",
		}},
		CACertificates:    []string{"cert1"},
		SkipTLSVerify:     true,
		IsControllerCloud: false,
	}
)

func (s *stateSuite) setupModel(c *gc.C) coremodel.UUID {
	ctx := context.Background()

	err := bootstrap.CreateDefaultBackends(coremodel.IAAS)(ctx, s.ControllerTxnRunner(), s.TxnRunner())
	c.Assert(err, jc.ErrorIsNil)

	userName, err := user.NewName("test-usertest")
	c.Assert(err, jc.ErrorIsNil)
	userUUID := usertesting.GenUserUUID(c)
	err = userstate.NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c)).AddUser(ctx, userUUID, userName, userName.String(), false, userUUID)
	c.Assert(err, jc.ErrorIsNil)

	cloudUUID := cloudtesting.GenCloudUUID(c)
	err = cloudstate.NewState(s.TxnRunnerFactory()).CreateCloud(ctx, userName, cloudUUID.String(), testCloud)
	c.Assert(err, jc.ErrorIsNil)

	key := corecredential.Key{
		Cloud: "test",
		Owner: userName,
		Name:  "default",
	}
	authType := cloud.AccessKeyAuthType
	attributes := map[string]string{
		"foo": "foo val",
		"bar": "bar val",
	}

	credInfo := credential.CloudCredentialInfo{
		Label:      key.Name,
		AuthType:   string(authType),
		Attributes: attributes,
	}
	err = credentialstate.NewState(s.TxnRunnerFactory()).UpsertCloudCredential(ctx, key, credInfo)
	c.Assert(err, jc.ErrorIsNil)

	modelUUID := modeltesting.GenModelUUID(c)
	modelSt := modelstate.NewState(s.TxnRunnerFactory())
	err = modelSt.Create(ctx, modelUUID, coremodel.IAAS, model.GlobalModelCreationArgs{
		Cloud:         "test",
		CloudRegion:   "test-region",
		Credential:    key,
		Name:          "test",
		Owner:         userUUID,
		SecretBackend: juju.BackendName,
	})
	c.Assert(err, jc.ErrorIsNil)
	err = modelSt.Activate(ctx, modelUUID)
	c.Assert(err, jc.ErrorIsNil)
	return modelUUID
}

func (s *stateSuite) TestGetModelCloudAndCredential(c *gc.C) {
	uuid := s.setupModel(c)
	st := NewState(s.TxnRunnerFactory())
	cld, region, cred, err := st.GetModelCloudAndCredential(context.Background(), uuid)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(cld, gc.DeepEquals, &testCloud)
	c.Check(region, gc.Equals, "test-region")
	c.Check(cred.AuthType, gc.Equals, cloud.AccessKeyAuthType)
	c.Check(cred.Attributes, jc.DeepEquals, map[string]string{
		"foo": "foo val",
		"bar": "bar val",
	})
}

func (s *stateSuite) TestGetModelCloudAndCredentialNotFound(c *gc.C) {
	uuid := modeltesting.GenModelUUID(c)
	st := NewState(s.TxnRunnerFactory())
	_, _, _, err := st.GetModelCloudAndCredential(context.Background(), uuid)
	c.Assert(err, jc.ErrorIs, modelerrors.NotFound)
}
