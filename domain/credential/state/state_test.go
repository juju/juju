// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"
	"fmt"
	"regexp"
	"time"

	"github.com/canonical/sqlair"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v4/workertest"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/core/changestream"
	"github.com/juju/juju/core/user"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/eventsource"
	"github.com/juju/juju/core/watcher/watchertest"
	dbcloud "github.com/juju/juju/domain/cloud/state"
	"github.com/juju/juju/domain/credential"
	"github.com/juju/juju/domain/model"
	userstate "github.com/juju/juju/domain/user/state"
	changestreamtesting "github.com/juju/juju/internal/changestream/testing"
	"github.com/juju/juju/internal/uuid"
	jujutesting "github.com/juju/juju/testing"
)

type credentialSuite struct {
	changestreamtesting.ControllerSuite
	userUUID user.UUID
}

var _ = gc.Suite(&credentialSuite{})

func (s *credentialSuite) SetUpTest(c *gc.C) {
	s.ControllerSuite.SetUpTest(c)

	userUUID, err := user.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	s.userUUID = userUUID
	userState := userstate.NewState(s.TxnRunnerFactory())
	err = userState.AddUser(
		context.Background(),
		s.userUUID,
		"test-user",
		"test user",
		s.userUUID,
	)
	c.Assert(err, jc.ErrorIsNil)

	s.addCloud(c, cloud.Cloud{
		Name:      "stratus",
		Type:      "ec2",
		AuthTypes: cloud.AuthTypes{cloud.AccessKeyAuthType, cloud.UserPassAuthType},
	})
}

func (s *credentialSuite) addCloud(c *gc.C, cloud cloud.Cloud) string {
	cloudSt := dbcloud.NewState(s.TxnRunnerFactory())
	ctx := context.Background()
	err := cloudSt.UpsertCloud(ctx, cloud)
	c.Assert(err, jc.ErrorIsNil)

	db := s.DB()
	row := db.QueryRow("SELECT uuid FROM cloud WHERE name = ?", cloud.Name)
	c.Assert(row.Err(), jc.ErrorIsNil)

	var dbCloud dbcloud.Cloud
	err = row.Scan(&dbCloud.ID)
	c.Assert(err, jc.ErrorIsNil)
	return dbCloud.ID
}

func (s *credentialSuite) TestUpdateCloudCredentialNew(c *gc.C) {
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
	id := credential.ID{Cloud: "stratus", Owner: "bob", Name: "foobar"}
	ctx := context.Background()
	existingInvalid, err := st.UpsertCloudCredential(ctx, id, credInfo)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(existingInvalid, gc.IsNil)

	credResult := credential.CloudCredentialResult{
		CloudCredentialInfo: credInfo,
		CloudName:           "stratus",
	}
	out, err := st.CloudCredential(ctx, id)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(out, jc.DeepEquals, credResult)
}

func (s *credentialSuite) TestUpdateCloudCredentialNoValues(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())

	credInfo := credential.CloudCredentialInfo{
		AuthType:   string(cloud.AccessKeyAuthType),
		Attributes: map[string]string{},
		Label:      "foobar",
	}
	id := credential.ID{Cloud: "stratus", Owner: "bob", Name: "foobar"}
	ctx := context.Background()
	existingInvalid, err := st.UpsertCloudCredential(ctx, id, credInfo)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(existingInvalid, gc.IsNil)

	credResult := credential.CloudCredentialResult{
		CloudCredentialInfo: credInfo,
		CloudName:           "stratus",
	}
	out, err := st.CloudCredential(ctx, id)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(out, jc.DeepEquals, credResult)
}

func (s *credentialSuite) TestUpdateCloudCredentialMissingName(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())

	credInfo := credential.CloudCredentialInfo{
		AuthType: string(cloud.AccessKeyAuthType),
		Attributes: map[string]string{
			"foo": "foo val",
			"bar": "bar val",
		},
		Label: "foobar",
	}
	ctx := context.Background()
	_, err := st.UpsertCloudCredential(ctx, credential.ID{Cloud: "stratus", Owner: "bob"}, credInfo)
	c.Assert(errors.Is(err, errors.NotValid), jc.IsTrue)
}

func (s *credentialSuite) TestCreateInvalidCredential(c *gc.C) {
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
	id := credential.ID{Cloud: "stratus", Owner: "bob", Name: "foobar"}
	ctx := context.Background()
	_, err := st.UpsertCloudCredential(ctx, id, credInfo)
	c.Assert(err, gc.ErrorMatches, "adding invalid credential not supported")
}

func (s *credentialSuite) TestUpdateCloudCredentialExisting(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())

	credInfo := credential.CloudCredentialInfo{
		AuthType: string(cloud.AccessKeyAuthType),
		Attributes: map[string]string{
			"foo": "foo val",
			"bar": "bar val",
		},
		Label: "foobar",
	}
	id := credential.ID{Cloud: "stratus", Owner: "bob", Name: "foobar"}
	ctx := context.Background()
	existingInvalid, err := st.UpsertCloudCredential(ctx, id, credInfo)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(existingInvalid, gc.IsNil)

	credInfo = credential.CloudCredentialInfo{
		AuthType: string(cloud.UserPassAuthType),
		Attributes: map[string]string{
			"user":     "bob's nephew",
			"password": "simple",
		},
		Label: "foobar",
	}
	existingInvalid, err = st.UpsertCloudCredential(ctx, id, credInfo)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(existingInvalid, gc.NotNil)
	c.Assert(*existingInvalid, jc.IsFalse)

	credResult := credential.CloudCredentialResult{
		CloudCredentialInfo: credInfo,
		CloudName:           "stratus",
	}
	out, err := st.CloudCredential(ctx, id)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(out, jc.DeepEquals, credResult)
}

func (s *credentialSuite) TestUpdateCloudCredentialInvalidAuthType(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())

	credInfo := credential.CloudCredentialInfo{
		AuthType: string(cloud.OAuth2AuthType),
		Attributes: map[string]string{
			"foo": "foo val",
			"bar": "bar val",
		},
		Label: "foobar",
	}
	id := credential.ID{Cloud: "stratus", Owner: "bob", Name: "foobar"}
	ctx := context.Background()
	_, err := st.UpsertCloudCredential(ctx, id, credInfo)
	c.Assert(err, gc.ErrorMatches, regexp.QuoteMeta(
		`updating credential: validating credential "foobar" owned by "bob" for cloud "stratus": supported auth-types ["access-key" "userpass"], "oauth2" not supported`))
}

func (s *credentialSuite) TestCloudCredentialsEmpty(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())

	creds, err := st.CloudCredentialsForOwner(context.Background(), "bob", "dummy")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(creds, gc.HasLen, 0)
}

func (s *credentialSuite) TestCloudCredentials(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())

	cred1Info := credential.CloudCredentialInfo{
		AuthType: string(cloud.AccessKeyAuthType),
		Attributes: map[string]string{
			"foo": "foo val",
			"bar": "bar val",
		},
	}
	ctx := context.Background()
	_, err := st.UpsertCloudCredential(ctx, credential.ID{Cloud: "stratus", Owner: "bob", Name: "bobcred1"}, cred1Info)
	c.Assert(err, jc.ErrorIsNil)

	cred2Info := credential.CloudCredentialInfo{
		AuthType: string(cloud.AccessKeyAuthType),
		Attributes: map[string]string{
			"baz": "baz val",
			"qux": "qux val",
		},
	}
	_, err = st.UpsertCloudCredential(ctx, credential.ID{Cloud: "stratus", Owner: "bob", Name: "bobcred2"}, cred2Info)
	c.Assert(err, jc.ErrorIsNil)
	_, err = st.UpsertCloudCredential(ctx, credential.ID{Cloud: "stratus", Owner: "mary", Name: "foobar"}, cred2Info)
	c.Assert(err, jc.ErrorIsNil)

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

	for _, userName := range []string{"bob", "bob"} {
		creds, err := st.CloudCredentialsForOwner(ctx, userName, "stratus")
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(creds, jc.DeepEquals, map[string]credential.CloudCredentialResult{
			"stratus/bob/bobcred1": cred1Result,
			"stratus/bob/bobcred2": cred2Result,
		})
	}
}

func (s *credentialSuite) assertCredentialInvalidated(c *gc.C, st *State, id credential.ID) {
	credInfo := credential.CloudCredentialInfo{
		AuthType: string(cloud.AccessKeyAuthType),
		Attributes: map[string]string{
			"foo": "foo val",
			"bar": "bar val",
		},
	}
	ctx := context.Background()
	existingInvalid, err := st.UpsertCloudCredential(ctx, id, credInfo)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(existingInvalid, gc.IsNil)

	credInfo = credential.CloudCredentialInfo{
		AuthType: string(cloud.UserPassAuthType),
		Attributes: map[string]string{
			"user":     "bob's nephew",
			"password": "simple",
		},
	}
	credInfo.Invalid = true
	credInfo.InvalidReason = "because it is really really invalid"
	existingInvalid, err = st.UpsertCloudCredential(ctx, id, credInfo)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(existingInvalid, gc.NotNil)
	c.Assert(*existingInvalid, jc.IsFalse)

	credInfo.Label = "foobar"
	credResult := credential.CloudCredentialResult{
		CloudCredentialInfo: credInfo,
		CloudName:           "stratus",
	}
	out, err := st.CloudCredential(ctx, id)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(out, jc.DeepEquals, credResult)
}

func (s *credentialSuite) TestInvalidateCredential(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())
	s.assertCredentialInvalidated(c, st, credential.ID{Cloud: "stratus", Owner: "bob", Name: "foobar"})
}

func (s *credentialSuite) assertCredentialMarkedValid(c *gc.C, st *State, id credential.ID, credInfo credential.CloudCredentialInfo) {
	ctx := context.Background()
	existingInvalid, err := st.UpsertCloudCredential(ctx, id, credInfo)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(existingInvalid, gc.NotNil)
	c.Assert(*existingInvalid, jc.IsTrue)

	out, err := st.CloudCredential(ctx, id)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(out.Invalid, jc.IsFalse)
}

func (s *credentialSuite) TestMarkInvalidCredentialAsValidExplicitly(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())
	// This call will ensure that there is an invalid credential to test with.
	id := credential.ID{Cloud: "stratus", Owner: "bob", Name: "foobar"}
	s.assertCredentialInvalidated(c, st, id)

	credInfo := credential.CloudCredentialInfo{
		AuthType: string(cloud.UserPassAuthType),
		Attributes: map[string]string{
			"user":     "bob's nephew",
			"password": "simple",
		},
	}
	s.assertCredentialMarkedValid(c, st, id, credInfo)
}

func (s *credentialSuite) TestMarkInvalidCredentialAsValidImplicitly(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())
	id := credential.ID{Cloud: "stratus", Owner: "bob", Name: "foobar"}
	// This call will ensure that there is an invalid credential to test with.
	s.assertCredentialInvalidated(c, st, id)

	credInfo := credential.CloudCredentialInfo{
		AuthType: string(cloud.UserPassAuthType),
		Attributes: map[string]string{
			"user":     "bob's nephew",
			"password": "simple",
		},
	}
	s.assertCredentialMarkedValid(c, st, id, credInfo)
}

func (s *credentialSuite) TestRemoveCredentials(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())

	cred1Info := credential.CloudCredentialInfo{
		AuthType: string(cloud.AccessKeyAuthType),
		Attributes: map[string]string{
			"foo": "foo val",
			"bar": "bar val",
		},
	}
	id := credential.ID{Cloud: "stratus", Owner: "bob", Name: "bobcred1"}
	ctx := context.Background()
	_, err := st.UpsertCloudCredential(ctx, id, cred1Info)
	c.Assert(err, jc.ErrorIsNil)

	err = st.RemoveCloudCredential(ctx, id)
	c.Assert(err, jc.ErrorIsNil)

	_, err = st.CloudCredential(ctx, id)
	c.Assert(err, jc.ErrorIs, errors.NotFound)
}

func (s *credentialSuite) TestAllCloudCredentialsNotFound(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())

	out, err := st.AllCloudCredentialsForOwner(context.Background(), "bob")
	c.Assert(err, gc.ErrorMatches, "cloud credentials for \"bob\" not found")
	c.Assert(out, gc.IsNil)
}

func (s *credentialSuite) createCloudCredential(c *gc.C, st *State, id credential.ID) credential.CloudCredentialInfo {
	authType := cloud.AccessKeyAuthType
	attributes := map[string]string{
		"foo": "foo val",
		"bar": "bar val",
	}

	s.addCloud(c, cloud.Cloud{
		Name:      id.Cloud,
		Type:      "ec2",
		AuthTypes: cloud.AuthTypes{cloud.AccessKeyAuthType, cloud.UserPassAuthType},
	})

	credInfo := credential.CloudCredentialInfo{
		Label:      id.Name,
		AuthType:   string(authType),
		Attributes: attributes,
	}
	_, err := st.UpsertCloudCredential(context.Background(), id, credInfo)
	c.Assert(err, jc.ErrorIsNil)
	return credInfo
}

func (s *credentialSuite) TestAllCloudCredentials(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())

	idOne := credential.ID{Cloud: "cirrus", Owner: "bob", Name: "foobar"}
	idTwo := credential.ID{Cloud: "stratus", Owner: "bob", Name: "foobar"}
	one := s.createCloudCredential(c, st, idOne)
	two := s.createCloudCredential(c, st, idTwo)

	// Added to make sure it is not returned.
	s.createCloudCredential(c, st, credential.ID{Cloud: "cumulus", Owner: "mary", Name: "foobar"})

	resultOne := credential.CloudCredentialResult{
		CloudCredentialInfo: one,
		CloudName:           "cirrus",
	}
	resultTwo := credential.CloudCredentialResult{
		CloudCredentialInfo: two,
		CloudName:           "stratus",
	}
	out, err := st.AllCloudCredentialsForOwner(context.Background(), "bob")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(out, jc.DeepEquals, map[credential.ID]credential.CloudCredentialResult{
		idOne: resultOne, idTwo: resultTwo})
}

func (s *credentialSuite) TestInvalidateCloudCredential(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())

	id := credential.ID{Cloud: "stratus", Owner: "bob", Name: "foobar"}
	one := s.createCloudCredential(c, st, id)
	c.Assert(one.Invalid, jc.IsFalse)

	ctx := context.Background()
	reason := "testing, testing 1,2,3"
	err := st.InvalidateCloudCredential(ctx, id, reason)
	c.Assert(err, jc.ErrorIsNil)

	updated, err := st.CloudCredential(ctx, id)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(updated.Invalid, jc.IsTrue)
	c.Assert(updated.InvalidReason, gc.Equals, reason)
}

func (s *credentialSuite) TestInvalidateCloudCredentialNotFound(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())

	id := credential.ID{Cloud: "stratus", Owner: "bob", Name: "foobar"}
	ctx := context.Background()
	err := st.InvalidateCloudCredential(ctx, id, "reason")
	c.Assert(err, jc.ErrorIs, errors.NotFound)
}

type watcherFunc func(namespace, changeValue string, changeMask changestream.ChangeType) (watcher.NotifyWatcher, error)

func (f watcherFunc) NewValueWatcher(
	namespace, changeValue string, changeMask changestream.ChangeType,
) (watcher.NotifyWatcher, error) {
	return f(namespace, changeValue, changeMask)
}

func (s *credentialSuite) watcherFunc(c *gc.C, expectedChangeValue string) watcherFunc {
	return func(namespace, changeValue string, changeMask changestream.ChangeType) (watcher.NotifyWatcher, error) {
		c.Assert(namespace, gc.Equals, "cloud_credential")
		c.Assert(changeMask, gc.Equals, changestream.All)
		c.Assert(changeValue, gc.Equals, expectedChangeValue)

		db, err := s.GetWatchableDB(namespace)
		c.Assert(err, jc.ErrorIsNil)

		base := eventsource.NewBaseWatcher(db, jujutesting.NewCheckLogger(c))
		return eventsource.NewValueWatcher(base, namespace, changeValue, changeMask), nil
	}
}

func (s *credentialSuite) TestWatchCredentialNotFound(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())

	id := credential.ID{Cloud: "stratus", Owner: "bob", Name: "foobar"}
	ctx := context.Background()
	_, err := st.WatchCredential(ctx, s.watcherFunc(c, ""), id)
	c.Assert(err, jc.ErrorIs, errors.NotFound)
}

func (s *credentialSuite) TestWatchCredential(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())
	id := credential.ID{Cloud: "stratus", Owner: "bob", Name: "foobar"}
	s.createCloudCredential(c, st, id)

	var uuid string
	err := s.TxnRunner().Txn(context.Background(), func(ctx context.Context, tx *sqlair.TX) error {
		var err error
		uuid, err = st.credentialUUID(ctx, tx, id)
		return err
	})
	c.Assert(err, jc.ErrorIsNil)

	w, err := st.WatchCredential(context.Background(), s.watcherFunc(c, uuid), id)
	c.Assert(err, jc.ErrorIsNil)
	s.AddCleanup(func(c *gc.C) { workertest.CleanKill(c, w) })

	wc := watchertest.NewNotifyWatcherC(c, w)
	wc.AssertChanges(time.Second) // Initial event.

	credInfo := credential.CloudCredentialInfo{
		AuthType: string(cloud.AccessKeyAuthType),
		Attributes: map[string]string{
			"foo": "foo val",
			"bar": "bar val",
		},
		Revoked: true,
		Label:   "foobar",
	}
	err = s.TxnRunner().Txn(context.Background(), func(ctx context.Context, tx *sqlair.TX) error {
		_, err := st.UpsertCloudCredential(ctx, id, credInfo)
		return err
	})
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertOneChange()

	workertest.CleanKill(c, w)
}

func (s *credentialSuite) TestNoModelsUsingCloudCredential(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())

	ctx := context.Background()
	result, err := st.ModelsUsingCloudCredential(ctx, credential.ID{
		Cloud: "cirrus",
		Owner: "bob",
		Name:  "foobar",
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.HasLen, 0)
}

func (s *credentialSuite) TestModelsUsingCloudCredential(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())

	id := credential.ID{Cloud: "stratus", Owner: "bob", Name: "foobar"}
	one := s.createCloudCredential(c, st, id)
	c.Assert(one.Invalid, jc.IsFalse)

	insertOne := func(ctx context.Context, tx *sql.Tx, modelUUID, name string) error {
		_, err := tx.ExecContext(ctx, fmt.Sprintf(`
		INSERT INTO model_list (uuid) VALUES(%q)`, modelUUID))
		if err != nil {
			return err
		}
		result, err := tx.ExecContext(ctx, fmt.Sprintf(`
		INSERT INTO model_metadata (model_uuid, name, owner_uuid, model_type_id, cloud_uuid, cloud_credential_uuid)
		SELECT %q, %q, %q, 0,
			(SELECT uuid FROM cloud WHERE cloud.name="stratus"),
			(SELECT uuid FROM cloud_credential cc WHERE cc.name="foobar")`,
			modelUUID, name, s.userUUID),
		)
		if err != nil {
			return err
		}
		numRows, err := result.RowsAffected()
		if err != nil {
			return err
		}
		c.Assert(numRows, gc.Equals, int64(1))
		return nil
	}

	modelUUID := uuid.MustNewUUID().String()
	modelUUID2 := uuid.MustNewUUID().String()
	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		if err := insertOne(ctx, tx, modelUUID, "mymodel"); err != nil {
			return err
		}
		if err := insertOne(ctx, tx, modelUUID2, "mymodel2"); err != nil {
			return err
		}
		return nil
	})
	c.Assert(err, jc.ErrorIsNil)

	result, err := st.ModelsUsingCloudCredential(context.Background(), credential.ID{
		Cloud: "stratus",
		Owner: "bob",
		Name:  "foobar",
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, map[model.UUID]string{
		model.UUID(modelUUID):  "mymodel",
		model.UUID(modelUUID2): "mymodel2",
	})
}
