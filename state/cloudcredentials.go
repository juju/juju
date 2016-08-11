// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/utils/set"
	"gopkg.in/juju/names.v2"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"

	"github.com/juju/juju/cloud"
)

// cloudCredentialDoc records information about a user's cloud credentials.
type cloudCredentialDoc struct {
	DocID      string            `bson:"_id"`
	Owner      string            `bson:"owner"`
	Cloud      string            `bson:"cloud"`
	Name       string            `bson:"name"`
	AuthType   string            `bson:"auth-type"`
	Attributes map[string]string `bson:"attributes,omitempty"`
}

// CloudCredentials returns the user's cloud credentials for a given cloud,
// keyed by credential name.
func (st *State) CloudCredentials(user names.UserTag, cloudName string) (map[string]cloud.Credential, error) {
	coll, cleanup := st.getCollection(cloudCredentialsC)
	defer cleanup()

	var doc cloudCredentialDoc
	credentials := make(map[string]cloud.Credential)
	iter := coll.Find(bson.D{
		{"owner", user.Canonical()},
		{"cloud", cloudName},
	}).Iter()
	for iter.Next(&doc) {
		credentials[doc.Name] = doc.toCredential()
	}
	if err := iter.Err(); err != nil {
		return nil, errors.Annotatef(
			err, "cannot get cloud credentials for user %q, cloud %q",
			user.Canonical(), cloudName,
		)
	}
	return credentials, nil
}

// UpdateCloudCredentials updates the user's cloud credentials. Any existing
// credentials with the same names will be replaced, and any other credentials
// not in the updated set will be untouched.
func (st *State) UpdateCloudCredentials(user names.UserTag, cloudName string, credentials map[string]cloud.Credential) error {
	buildTxn := func(attempt int) ([]txn.Op, error) {
		cloud, err := st.Cloud(cloudName)
		if err != nil {
			return nil, errors.Trace(err)
		}
		ops, err := validateCloudCredentials(cloud, cloudName, credentials)
		if err != nil {
			return nil, errors.Annotate(err, "validating cloud credentials")
		}
		existingCreds, err := st.CloudCredentials(user, cloudName)
		if err != nil {
			return nil, errors.Maskf(err, "fetching cloud credentials")
		}
		for credName, cred := range credentials {
			if _, ok := existingCreds[credName]; ok {
				ops = append(ops, updateCloudCredentialOp(user, cloudName, credName, cred))
			} else {
				ops = append(ops, createCloudCredentialOp(user, cloudName, credName, cred))
			}
		}
		return ops, nil
	}
	if err := st.run(buildTxn); err != nil {
		return errors.Annotatef(
			err, "updating cloud credentials for user %q, cloud %q",
			user.String(), cloudName,
		)
	}
	return nil
}

// createCloudCredentialOp returns a txn.Op that will create
// a cloud credential.
func createCloudCredentialOp(user names.UserTag, cloudName, credName string, cred cloud.Credential) txn.Op {
	return txn.Op{
		C:      cloudCredentialsC,
		Id:     cloudCredentialDocID(user, cloudName, credName),
		Assert: txn.DocMissing,
		Insert: &cloudCredentialDoc{
			Owner:      user.Canonical(),
			Cloud:      cloudName,
			Name:       credName,
			AuthType:   string(cred.AuthType()),
			Attributes: cred.Attributes(),
		},
	}
}

// updateCloudCredentialOp returns a txn.Op that will update
// a cloud credential.
func updateCloudCredentialOp(user names.UserTag, cloudName, credName string, cred cloud.Credential) txn.Op {
	return txn.Op{
		C:      cloudCredentialsC,
		Id:     cloudCredentialDocID(user, cloudName, credName),
		Assert: txn.DocExists,
		Update: bson.D{{"$set", bson.D{
			{"auth-type", string(cred.AuthType())},
			{"attributes", cred.Attributes()},
		}}},
	}
}

func cloudCredentialDocID(user names.UserTag, cloudName, credentialName string) string {
	return fmt.Sprintf("%s#%s#%s", user.Canonical(), cloudName, credentialName)
}

func (c cloudCredentialDoc) toCredential() cloud.Credential {
	out := cloud.NewCredential(cloud.AuthType(c.AuthType), c.Attributes)
	out.Label = c.Name
	return out
}

// validateCloudCredentials checks that the supplied cloud credentials are
// valid for use with the controller's cloud, and returns a set of txn.Ops
// to assert the same in a transaction.
//
// TODO(rogpeppe) We're going to a lot of effort here to assert that a
// cloud's auth types haven't changed since we looked at them a moment
// ago, but we don't support changing a cloud's definition currently and
// it's not clear that doing so would be a good idea, as changing a
// cloud's auth type would invalidate all existing credentials and would
// usually involve a new provider version and juju binary too, so
// perhaps all this code is unnecessary.
func validateCloudCredentials(cloud cloud.Cloud, cloudName string, credentials map[string]cloud.Credential) ([]txn.Op, error) {
	requiredAuthTypes := make(set.Strings)
	for name, credential := range credentials {
		var found bool
		for _, authType := range cloud.AuthTypes {
			if credential.AuthType() == authType {
				found = true
				break
			}
		}
		if !found {
			return nil, errors.NewNotValid(nil, fmt.Sprintf(
				"credential %q with auth-type %q is not supported (expected one of %q)",
				name, credential.AuthType(), cloud.AuthTypes,
			))
		}
		requiredAuthTypes.Add(string(credential.AuthType()))
	}
	ops := make([]txn.Op, len(requiredAuthTypes))
	for i, authType := range requiredAuthTypes.SortedValues() {
		ops[i] = txn.Op{
			C:      cloudsC,
			Id:     cloudName,
			Assert: bson.D{{"auth-types", authType}},
		}
	}
	return ops, nil
}
