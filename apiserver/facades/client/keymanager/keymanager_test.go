// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package keymanager

import (
	"testing"

	"github.com/juju/collections/transform"
	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/apiserver/errors"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	coremodel "github.com/juju/juju/core/model"
	modeltesting "github.com/juju/juju/core/model/testing"
	coressh "github.com/juju/juju/core/ssh"
	coreuser "github.com/juju/juju/core/user"
	usertesting "github.com/juju/juju/core/user/testing"
	accesserrors "github.com/juju/juju/domain/access/errors"
	"github.com/juju/juju/internal/ssh"
	"github.com/juju/juju/rpc/params"
)

type keyManagerSuite struct {
	blockChecker      *MockBlockChecker
	authorizer        apiservertesting.FakeAuthorizer
	keyManagerService *MockKeyManagerService
	userService       *MockUserService
	apiUser           names.UserTag

	controllerUUID string
	modelID        coremodel.UUID
}

func TestKeyManagerSuite(t *testing.T) {
	tc.Run(t, &keyManagerSuite{})
}

var (
	// testingPublicKeys represents a set of keys that can be used and are valid.
	testingPublicKeys = []string{
		// ecdsa testing public key
		"ecdsa-sha2-nistp256 AAAAE2VjZHNhLXNoYTItbmlzdHAyNTYAAAAIbmlzdHAyNTYAAABBBG00bYFLb/sxPcmVRMg8NXZK/ldefElAkC9wD41vABdHZiSRvp+2y9BMNVYzE/FnzKObHtSvGRX65YQgRn7k5p0= juju@example.com",

		// ed25519 testing public key
		"ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIN8h8XBpjS9aBUG5cdoSWubs7wT2Lc/BEZIUQCqoaOZR juju@example.com",

		// rsa testing public key
		"ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABgQDvplNOK3UBpULZKvZf/I5JHci/DufpSxj8yR4yKE2grescJxu6754jPT3xztSeLGD31/oJApJZGkMUAMRenvDqIaq+taRfOUo/l19AlGZc+Edv4bTlJzZ1Lzwex1vvL1doaLb/f76IIUHClGUgIXRceQH1ovHiIWj6nGltuLanG8YTWxlzzK33yhitmZt142DmpX1VUVF5c/Hct6Rav5lKmwej1TDed1KmHzXVoTHEsmWhKsOK27ue5yTuq0GX6LrAYDucF+2MqZCsuddXsPAW1tj5GNZSR7RrKW5q1CI0G7k9gSomuCsRMlCJ3BqID/vUSs/0qOWg4he0HUsYKQSrXIhckuZu+jYP8B80MoXT50ftRidoG/zh/PugBdXTk46FloVClQopG5A2fbqrphADcUUbRUxZ2lWQN+OVHKfEsfV2b8L2aSqZUGlryfW1cirB5JCTDvtv7rUy9/ny9iKA+8tAyKSDF0I901RDDqKc9dSkrHCg2bLnJZDoiRoWczE= juju@example.com",
	}
)

func genListPublicKey(c *tc.C, keys []string) []coressh.PublicKey {
	rval := make([]coressh.PublicKey, 0, len(keys))
	for _, key := range keys {
		parsedKey, err := ssh.ParsePublicKey(key)
		c.Assert(err, tc.ErrorIsNil)
		rval = append(rval, coressh.PublicKey{
			Key:         key,
			Fingerprint: parsedKey.Fingerprint(),
		})
	}

	return rval
}

func (s *keyManagerSuite) SetUpTest(c *tc.C) {
	s.apiUser = names.NewUserTag("admin")
	s.modelID = modeltesting.GenModelUUID(c)
	s.controllerUUID = "controller"
}

func (s *keyManagerSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.blockChecker = NewMockBlockChecker(ctrl)
	s.keyManagerService = NewMockKeyManagerService(ctrl)
	s.userService = NewMockUserService(ctrl)
	s.authorizer = apiservertesting.FakeAuthorizer{
		Tag: s.apiUser,
	}
	return ctrl
}

// TestListKeysForUserNotFound is asserting that if we attempt to list keys for
// a user that doesn't exist we get back a [params.CodeUserNotFound] error.
func (s *keyManagerSuite) TestListKeysForUserNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.userService.EXPECT().GetUserByName(gomock.Any(), coreuser.AdminUserName).Return(
		coreuser.User{},
		accesserrors.UserNotFound,
	)
	args := params.ListSSHKeys{
		Entities: params.Entities{Entities: []params.Entity{
			{},
		}},
		Mode: params.SSHListModeFull,
	}

	api := newKeyManagerAPI(
		s.keyManagerService,
		s.userService,
		s.authorizer,
		s.blockChecker,
		s.controllerUUID,
		s.modelID,
		s.apiUser,
	)

	results, err := api.ListKeys(c.Context(), args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results, tc.DeepEquals, params.StringsResults{
		Results: []params.StringsResult{
			{
				Error: &params.Error{
					Code:    params.CodeUserNotFound,
					Message: "user \"admin\" does not exist",
				},
			},
		},
	})
}

// TestListKeys is asserting all the cases under which ListKeys can work and
// fail. Specifically because this facade func supports retrieving the keys for
// multiple entities we are passing a combination of entities that will succeed
// and fail for various reasons.
//
// We are also asserting that the results are passed back in the same order that
// they are received.
func (s *keyManagerSuite) TestListKeys(c *tc.C) {
	defer s.setupMocks(c).Finish()

	userID := usertesting.GenUserUUID(c)
	s.userService.EXPECT().GetUserByName(gomock.Any(), coreuser.AdminUserName).Return(coreuser.User{
		UUID: userID,
	}, nil)
	s.keyManagerService.EXPECT().ListPublicKeysForUser(
		gomock.Any(),
		userID,
	).Return(genListPublicKey(c, testingPublicKeys), nil)

	args := params.ListSSHKeys{
		Entities: params.Entities{Entities: []params.Entity{
			// Valid username that should succeed.
			{},
		}},
		Mode: params.SSHListModeFull,
	}

	api := newKeyManagerAPI(
		s.keyManagerService,
		s.userService,
		s.authorizer,
		s.blockChecker,
		s.controllerUUID,
		s.modelID,
		s.apiUser,
	)

	results, err := api.ListKeys(c.Context(), args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results, tc.DeepEquals, params.StringsResults{
		Results: []params.StringsResult{
			{
				Result: testingPublicKeys,
			},
		},
	})
}

// TestListKeysFingerprintMode is testing that when the list mode is set to
// fingerprint we get back the ssh public key fingerprints.
func (s *keyManagerSuite) TestListKeysFingerprintMode(c *tc.C) {
	defer s.setupMocks(c).Finish()

	userID := usertesting.GenUserUUID(c)
	s.userService.EXPECT().GetUserByName(gomock.Any(), coreuser.AdminUserName).Return(coreuser.User{
		UUID: userID,
	}, nil)

	testPublicKeys := genListPublicKey(c, testingPublicKeys)
	fingerprints := transform.Slice(testPublicKeys, func(pk coressh.PublicKey) string {
		return pk.Fingerprint
	})

	s.keyManagerService.EXPECT().ListPublicKeysForUser(
		gomock.Any(),
		userID,
	).Return(testPublicKeys, nil)

	args := params.ListSSHKeys{
		Entities: params.Entities{Entities: []params.Entity{
			{Tag: names.NewUserTag("admin").String()},
		}},
		Mode: params.SSHListModeFingerprint,
	}

	api := newKeyManagerAPI(
		s.keyManagerService,
		s.userService,
		s.authorizer,
		s.blockChecker,
		s.controllerUUID,
		s.modelID,
		s.apiUser,
	)

	results, err := api.ListKeys(c.Context(), args)
	c.Check(err, tc.ErrorIsNil)
	c.Check(results, tc.DeepEquals, params.StringsResults{
		Results: []params.StringsResult{
			{
				Result: fingerprints,
			},
		},
	})
}

// TestListKeysNoPermissions is testing that if a user doesn't have at least
// read permission on to the model that we get back a permission denied error.
func (s *keyManagerSuite) TestListKeysNoPermission(c *tc.C) {
	defer s.setupMocks(c).Finish()

	userTag := names.NewUserTag("tlm")
	s.authorizer.Tag = userTag

	args := params.ListSSHKeys{
		Entities: params.Entities{Entities: []params.Entity{
			{Tag: names.NewUserTag("admin").String()},
		}},
		Mode: params.SSHListModeFingerprint,
	}

	api := newKeyManagerAPI(
		s.keyManagerService,
		s.userService,
		s.authorizer,
		s.blockChecker,
		s.controllerUUID,
		s.modelID,
		userTag,
	)

	_, err := api.ListKeys(c.Context(), args)
	c.Check(err, tc.DeepEquals, &params.Error{
		Code:    params.CodeUnauthorized,
		Message: "permission denied",
	})
}

// TestAddKeysForUser is here to assert the happy path of adding keys for a user.
func (s *keyManagerSuite) TestAddKeysForUser(c *tc.C) {
	defer s.setupMocks(c).Finish()

	userID := usertesting.GenUserUUID(c)
	s.userService.EXPECT().GetUserByName(gomock.Any(), coreuser.AdminUserName).Return(coreuser.User{
		UUID: userID,
	}, nil)

	s.keyManagerService.EXPECT().AddPublicKeysForUser(
		gomock.Any(),
		userID,
		testingPublicKeys,
	).Return(nil)
	s.blockChecker.EXPECT().ChangeAllowed(gomock.Any()).Return(nil)

	args := params.ModifyUserSSHKeys{
		Keys: testingPublicKeys,
	}

	api := newKeyManagerAPI(
		s.keyManagerService,
		s.userService,
		s.authorizer,
		s.blockChecker,
		s.controllerUUID,
		s.modelID,
		s.apiUser,
	)

	res, err := api.AddKeys(c.Context(), args)
	c.Check(err, tc.ErrorIsNil)
	c.Check(res, tc.DeepEquals, params.ErrorResults{})
}

// TestAddKeysSuperUser is testing that a user with superuser permissions can
// add keys to the model.
func (s *keyManagerSuite) TestAddKeysSuperUser(c *tc.C) {
	s.apiUser = names.NewUserTag("superuser-fred")
	defer s.setupMocks(c).Finish()

	userID := usertesting.GenUserUUID(c)
	s.userService.EXPECT().GetUserByName(gomock.Any(), usertesting.GenNewName(c, "superuser-fred")).Return(coreuser.User{
		UUID: userID,
	}, nil)

	s.keyManagerService.EXPECT().AddPublicKeysForUser(
		gomock.Any(),
		userID,
		testingPublicKeys,
	).Return(nil)
	s.blockChecker.EXPECT().ChangeAllowed(gomock.Any()).Return(nil)

	args := params.ModifyUserSSHKeys{
		Keys: testingPublicKeys,
	}

	api := newKeyManagerAPI(
		s.keyManagerService,
		s.userService,
		s.authorizer,
		s.blockChecker,
		s.controllerUUID,
		s.modelID,
		s.apiUser,
	)

	res, err := api.AddKeys(c.Context(), args)
	c.Check(err, tc.ErrorIsNil)
	c.Check(res, tc.DeepEquals, params.ErrorResults{})
}

// TestAddKeysModelAdmin is testing that model admin's have permissions to add
// public keys.
func (s *keyManagerSuite) TestAddKeysModelAdmin(c *tc.C) {
	s.apiUser = names.NewUserTag("admin-" + names.NewModelTag(s.modelID.String()).String())
	defer s.setupMocks(c).Finish()

	userID := usertesting.GenUserUUID(c)
	s.userService.EXPECT().GetUserByName(gomock.Any(), coreuser.NameFromTag(s.apiUser)).Return(coreuser.User{
		UUID: userID,
	}, nil)

	s.keyManagerService.EXPECT().AddPublicKeysForUser(
		gomock.Any(),
		userID,
		testingPublicKeys,
	).Return(nil)
	s.blockChecker.EXPECT().ChangeAllowed(gomock.Any()).Return(nil)

	args := params.ModifyUserSSHKeys{
		Keys: testingPublicKeys,
	}

	api := newKeyManagerAPI(
		s.keyManagerService,
		s.userService,
		s.authorizer,
		s.blockChecker,
		s.controllerUUID,
		s.modelID,
		s.apiUser,
	)

	res, err := api.AddKeys(c.Context(), args)
	c.Check(err, tc.ErrorIsNil)
	c.Check(res, tc.DeepEquals, params.ErrorResults{})
}

// TestAddKeysNonAuthorised is testing that if a user that isn't authorised for
// adding keys to a model attempts to add keys they get back a permission error.
func (s *keyManagerSuite) TestAddKeysNonAuthorised(c *tc.C) {
	s.apiUser = names.NewUserTag("tlm")
	defer s.setupMocks(c).Finish()

	args := params.ModifyUserSSHKeys{
		Keys: testingPublicKeys,
	}

	api := newKeyManagerAPI(
		s.keyManagerService,
		s.userService,
		s.authorizer,
		s.blockChecker,
		s.controllerUUID,
		s.modelID,
		s.apiUser,
	)

	_, err := api.AddKeys(c.Context(), args)
	c.Check(err, tc.DeepEquals, &params.Error{
		Code:    params.CodeUnauthorized,
		Message: "permission denied",
	})
}

// TestBlockAddKeys is testing that if a change allowed block is in place that
// no keys can be added to the model.
func (s *keyManagerSuite) TestBlockAddKeys(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.blockChecker.EXPECT().ChangeAllowed(gomock.Any()).Return(errors.OperationBlockedError("TestAddKeys"))

	args := params.ModifyUserSSHKeys{
		Keys: testingPublicKeys,
	}

	api := newKeyManagerAPI(
		s.keyManagerService,
		s.userService,
		s.authorizer,
		s.blockChecker,
		s.controllerUUID,
		s.modelID,
		s.apiUser,
	)

	_, err := api.AddKeys(c.Context(), args)
	c.Check(err, tc.DeepEquals, &params.Error{
		Code:    params.CodeOperationBlocked,
		Message: "TestAddKeys",
	})
}

// TestDeleteKeys is testing the happy path of deleting public keys for a user.
func (s *keyManagerSuite) TesDeleteKeys(c *tc.C) {
	defer s.setupMocks(c).Finish()

	userID := usertesting.GenUserUUID(c)
	s.userService.EXPECT().GetUserByName(gomock.Any(), coreuser.AdminUserName).Return(coreuser.User{
		UUID: userID,
	}, nil)

	s.keyManagerService.EXPECT().DeleteKeysForUser(
		gomock.Any(),
		userID,
		testingPublicKeys,
	).Return(nil)
	s.blockChecker.EXPECT().RemoveAllowed(gomock.Any()).Return(nil)

	args := params.ModifyUserSSHKeys{
		Keys: testingPublicKeys,
	}

	api := newKeyManagerAPI(
		s.keyManagerService,
		s.userService,
		s.authorizer,
		s.blockChecker,
		s.controllerUUID,
		s.modelID,
		s.apiUser,
	)

	res, err := api.DeleteKeys(c.Context(), args)
	c.Check(err, tc.ErrorIsNil)
	c.Check(res, tc.DeepEquals, params.ErrorResults{})
}

// TestDeleteKeysSuperUser is asserting that a super user can remove public ssh
// keys for a model.
func (s *keyManagerSuite) TestDeleteKeysSuperUser(c *tc.C) {
	s.apiUser = names.NewUserTag("superuser-fred")
	defer s.setupMocks(c).Finish()

	userID := usertesting.GenUserUUID(c)
	s.userService.EXPECT().GetUserByName(gomock.Any(), usertesting.GenNewName(c, "superuser-fred")).Return(coreuser.User{
		UUID: userID,
	}, nil)

	s.keyManagerService.EXPECT().DeleteKeysForUser(
		gomock.Any(),
		userID,
		testingPublicKeys,
	).Return(nil)
	s.blockChecker.EXPECT().RemoveAllowed(gomock.Any()).Return(nil)

	args := params.ModifyUserSSHKeys{
		Keys: testingPublicKeys,
	}

	api := newKeyManagerAPI(
		s.keyManagerService,
		s.userService,
		s.authorizer,
		s.blockChecker,
		s.controllerUUID,
		s.modelID,
		s.apiUser,
	)

	res, err := api.DeleteKeys(c.Context(), args)
	c.Check(err, tc.ErrorIsNil)
	c.Check(res, tc.DeepEquals, params.ErrorResults{})
}

// TestDeleteKeysModelAdmin is asserting that model admins can removed public
// ssh keys from the model.
func (s *keyManagerSuite) TestDeleteKeysModelAdmin(c *tc.C) {
	s.apiUser = names.NewUserTag("admin" + names.NewModelTag(s.modelID.String()).String())
	defer s.setupMocks(c).Finish()

	userID := usertesting.GenUserUUID(c)
	s.userService.EXPECT().GetUserByName(gomock.Any(), coreuser.NameFromTag(s.apiUser)).Return(coreuser.User{
		UUID: userID,
	}, nil)

	s.keyManagerService.EXPECT().DeleteKeysForUser(
		gomock.Any(),
		userID,
		testingPublicKeys,
	).Return(nil)
	s.blockChecker.EXPECT().RemoveAllowed(gomock.Any()).Return(nil)

	args := params.ModifyUserSSHKeys{
		Keys: testingPublicKeys,
	}

	api := newKeyManagerAPI(
		s.keyManagerService,
		s.userService,
		s.authorizer,
		s.blockChecker,
		s.controllerUUID,
		s.modelID,
		s.apiUser,
	)

	res, err := api.DeleteKeys(c.Context(), args)
	c.Check(err, tc.ErrorIsNil)
	c.Check(res, tc.DeepEquals, params.ErrorResults{})
}

// TestDeleteKeysNonAuthorised is asserting that user that is not authorised for
// writing to a model cannot not remove keys from the model and receives an
// unauthorized error.
func (s *keyManagerSuite) TestDeleteKeysNonAuthorised(c *tc.C) {
	s.apiUser = names.NewUserTag("tlm")
	defer s.setupMocks(c).Finish()

	args := params.ModifyUserSSHKeys{
		Keys: testingPublicKeys,
	}

	api := newKeyManagerAPI(
		s.keyManagerService,
		s.userService,
		s.authorizer,
		s.blockChecker,
		s.controllerUUID,
		s.modelID,
		s.apiUser,
	)

	_, err := api.DeleteKeys(c.Context(), args)
	c.Check(err, tc.DeepEquals, &params.Error{
		Code:    params.CodeUnauthorized,
		Message: "permission denied",
	})
}

// TestBlockDeleteKeys is testing that if we try and delete any model keys while
// a remove block is in place the operation results in a operation blocked
// error.
func (s *keyManagerSuite) TestBlockDeleteKeys(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.blockChecker.EXPECT().RemoveAllowed(gomock.Any()).Return(errors.OperationBlockedError("TestDeleteKeys"))

	args := params.ModifyUserSSHKeys{
		Keys: testingPublicKeys,
	}

	api := newKeyManagerAPI(
		s.keyManagerService,
		s.userService,
		s.authorizer,
		s.blockChecker,
		s.controllerUUID,
		s.modelID,
		s.apiUser,
	)

	_, err := api.DeleteKeys(c.Context(), args)
	c.Check(err, tc.DeepEquals, &params.Error{
		Code:    params.CodeOperationBlocked,
		Message: "TestDeleteKeys",
	})
}

//func (s *keyManagerSuite) TestBlockDeleteKeys(c *tc.C) {
//	defer s.setup(c).Finish()
//	s.blockChecker.EXPECT().RemoveAllowed(gomock.Any()).Return(errors.OperationBlockedError("TestDeleteKeys"))
//
//	_, err := s.api.DeleteKeys(c.Context(), params.ModifyUserSSHKeys{})
//
//	c.Assert(params.IsCodeOperationBlocked(err), tc.IsTrue)
//}
//
//func (s *keyManagerSuite) TestDeleteJujuSystemKey(c *tc.C) {
//	defer s.setup(c).Finish()
//	s.blockChecker.EXPECT().RemoveAllowed(gomock.Any()).Return(nil)
//
//	key1 := sshtesting.ValidKeyOne.Key + " juju-client-key"
//	key2 := sshtesting.ValidKeyTwo.Key + " " + config.JujuSystemKey
//	key3 := sshtesting.ValidKeyThree.Key + " a user key"
//	s.setAuthorizedKeys(c, key1, key2, key3)
//
//	newAttrs := map[string]interface{}{
//		config.AuthorizedKeysKey: strings.Join([]string{key1, key2, key3}, "\n"),
//	}
//	s.model.EXPECT().UpdateModelConfig(gomock.Any(), newAttrs, nil)
//
//	args := params.ModifyUserSSHKeys{
//		User: names.NewUserTag("admin").Name(),
//		Keys: []string{"juju-client-key", config.JujuSystemKey},
//	}
//	results, err := s.api.DeleteKeys(c.Context(), args)
//	c.Assert(err, tc.ErrorIsNil)
//	c.Assert(results, tc.DeepEquals, params.ErrorResults{
//		Results: []params.ErrorResult{
//			{Error: apiservertesting.ServerError("may not delete internal key: juju-client-key")},
//			{Error: apiservertesting.ServerError("may not delete internal key: " + config.JujuSystemKey)},
//		},
//	})
//}
//
//// This should be impossible to do anyway since it's impossible to request
//// to remove the client and system key
//func (s *keyManagerSuite) TestCannotDeleteAllKeys(c *tc.C) {
//	defer s.setup(c).Finish()
//	s.blockChecker.EXPECT().RemoveAllowed(gomock.Any()).Return(nil)
//
//	key1 := sshtesting.ValidKeyOne.Key + " user@host"
//	key2 := sshtesting.ValidKeyTwo.Key
//	s.setAuthorizedKeys(c, key1, key2)
//
//	args := params.ModifyUserSSHKeys{
//		User: names.NewUserTag("admin").String(),
//		Keys: []string{sshtesting.ValidKeyTwo.Fingerprint, "user@host"},
//	}
//	_, err := s.api.DeleteKeys(c.Context(), args)
//	c.Assert(err, tc.ErrorMatches, "cannot delete all keys")
//}
//
//func (s *keyManagerSuite) assertImportKeys(c *tc.C) {
//	key1 := sshtesting.ValidKeyOne.Key + " user@host"
//	key2 := sshtesting.ValidKeyTwo.Key
//	key3 := sshtesting.ValidKeyThree.Key
//	key4 := sshtesting.ValidKeyFour.Key
//	keymv := strings.Split(sshtesting.ValidKeyMulti, "\n")
//	keymp := strings.Split(sshtesting.PartValidKeyMulti, "\n")
//	keymi := strings.Split(sshtesting.MultiInvalid, "\n")
//	s.setAuthorizedKeys(c, key1, key2, "bad key")
//
//	newAttrs := map[string]interface{}{
//		config.AuthorizedKeysKey: strings.Join([]string{
//			key1, key2, "bad key", key3, keymv[0], keymv[1], keymp[0], key4,
//		}, "\n"),
//	}
//	s.model.EXPECT().UpdateModelConfig(gomock.Any(), newAttrs, nil)
//
//	args := params.ModifyUserSSHKeys{
//		User: names.NewUserTag("admin").String(),
//		Keys: []string{
//			"lp:existing",
//			"lp:validuser",
//			"invalid-key",
//			"lp:multi",
//			"lp:multiempty",
//			"lp:multipartial",
//			"lp:multiinvalid",
//			"lp:multionedup",
//		},
//	}
//	results, err := s.api.ImportKeys(c.Context(), args)
//
//	c.Assert(err, tc.ErrorIsNil)
//	c.Assert(results.Results, tc.HasLen, 8)
//	c.Assert(results, tc.DeepEquals, params.ErrorResults{
//		Results: []params.ErrorResult{
//			{Error: apiservertesting.ServerError(fmt.Sprintf("duplicate ssh key: %s", key2))},
//			{Error: nil},
//			{Error: apiservertesting.ServerError("invalid ssh key id: invalid-key")},
//			{Error: nil},
//			{Error: apiservertesting.ServerError("invalid ssh key id: lp:multiempty")},
//			{Error: apiservertesting.ServerError(fmt.Sprintf(
//				`invalid ssh key for lp:multipartial: `+
//					`generating key fingerprint: `+
//					`invalid authorized_key "%s"`, keymp[1]))},
//			{Error: apiservertesting.ServerError(fmt.Sprintf(
//				`invalid ssh key for lp:multiinvalid: `+
//					`generating key fingerprint: `+
//					`invalid authorized_key "%s"`+"\n"+
//					`invalid ssh key for lp:multiinvalid: `+
//					`generating key fingerprint: `+
//					`invalid authorized_key "%s"`, keymi[0], keymi[1]))},
//			{Error: apiservertesting.ServerError(fmt.Sprintf("duplicate ssh key: %s", key2))},
//		},
//	})
//}
//
//func (s *keyManagerSuite) TestImportKeys(c *tc.C) {
//	defer s.setup(c).Finish()
//	s.blockChecker.EXPECT().ChangeAllowed(gomock.Any()).Return(nil)
//	s.assertImportKeys(c)
//}
//
//func (s *keyManagerSuite) TestImportKeysSuperUser(c *tc.C) {
//	s.apiUser = names.NewUserTag("superuser-fred")
//	defer s.setup(c).Finish()
//	s.blockChecker.EXPECT().ChangeAllowed(gomock.Any()).Return(nil)
//	s.assertImportKeys(c)
//}
//
//func (s *keyManagerSuite) TestImportKeysModelAdmin(c *tc.C) {
//	s.apiUser = names.NewUserTag("admin" + coretesting.ModelTag.String())
//	defer s.setup(c).Finish()
//	s.blockChecker.EXPECT().ChangeAllowed(gomock.Any()).Return(nil)
//	s.assertImportKeys(c)
//}
//
//func (s *keyManagerSuite) TestImportKeysNonAuthorised(c *tc.C) {
//	s.apiUser = names.NewUserTag("fred")
//	defer s.setup(c).Finish()
//
//	_, err := s.api.ImportKeys(c.Context(), params.ModifyUserSSHKeys{})
//	c.Assert(err, tc.ErrorMatches, "permission denied")
//	c.Assert(params.ErrCode(err), tc.Equals, params.CodeUnauthorized)
//}
//
//func (s *keyManagerSuite) TestImportJujuSystemKey(c *tc.C) {
//	defer s.setup(c).Finish()
//	s.blockChecker.EXPECT().ChangeAllowed(gomock.Any()).Return(nil)
//
//	key1 := sshtesting.ValidKeyOne.Key
//	s.setAuthorizedKeys(c, key1)
//	newAttrs := map[string]interface{}{
//		config.AuthorizedKeysKey: key1,
//	}
//	s.model.EXPECT().UpdateModelConfig(gomock.Any(), newAttrs, nil)
//
//	args := params.ModifyUserSSHKeys{
//		User: names.NewUserTag("admin").String(),
//		Keys: []string{"lp:systemkey"},
//	}
//	results, err := s.api.ImportKeys(c.Context(), args)
//	c.Assert(err, tc.IsNil)
//	c.Assert(results, tc.DeepEquals, params.ErrorResults{
//		Results: []params.ErrorResult{
//			{Error: apiservertesting.ServerError("may not add key with comment juju-system-key: " + keymanagertesting.SystemKey)},
//		},
//	})
//}
//
//func (s *keyManagerSuite) TestBlockImportKeys(c *tc.C) {
//	defer s.setup(c).Finish()
//	s.blockChecker.EXPECT().ChangeAllowed(gomock.Any()).Return(errors.OperationBlockedError("TestImportKeys"))
//
//	_, err := s.api.ImportKeys(c.Context(), params.ModifyUserSSHKeys{})
//
//	c.Assert(params.IsCodeOperationBlocked(err), tc.IsTrue)
//}
//
