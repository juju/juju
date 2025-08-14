// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"
	"regexp"
	stdtesting "testing"

	"github.com/juju/tc"

	"github.com/juju/juju/cloud"
	corecredential "github.com/juju/juju/core/credential"
	coreerrors "github.com/juju/juju/core/errors"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/core/user"
	userstate "github.com/juju/juju/domain/access/state"
	dbcloud "github.com/juju/juju/domain/cloud/state"
	"github.com/juju/juju/domain/credential"
	credentialerrors "github.com/juju/juju/domain/credential/errors"
	modelerrors "github.com/juju/juju/domain/model/errors"
	"github.com/juju/juju/domain/model/state/testing"
	changestreamtesting "github.com/juju/juju/internal/changestream/testing"
	"github.com/juju/juju/internal/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/uuid"
)

type credentialSuite struct {
	changestreamtesting.ControllerSuite
	userUUID       user.UUID
	userName       user.Name
	controllerUUID string
}

func TestCredentialSuite(t *stdtesting.T) {
	tc.Run(t, &credentialSuite{})
}

func (s *credentialSuite) SetUpTest(c *tc.C) {
	s.ControllerSuite.SetUpTest(c)

	s.controllerUUID = s.SeedControllerUUID(c)

	s.userName = user.GenName(c, "test-user")
	s.userUUID = s.addOwner(c, s.userName)

	s.addCloud(c, s.userName, cloud.Cloud{
		Name:      "stratus",
		Type:      "ec2",
		AuthTypes: cloud.AuthTypes{cloud.AccessKeyAuthType, cloud.UserPassAuthType},
	})
}

func (s *credentialSuite) TestUpdateCloudCredentialNew(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())

	credInfo := credential.CloudCredentialInfo{
		AuthType: string(cloud.AccessKeyAuthType),
		Attributes: map[string]string{
			"foo": "foo val",
			"bar": "bar val",
		},
		Revoked: true,
		Label:   "foobar",
	}
	key := corecredential.Key{Cloud: "stratus", Owner: s.userName, Name: "foobar"}
	ctx := c.Context()
	err := st.UpsertCloudCredential(ctx, key, credInfo)
	c.Assert(err, tc.ErrorIsNil)

	id, err := st.CredentialUUIDForKey(c.Context(), key)
	c.Check(err, tc.ErrorIsNil)
	c.Check(id != corecredential.UUID(""), tc.IsTrue)

	credResult := credential.CloudCredentialResult{
		CloudCredentialInfo: credInfo,
		CloudName:           "stratus",
	}
	out, err := st.CloudCredential(ctx, key)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(out, tc.DeepEquals, credResult)
}

func (s *credentialSuite) TestUpdateCloudCredentialNoValues(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())

	credInfo := credential.CloudCredentialInfo{
		AuthType:   string(cloud.AccessKeyAuthType),
		Attributes: map[string]string{},
		Label:      "foobar",
	}
	key := corecredential.Key{Cloud: "stratus", Owner: s.userName, Name: "foobar"}
	ctx := c.Context()
	err := st.UpsertCloudCredential(ctx, key, credInfo)
	c.Assert(err, tc.ErrorIsNil)

	credResult := credential.CloudCredentialResult{
		CloudCredentialInfo: credInfo,
		CloudName:           "stratus",
	}
	out, err := st.CloudCredential(ctx, key)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(out, tc.DeepEquals, credResult)
}

func (s *credentialSuite) TestUpdateCloudCredentialMissingName(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())

	credInfo := credential.CloudCredentialInfo{
		AuthType: string(cloud.AccessKeyAuthType),
		Attributes: map[string]string{
			"foo": "foo val",
			"bar": "bar val",
		},
		Label: "foobar",
	}
	ctx := c.Context()
	err := st.UpsertCloudCredential(ctx, corecredential.Key{Cloud: "stratus", Owner: s.userName}, credInfo)
	c.Assert(errors.Is(err, coreerrors.NotValid), tc.IsTrue)
}

func (s *credentialSuite) TestCreateInvalidCredential(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())

	credInfo := credential.CloudCredentialInfo{
		AuthType: string(cloud.AccessKeyAuthType),
		Attributes: map[string]string{
			"foo": "foo val",
			"bar": "bar val",
		},
		Label:         "foobar",
		Invalid:       true,
		InvalidReason: "because am testing you",
	}
	key := corecredential.Key{Cloud: "stratus", Owner: s.userName, Name: "foobar"}
	ctx := c.Context()
	err := st.UpsertCloudCredential(ctx, key, credInfo)
	c.Assert(err, tc.ErrorMatches, "adding invalid credential not supported")
}

func (s *credentialSuite) TestUpdateCloudCredentialExisting(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())

	credInfo := credential.CloudCredentialInfo{
		AuthType: string(cloud.AccessKeyAuthType),
		Attributes: map[string]string{
			"foo": "foo val",
			"bar": "bar val",
		},
		Label: "foobar",
	}
	key := corecredential.Key{Cloud: "stratus", Owner: s.userName, Name: "foobar"}
	ctx := c.Context()
	err := st.UpsertCloudCredential(ctx, key, credInfo)
	c.Assert(err, tc.ErrorIsNil)

	credInfo = credential.CloudCredentialInfo{
		AuthType: string(cloud.UserPassAuthType),
		Attributes: map[string]string{
			"user":     "bob's nephew",
			"password": "simple",
		},
		Label: "foobar",
	}
	err = st.UpsertCloudCredential(ctx, key, credInfo)
	c.Assert(err, tc.ErrorIsNil)

	credResult := credential.CloudCredentialResult{
		CloudCredentialInfo: credInfo,
		CloudName:           "stratus",
	}
	out, err := st.CloudCredential(ctx, key)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(out, tc.DeepEquals, credResult)
}

func (s *credentialSuite) TestUpdateCloudCredentialInvalidAuthType(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())

	credInfo := credential.CloudCredentialInfo{
		AuthType: string(cloud.OAuth2AuthType),
		Attributes: map[string]string{
			"foo": "foo val",
			"bar": "bar val",
		},
		Label: "foobar",
	}
	key := corecredential.Key{Cloud: "stratus", Owner: s.userName, Name: "foobar"}
	ctx := c.Context()
	err := st.UpsertCloudCredential(ctx, key, credInfo)
	c.Assert(err, tc.ErrorMatches, regexp.QuoteMeta(
		`updating credential: validating credential "foobar" owned by "test-user" for cloud "stratus": supported auth-types ["access-key" "userpass"], "oauth2" not supported`))
}

func (s *credentialSuite) TestCloudCredentialsEmpty(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())

	creds, err := st.CloudCredentialsForOwner(c.Context(), s.userName, "dummy")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(creds, tc.HasLen, 0)
}

func (s *credentialSuite) TestCloudCredentials(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())
	s.addOwner(c, user.GenName(c, "mary"))

	cred1Info := credential.CloudCredentialInfo{
		AuthType: string(cloud.AccessKeyAuthType),
		Attributes: map[string]string{
			"foo": "foo val",
			"bar": "bar val",
		},
	}
	ctx := c.Context()
	err := st.UpsertCloudCredential(ctx, corecredential.Key{Cloud: "stratus", Owner: s.userName, Name: "bobcred1"}, cred1Info)
	c.Assert(err, tc.ErrorIsNil)

	cred2Info := credential.CloudCredentialInfo{
		AuthType: string(cloud.AccessKeyAuthType),
		Attributes: map[string]string{
			"baz": "baz val",
			"qux": "qux val",
		},
	}
	err = st.UpsertCloudCredential(ctx, corecredential.Key{Cloud: "stratus", Owner: s.userName, Name: "bobcred2"}, cred2Info)
	c.Assert(err, tc.ErrorIsNil)
	err = st.UpsertCloudCredential(ctx, corecredential.Key{Cloud: "stratus", Owner: user.GenName(c, "mary"), Name: "foobar"}, cred2Info)
	c.Assert(err, tc.ErrorIsNil)

	cred1Info.Label = "bobcred1"
	cred1Result := credential.CloudCredentialResult{
		CloudCredentialInfo: cred1Info,
		CloudName:           "stratus",
	}
	cred2Info.Label = "bobcred2"
	cred2Result := credential.CloudCredentialResult{
		CloudCredentialInfo: cred2Info,
		CloudName:           "stratus",
	}

	creds, err := st.CloudCredentialsForOwner(ctx, s.userName, "stratus")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(creds, tc.DeepEquals, map[string]credential.CloudCredentialResult{
		"stratus/test-user/bobcred1": cred1Result,
		"stratus/test-user/bobcred2": cred2Result,
	})
}

func (s *credentialSuite) assertCredentialInvalidated(c *tc.C, st *State, key corecredential.Key) {
	credInfo := credential.CloudCredentialInfo{
		AuthType: string(cloud.AccessKeyAuthType),
		Attributes: map[string]string{
			"foo": "foo val",
			"bar": "bar val",
		},
	}
	ctx := c.Context()
	err := st.UpsertCloudCredential(ctx, key, credInfo)
	c.Assert(err, tc.ErrorIsNil)

	credInfo = credential.CloudCredentialInfo{
		AuthType: string(cloud.UserPassAuthType),
		Attributes: map[string]string{
			"user":     "bob's nephew",
			"password": "simple",
		},
	}
	credInfo.Invalid = true
	credInfo.InvalidReason = "because it is really really invalid"
	err = st.UpsertCloudCredential(ctx, key, credInfo)
	c.Assert(err, tc.ErrorIsNil)

	credInfo.Label = "foobar"
	credResult := credential.CloudCredentialResult{
		CloudCredentialInfo: credInfo,
		CloudName:           "stratus",
	}
	out, err := st.CloudCredential(ctx, key)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(out, tc.DeepEquals, credResult)
}

func (s *credentialSuite) TestInvalidateCredential(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())
	s.assertCredentialInvalidated(c, st, corecredential.Key{Cloud: "stratus", Owner: s.userName, Name: "foobar"})
}

func (s *credentialSuite) assertCredentialMarkedValid(c *tc.C, st *State, key corecredential.Key, credInfo credential.CloudCredentialInfo) {
	ctx := c.Context()
	err := st.UpsertCloudCredential(ctx, key, credInfo)
	c.Assert(err, tc.ErrorIsNil)

	out, err := st.CloudCredential(ctx, key)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(out.Invalid, tc.IsFalse)
}

func (s *credentialSuite) TestMarkInvalidCredentialAsValidExplicitly(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())
	// This call will ensure that there is an invalid credential to test with.
	key := corecredential.Key{Cloud: "stratus", Owner: s.userName, Name: "foobar"}
	s.assertCredentialInvalidated(c, st, key)

	credInfo := credential.CloudCredentialInfo{
		AuthType: string(cloud.UserPassAuthType),
		Attributes: map[string]string{
			"user":     "bob's nephew",
			"password": "simple",
		},
	}
	s.assertCredentialMarkedValid(c, st, key, credInfo)
}

func (s *credentialSuite) TestMarkInvalidCredentialAsValidImplicitly(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())
	key := corecredential.Key{Cloud: "stratus", Owner: s.userName, Name: "foobar"}
	// This call will ensure that there is an invalid credential to test with.
	s.assertCredentialInvalidated(c, st, key)

	credInfo := credential.CloudCredentialInfo{
		AuthType: string(cloud.UserPassAuthType),
		Attributes: map[string]string{
			"user":     "bob's nephew",
			"password": "simple",
		},
	}
	s.assertCredentialMarkedValid(c, st, key, credInfo)
}

func (s *credentialSuite) TestRemoveCredentials(c *tc.C) {
	modelUUID := testing.CreateTestModel(c, s.TxnRunnerFactory(), "foo")
	st := NewState(s.TxnRunnerFactory())

	cred1Info := credential.CloudCredentialInfo{
		AuthType: string(cloud.AccessKeyAuthType),
		Attributes: map[string]string{
			"foo": "foo val",
			"bar": "bar val",
		},
	}
	key := corecredential.Key{
		Cloud: "foo",
		Owner: user.GenName(c, "test-userfoo"),
		Name:  "foobar",
	}
	ctx := c.Context()
	err := st.UpsertCloudCredential(ctx, key, cred1Info)
	c.Assert(err, tc.ErrorIsNil)

	models, err := st.ModelsUsingCloudCredential(ctx, key)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(models, tc.DeepEquals, map[coremodel.UUID]string{
		modelUUID: "foo",
	})

	err = st.RemoveCloudCredential(ctx, key)
	c.Assert(err, tc.ErrorIsNil)

	_, err = st.CloudCredential(ctx, key)
	c.Assert(err, tc.ErrorIs, credentialerrors.NotFound)

	_, err = st.ModelsUsingCloudCredential(ctx, key)
	c.Assert(err, tc.ErrorIs, credentialerrors.NotFound)
}

func (s *credentialSuite) TestAllCloudCredentialsNotFound(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())

	out, err := st.AllCloudCredentialsForOwner(c.Context(), s.userName)
	c.Assert(err, tc.ErrorMatches, "cloud credentials for \"test-user\" not found")
	c.Assert(out, tc.IsNil)
}

func (s *credentialSuite) TestAllCloudCredentials(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())

	keyOne := corecredential.Key{Cloud: "cirrus", Owner: s.userName, Name: "foobar"}
	s.addCloud(c, keyOne.Owner, cloud.Cloud{
		Name:      keyOne.Cloud,
		Type:      "ec2",
		AuthTypes: cloud.AuthTypes{cloud.AccessKeyAuthType, cloud.UserPassAuthType},
	})
	keyTwo := corecredential.Key{Cloud: "stratus", Owner: s.userName, Name: "foobar"}
	one := s.createCloudCredential(c, st, keyOne)
	two := s.createCloudCredential(c, st, keyTwo)

	// We need to add mary here so that they are a valid user.
	s.addOwner(c, user.GenName(c, "mary"))

	// Added to make sure it is not returned.
	keyThree := corecredential.Key{Cloud: "cumulus", Owner: user.GenName(c, "mary"), Name: "foobar"}
	s.addCloud(c, keyThree.Owner, cloud.Cloud{
		Name:      keyThree.Cloud,
		Type:      "ec2",
		AuthTypes: cloud.AuthTypes{cloud.AccessKeyAuthType, cloud.UserPassAuthType},
	})
	s.createCloudCredential(c, st, keyThree)

	resultOne := credential.CloudCredentialResult{
		CloudCredentialInfo: one,
		CloudName:           "cirrus",
	}
	resultTwo := credential.CloudCredentialResult{
		CloudCredentialInfo: two,
		CloudName:           "stratus",
	}
	out, err := st.AllCloudCredentialsForOwner(c.Context(), s.userName)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(out, tc.DeepEquals, map[corecredential.Key]credential.CloudCredentialResult{
		keyOne: resultOne, keyTwo: resultTwo})
}

func (s *credentialSuite) TestInvalidateCloudCredential(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())

	key := corecredential.Key{Cloud: "stratus", Owner: s.userName, Name: "foobar"}
	one := s.createCloudCredential(c, st, key)
	c.Assert(one.Invalid, tc.IsFalse)
	uuid, err := st.CredentialUUIDForKey(c.Context(), key)
	c.Assert(err, tc.ErrorIsNil)

	ctx := c.Context()
	reason := "testing, testing 1,2,3"
	err = st.InvalidateCloudCredential(ctx, uuid, reason)
	c.Assert(err, tc.ErrorIsNil)

	updated, err := st.CloudCredential(ctx, key)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(updated.Invalid, tc.IsTrue)
	c.Assert(updated.InvalidReason, tc.Equals, reason)
}

func (s *credentialSuite) TestInvalidateCloudCredentialNotFound(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())

	badUUID := corecredential.UUID("not valid")
	ctx := c.Context()
	err := st.InvalidateCloudCredential(ctx, badUUID, "reason")
	c.Assert(err, tc.ErrorIs, credentialerrors.NotFound)
}

func (s *credentialSuite) TestNoModelsUsingCloudCredential(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())

	key := corecredential.Key{Cloud: "stratus", Owner: s.userName, Name: "foobar"}
	s.createCloudCredential(c, st, key)

	result, err := st.ModelsUsingCloudCredential(c.Context(), key)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.HasLen, 0)
}

func (s *credentialSuite) TestModelsUsingCloudCredential(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())

	key := corecredential.Key{Cloud: "stratus", Owner: s.userName, Name: "foobar"}
	one := s.createCloudCredential(c, st, key)
	c.Assert(one.Invalid, tc.IsFalse)

	insertOne := func(ctx context.Context, tx *sql.Tx, modelUUID, name string) error {
		result, err := tx.ExecContext(ctx, `
INSERT INTO model (uuid, name, qualifier, life_id, model_type_id, activated, cloud_uuid, cloud_credential_uuid)
SELECT ?, ?, ?, 0, 0, true,
	(SELECT uuid FROM cloud WHERE cloud.name="stratus"),
	(SELECT uuid FROM cloud_credential cc WHERE cc.name="foobar")
			`,
			modelUUID, name, "prod",
		)
		if err != nil {
			return err
		}
		numRows, err := result.RowsAffected()
		if err != nil {
			return err
		}
		c.Assert(numRows, tc.Equals, int64(1))

		return nil
	}

	modelUUID := uuid.MustNewUUID().String()
	modelUUID2 := uuid.MustNewUUID().String()
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		if err := insertOne(ctx, tx, modelUUID, "mymodel"); err != nil {
			return err
		}
		if err := insertOne(ctx, tx, modelUUID2, "mymodel2"); err != nil {
			return err
		}
		return nil
	})
	c.Assert(err, tc.ErrorIsNil)

	result, err := st.ModelsUsingCloudCredential(c.Context(), corecredential.Key{
		Cloud: "stratus",
		Owner: s.userName,
		Name:  "foobar",
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, map[coremodel.UUID]string{
		coremodel.UUID(modelUUID):  "mymodel",
		coremodel.UUID(modelUUID2): "mymodel2",
	})
}

// TestGetCloudCredential is testing the happy path for GetCloudCredential.
func (s *credentialSuite) TestGetCloudCredential(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())

	s.addCloud(c, s.userName, cloud.Cloud{
		Name:      "cirrus",
		Type:      "ec2",
		AuthTypes: cloud.AuthTypes{cloud.AccessKeyAuthType, cloud.UserPassAuthType},
	})

	keyOne := corecredential.Key{Cloud: "cirrus", Owner: s.userName, Name: "foobar"}
	one := s.createCloudCredential(c, st, keyOne)

	id, err := st.CredentialUUIDForKey(c.Context(), keyOne)
	c.Assert(err, tc.ErrorIsNil)

	res, err := st.GetCloudCredential(c.Context(), id)
	c.Check(err, tc.ErrorIsNil)
	c.Check(res.CloudCredentialInfo, tc.DeepEquals, one)
	c.Check(res.CloudName, tc.Equals, "cirrus")
}

func (s *credentialSuite) TestGetCloudCredentialNonExistent(c *tc.C) {
	id, err := corecredential.NewUUID()
	c.Assert(err, tc.ErrorIsNil)

	st := NewState(s.TxnRunnerFactory())
	_, err = st.GetCloudCredential(c.Context(), id)
	c.Check(err, tc.ErrorIs, credentialerrors.NotFound)
}

func (s *credentialSuite) addOwner(c *tc.C, name user.Name) user.UUID {
	userUUID, err := user.NewUUID()
	c.Assert(err, tc.ErrorIsNil)
	userState := userstate.NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))
	err = userState.AddUserWithPermission(
		c.Context(),
		userUUID,
		name,
		"test user",
		false,
		userUUID,
		permission.AccessSpec{
			Access: permission.SuperuserAccess,
			Target: permission.ID{
				ObjectType: permission.Controller,
				Key:        s.controllerUUID,
			},
		},
	)
	c.Assert(err, tc.ErrorIsNil)
	return userUUID
}

func (s *credentialSuite) addCloud(c *tc.C, userName user.Name, cloud cloud.Cloud) string {
	cloudSt := dbcloud.NewState(s.TxnRunnerFactory())
	ctx := c.Context()
	cloudUUID := uuid.MustNewUUID().String()
	err := cloudSt.CreateCloud(ctx, userName, cloudUUID, cloud)
	c.Assert(err, tc.ErrorIsNil)

	return cloudUUID
}

func (s *credentialSuite) createCloudCredential(c *tc.C, st *State, key corecredential.Key) credential.CloudCredentialInfo {
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
	err := st.UpsertCloudCredential(c.Context(), key, credInfo)
	c.Assert(err, tc.ErrorIsNil)
	return credInfo
}

func (s *credentialSuite) TestInvalidateModelCloudCredential(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())

	key := corecredential.Key{Cloud: "stratus", Owner: s.userName, Name: "foobar"}
	s.createCloudCredential(c, st, key)

	insertOne := func(ctx context.Context, tx *sql.Tx, modelUUID coremodel.UUID, name string) error {
		result, err := tx.ExecContext(ctx, `
INSERT INTO model (uuid, name, qualifier, life_id, model_type_id, activated, cloud_uuid, cloud_credential_uuid)
SELECT ?, ?, ?, 0, 0, true,
	(SELECT uuid FROM cloud WHERE cloud.name="stratus"),
	(SELECT uuid FROM cloud_credential cc WHERE cc.name="foobar")
			`,
			modelUUID, name, "prod",
		)
		if err != nil {
			return err
		}
		numRows, err := result.RowsAffected()
		if err != nil {
			return err
		}
		c.Assert(numRows, tc.Equals, int64(1))

		return nil
	}

	modelUUID := coremodel.GenUUID(c)
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		if err := insertOne(ctx, tx, modelUUID, "mymodel"); err != nil {
			return err
		}
		return nil
	})
	c.Assert(err, tc.ErrorIsNil)

	err = st.InvalidateModelCloudCredential(c.Context(), modelUUID, "test reason")
	c.Check(err, tc.ErrorIsNil)

	updated, err := st.CloudCredential(c.Context(), key)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(updated.Invalid, tc.IsTrue)
	c.Assert(updated.InvalidReason, tc.Equals, "test reason")
}

// TestInvalidateModelCloudCredentialNotFound is testing the case where we try
// to cancel the cloud credential for a model that does not exist. In this case
// we should get back an error satisfying [modelerrors.NotFound].
func (s *credentialSuite) TestInvalidateModelCloudCredentialNotFound(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())

	modelUUID := coremodel.GenUUID(c)
	err := st.InvalidateModelCloudCredential(c.Context(), modelUUID, "test reason")
	c.Check(err, tc.ErrorIs, modelerrors.NotFound)
}

// TestInvalidateModelCloudCredentialNotSet is testing the case where we try to
// invalidate the cloud credential of a model but the model does not have a
// cloud credential set. In this case we should get back an error satisfying
// [credentialerrors.ModelCredentialNotSet].
func (s *credentialSuite) TestInvalidateModelCloudCredentialNotSet(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())

	key := corecredential.Key{Cloud: "stratus", Owner: s.userName, Name: "foobar"}
	s.createCloudCredential(c, st, key)

	insertOne := func(ctx context.Context, tx *sql.Tx, modelUUID coremodel.UUID, name string) error {
		result, err := tx.ExecContext(ctx, `
INSERT INTO model (uuid, name, qualifier, life_id, model_type_id, activated, cloud_uuid, cloud_credential_uuid)
SELECT ?, ?, ?, 0, 0, true,
	(SELECT uuid FROM cloud WHERE cloud.name="stratus"),
	NULL
`,
			modelUUID, name, "prod",
		)
		if err != nil {
			return err
		}
		numRows, err := result.RowsAffected()
		if err != nil {
			return err
		}
		c.Assert(numRows, tc.Equals, int64(1))

		return nil
	}

	modelUUID := coremodel.GenUUID(c)
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		if err := insertOne(ctx, tx, modelUUID, "mymodel"); err != nil {
			return err
		}
		return nil
	})
	c.Assert(err, tc.ErrorIsNil)

	err = st.InvalidateModelCloudCredential(c.Context(), modelUUID, "test reason")
	c.Check(err, tc.ErrorIs, credentialerrors.ModelCredentialNotSet)
}

// Testis testing that if we ask for the
// credential and status of a model and the model does not exist we get back an
// error satisfying [modelerrors.NotFound].
func (s *credentialSuite) TestGetmodelCredentialStatusNotFound(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())
	modelUUID := coremodel.GenUUID(c)
	_, _, err := st.GetModelCredentialStatus(c.Context(), modelUUID)
	c.Check(err, tc.ErrorIs, modelerrors.NotFound)
}

// TestGetModelCredentialStatusNotSet is testing that if the credential and
// status is asked for and the model does not have a credential set we get back
// an error satisfying [credentialerrors.ModelCredentialNotSet].
func (s *credentialSuite) TestGetModelCredentialStatusNotSet(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())

	key := corecredential.Key{Cloud: "stratus", Owner: s.userName, Name: "foobar"}
	s.createCloudCredential(c, st, key)

	insertOne := func(ctx context.Context, tx *sql.Tx, modelUUID coremodel.UUID, name string) error {
		result, err := tx.ExecContext(ctx, `
INSERT INTO model (uuid, name, qualifier, life_id, model_type_id, activated, cloud_uuid, cloud_credential_uuid)
SELECT ?, ?, ?, 0, 0, true,
	(SELECT uuid FROM cloud WHERE cloud.name="stratus"),
	NULL`,
			modelUUID, name, "prod",
		)
		if err != nil {
			return err
		}
		numRows, err := result.RowsAffected()
		if err != nil {
			return err
		}
		c.Assert(numRows, tc.Equals, int64(1))

		return nil
	}

	modelUUID := coremodel.GenUUID(c)
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		if err := insertOne(ctx, tx, modelUUID, "mymodel"); err != nil {
			return err
		}
		return nil
	})
	c.Assert(err, tc.ErrorIsNil)

	_, _, err = st.GetModelCredentialStatus(c.Context(), modelUUID)
	c.Check(err, tc.ErrorIs, credentialerrors.ModelCredentialNotSet)
}

// TestGetModelCredentialValid is testing the happy path for getting the
// credential and validity status for a model when the credential is considered
// valid.
func (s *credentialSuite) TestGetModelCredentialValid(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())

	key := corecredential.Key{Cloud: "stratus", Owner: s.userName, Name: "foobar"}
	s.createCloudCredential(c, st, key)

	insertOne := func(ctx context.Context, tx *sql.Tx, modelUUID coremodel.UUID, name string) error {
		result, err := tx.ExecContext(ctx, `
INSERT INTO model (uuid, name, qualifier, life_id, model_type_id, activated, cloud_uuid, cloud_credential_uuid)
SELECT ?, ?, ?, 0, 0, true,
	(SELECT uuid FROM cloud WHERE cloud.name="stratus"),
	(SELECT uuid FROM cloud_credential cc WHERE cc.name="foobar")
			`,
			modelUUID, name, "prod",
		)
		if err != nil {
			return err
		}
		numRows, err := result.RowsAffected()
		if err != nil {
			return err
		}
		c.Assert(numRows, tc.Equals, int64(1))

		return nil
	}

	modelUUID := coremodel.GenUUID(c)
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		if err := insertOne(ctx, tx, modelUUID, "mymodel"); err != nil {
			return err
		}
		return nil
	})
	c.Assert(err, tc.ErrorIsNil)

	credKey, valid, err := st.GetModelCredentialStatus(c.Context(), modelUUID)
	c.Check(err, tc.ErrorIsNil)
	c.Check(valid, tc.IsTrue)
	c.Check(credKey, tc.Equals, key)
}

// TestGetModelCredentialInvalid is testing the happy path for getting the
// credential and validity status for a model when the credential is is
// considered invalid.
func (s *credentialSuite) TestGetModelCredentialInvalid(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())

	key := corecredential.Key{Cloud: "stratus", Owner: s.userName, Name: "foobar"}
	s.createCloudCredential(c, st, key)
	credUUID, err := st.CredentialUUIDForKey(c.Context(), key)
	c.Assert(err, tc.ErrorIsNil)
	err = st.InvalidateCloudCredential(c.Context(), credUUID, "test reason")
	c.Assert(err, tc.ErrorIsNil)

	insertOne := func(ctx context.Context, tx *sql.Tx, modelUUID coremodel.UUID, name string) error {
		result, err := tx.ExecContext(ctx, `
INSERT INTO model (uuid, name, qualifier, life_id, model_type_id, activated, cloud_uuid, cloud_credential_uuid)
SELECT ?, ?, ?, 0, 0, true,
	(SELECT uuid FROM cloud WHERE cloud.name="stratus"),
	(SELECT uuid FROM cloud_credential cc WHERE cc.name="foobar")
			`,
			modelUUID, name, "prod",
		)
		if err != nil {
			return err
		}
		numRows, err := result.RowsAffected()
		if err != nil {
			return err
		}
		c.Assert(numRows, tc.Equals, int64(1))

		return nil
	}

	modelUUID := coremodel.GenUUID(c)
	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		if err := insertOne(ctx, tx, modelUUID, "mymodel"); err != nil {
			return err
		}
		return nil
	})
	c.Assert(err, tc.ErrorIsNil)

	credKey, valid, err := st.GetModelCredentialStatus(c.Context(), modelUUID)
	c.Check(err, tc.ErrorIsNil)
	c.Check(valid, tc.IsFalse)
	c.Check(credKey, tc.Equals, key)
}
