// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	stdtesting "testing"

	"github.com/juju/tc"

	"github.com/juju/juju/cloud"
	corecloud "github.com/juju/juju/core/cloud"
	corecredential "github.com/juju/juju/core/credential"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/user"
	userstate "github.com/juju/juju/domain/access/state"
	cloudstate "github.com/juju/juju/domain/cloud/state"
	"github.com/juju/juju/domain/credential"
	credentialstate "github.com/juju/juju/domain/credential/state"
	"github.com/juju/juju/domain/model"
	modelerrors "github.com/juju/juju/domain/model/errors"
	statecontroller "github.com/juju/juju/domain/model/state/controller"
	"github.com/juju/juju/domain/secretbackend/bootstrap"
	"github.com/juju/juju/internal/changestream/testing"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/secrets/provider/juju"
)

type stateSuite struct {
	testing.ControllerSuite
}

func TestStateSuite(t *stdtesting.T) {
	tc.Run(t, &stateSuite{})
}

func (s *stateSuite) SetUpTest(c *tc.C) {
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

func (s *stateSuite) setupModel(c *tc.C) coremodel.UUID {
	ctx := c.Context()

	err := bootstrap.CreateDefaultBackends(coremodel.IAAS)(ctx, s.ControllerTxnRunner(), s.TxnRunner())
	c.Assert(err, tc.ErrorIsNil)

	userName, err := user.NewName("test-usertest")
	c.Assert(err, tc.ErrorIsNil)
	userUUID := user.GenUUID(c)
	err = userstate.NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c)).AddUser(ctx, userUUID, userName, userName.String(), false, userUUID)
	c.Assert(err, tc.ErrorIsNil)

	cloudUUID := corecloud.GenUUID(c)
	err = cloudstate.NewState(s.TxnRunnerFactory()).CreateCloud(ctx, userName, cloudUUID.String(), testCloud)
	c.Assert(err, tc.ErrorIsNil)

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
	c.Assert(err, tc.ErrorIsNil)

	modelUUID := coremodel.GenUUID(c)
	modelSt := statecontroller.NewState(s.TxnRunnerFactory())
	err = modelSt.Create(ctx, modelUUID, coremodel.IAAS, model.GlobalModelCreationArgs{
		Cloud:         "test",
		CloudRegion:   "test-region",
		Credential:    key,
		Name:          "test",
		Qualifier:     "prod",
		AdminUsers:    []user.UUID{userUUID},
		SecretBackend: juju.BackendName,
	})
	c.Assert(err, tc.ErrorIsNil)
	err = modelSt.Activate(ctx, modelUUID)
	c.Assert(err, tc.ErrorIsNil)
	return modelUUID
}

func (s *stateSuite) TestGetModelCloudAndCredential(c *tc.C) {
	uuid := s.setupModel(c)
	st := NewState(s.TxnRunnerFactory())
	cld, region, cred, err := st.GetModelCloudAndCredential(c.Context(), uuid)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(cld, tc.DeepEquals, &testCloud)
	c.Check(region, tc.Equals, "test-region")
	c.Check(cred.AuthType, tc.Equals, cloud.AccessKeyAuthType)
	c.Check(cred.Attributes, tc.DeepEquals, map[string]string{
		"foo": "foo val",
		"bar": "bar val",
	})
}

func (s *stateSuite) TestGetModelCloudAndCredentialNotFound(c *tc.C) {
	uuid := coremodel.GenUUID(c)
	st := NewState(s.TxnRunnerFactory())
	_, _, _, err := st.GetModelCloudAndCredential(c.Context(), uuid)
	c.Assert(err, tc.ErrorIs, modelerrors.NotFound)
}
