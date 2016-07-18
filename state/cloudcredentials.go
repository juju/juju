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
	if err := iter.Close(); err != nil {
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
		ops = append(ops, updateCloudCredentialsOps(user, cloudName, credentials)...)
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

// updateCloudCredentialsOps returns a list of txn.Ops that will create
// or update a set of cloud credentials for a user.
func updateCloudCredentialsOps(user names.UserTag, cloudName string, credentials map[string]cloud.Credential) []txn.Op {
	owner := user.Canonical()
	ops := make([]txn.Op, 0, len(credentials))
	for name, credential := range credentials {
		ops = append(ops, txn.Op{
			C:      cloudCredentialsC,
			Id:     cloudCredentialDocID(user, cloudName, name),
			Assert: txn.DocMissing,
			Insert: &cloudCredentialDoc{
				Owner:      owner,
				Cloud:      cloudName,
				Name:       name,
				AuthType:   string(credential.AuthType()),
				Attributes: credential.Attributes(),
			},
		})
	}
	return ops
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
