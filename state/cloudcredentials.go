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
	Name       string            `bson:"name"`
	AuthType   string            `bson:"auth-type"`
	Attributes map[string]string `bson:"attributes,omitempty"`
}

// CloudCredentials returns the user's cloud credentials, keyed by credential name.
func (st *State) CloudCredentials(user names.UserTag) (map[string]cloud.Credential, error) {
	coll, cleanup := st.getCollection(cloudCredentialsC)
	defer cleanup()

	var doc cloudCredentialDoc
	credentials := make(map[string]cloud.Credential)
	iter := coll.Find(bson.D{{"owner", user.Canonical()}}).Iter()
	for iter.Next(&doc) {
		credentials[doc.Name] = doc.toCredential()
	}
	if err := iter.Close(); err != nil {
		return nil, errors.Annotatef(err, "cannot get cloud credentials for %q", user.Canonical())
	}
	return credentials, nil
}

// UpdateCloudCredentials updates the user's cloud credentials. Any existing
// credentials with the same names will be replaced, and any other credentials
// not in the updated set will be untouched.
func (st *State) UpdateCloudCredentials(user names.UserTag, credentials map[string]cloud.Credential) error {
	buildTxn := func(attempt int) ([]txn.Op, error) {
		cloud, err := st.Cloud()
		if err != nil {
			return nil, errors.Trace(err)
		}
		ops, err := validateCloudCredentials(cloud, credentials)
		if err != nil {
			return nil, errors.Annotate(err, "validating cloud credentials")
		}
		ops = append(ops, updateCloudCredentialsOps(user, credentials)...)
		return ops, nil
	}
	if err := st.run(buildTxn); err != nil {
		return errors.Annotatef(err, "updating cloud credentials for %q", user.String())
	}
	return nil
}

// updateCloudCredentialsOps returns a list of txn.Ops that will create
// or update a set of cloud credentials for a user.
func updateCloudCredentialsOps(user names.UserTag, credentials map[string]cloud.Credential) []txn.Op {
	owner := user.Canonical()
	ops := make([]txn.Op, 0, len(credentials))
	for name, credential := range credentials {
		ops = append(ops, txn.Op{
			C:      cloudCredentialsC,
			Id:     cloudCredentialDocID(user, name),
			Assert: txn.DocMissing,
			Insert: &cloudCredentialDoc{
				Owner:      owner,
				Name:       name,
				AuthType:   string(credential.AuthType()),
				Attributes: credential.Attributes(),
			},
		})
	}
	return ops
}

func cloudCredentialDocID(user names.UserTag, credentialName string) string {
	return fmt.Sprintf("%s#%s", user.Canonical(), credentialName)
}

func (c cloudCredentialDoc) toCredential() cloud.Credential {
	out := cloud.NewCredential(cloud.AuthType(c.AuthType), c.Attributes)
	out.Label = c.Name
	return out
}

// validateCloudCredentials checks that the supplied cloud credentials are
// valid for use with the controller's cloud, and returns a set of txn.Ops
// to assert the same in a transaction.
func validateCloudCredentials(cloud cloud.Cloud, credentials map[string]cloud.Credential) ([]txn.Op, error) {
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
			C:      controllersC,
			Id:     controllerCloudKey,
			Assert: bson.D{{"auth-types", authType}},
		}
	}
	return ops, nil
}
