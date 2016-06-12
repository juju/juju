// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"

	"github.com/juju/errors"
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

// initCloudCredentialsOps returns a list of txn.Ops that will initialize
// a set of cloud credentials for a user.
func initCloudCredentialsOps(user names.UserTag, credentials map[string]cloud.Credential) []txn.Op {
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

// CloudCredentials returns the user's cloud credentials, keyed by cloud name.
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
