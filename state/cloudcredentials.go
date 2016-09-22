// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"

	"github.com/juju/errors"
	jujutxn "github.com/juju/txn"
	"github.com/juju/utils/set"
	"gopkg.in/juju/names.v2"
	mgo "gopkg.in/mgo.v2"
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
	Revoked    bool              `bson:"revoked"`
	AuthType   string            `bson:"auth-type"`
	Attributes map[string]string `bson:"attributes,omitempty"`
}

// CloudCredential returns the cloud credential for the given tag.
func (st *State) CloudCredential(tag names.CloudCredentialTag) (cloud.Credential, error) {
	coll, cleanup := st.getCollection(cloudCredentialsC)
	defer cleanup()

	var doc cloudCredentialDoc
	err := coll.FindId(cloudCredentialDocID(tag)).One(&doc)
	if err == mgo.ErrNotFound {
		return cloud.Credential{}, errors.NotFoundf(
			"cloud credential %q", tag.Id(),
		)
	} else if err != nil {
		return cloud.Credential{}, errors.Annotatef(
			err, "getting cloud credential %q", tag.Id(),
		)
	}
	return doc.toCredential(), nil
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
		tag, err := doc.cloudCredentialTag()
		if err != nil {
			return nil, errors.Trace(err)
		}
		credentials[tag.Canonical()] = doc.toCredential()
	}
	if err := iter.Err(); err != nil {
		return nil, errors.Annotatef(
			err, "cannot get cloud credentials for user %q, cloud %q",
			user.Canonical(), cloudName,
		)
	}
	return credentials, nil
}

// UpdateCloudCredential adds or updates a cloud credential with the given tag.
func (st *State) UpdateCloudCredential(tag names.CloudCredentialTag, credential cloud.Credential) error {
	credentials := map[names.CloudCredentialTag]cloud.Credential{tag: credential}
	buildTxn := func(attempt int) ([]txn.Op, error) {
		cloudName := tag.Cloud().Id()
		cloud, err := st.Cloud(cloudName)
		if err != nil {
			return nil, errors.Trace(err)
		}
		ops, err := validateCloudCredentials(cloud, cloudName, credentials)
		if err != nil {
			return nil, errors.Annotate(err, "validating cloud credentials")
		}
		_, err = st.CloudCredential(tag)
		if err != nil && !errors.IsNotFound(err) {
			return nil, errors.Maskf(err, "fetching cloud credentials")
		}
		if err == nil {
			ops = append(ops, updateCloudCredentialOp(tag, credential))
		} else {
			ops = append(ops, createCloudCredentialOp(tag, credential))
		}
		return ops, nil
	}
	if err := st.run(buildTxn); err != nil {
		return errors.Annotate(err, "updating cloud credentials")
	}
	return nil
}

// RemoveCloudCredential removes a cloud credential with the given tag.
func (st *State) RemoveCloudCredential(tag names.CloudCredentialTag) error {
	buildTxn := func(attempt int) ([]txn.Op, error) {
		_, err := st.CloudCredential(tag)
		if errors.IsNotFound(err) {
			return nil, jujutxn.ErrNoOperations
		}
		if err != nil {
			return nil, errors.Trace(err)
		}
		return removeCloudCredentialOps(tag), nil
	}
	if err := st.run(buildTxn); err != nil {
		return errors.Annotate(err, "removing cloud credential")
	}
	return nil
}

// createCloudCredentialOp returns a txn.Op that will create
// a cloud credential.
func createCloudCredentialOp(tag names.CloudCredentialTag, cred cloud.Credential) txn.Op {
	return txn.Op{
		C:      cloudCredentialsC,
		Id:     cloudCredentialDocID(tag),
		Assert: txn.DocMissing,
		Insert: &cloudCredentialDoc{
			Owner:      tag.Owner().Canonical(),
			Cloud:      tag.Cloud().Id(),
			Name:       tag.Name(),
			AuthType:   string(cred.AuthType()),
			Attributes: cred.Attributes(),
			Revoked:    cred.Revoked,
		},
	}
}

// updateCloudCredentialOp returns a txn.Op that will update
// a cloud credential.
func updateCloudCredentialOp(tag names.CloudCredentialTag, cred cloud.Credential) txn.Op {
	return txn.Op{
		C:      cloudCredentialsC,
		Id:     cloudCredentialDocID(tag),
		Assert: txn.DocExists,
		Update: bson.D{{"$set", bson.D{
			{"auth-type", string(cred.AuthType())},
			{"attributes", cred.Attributes()},
			{"revoked", cred.Revoked},
		}}},
	}
}

// removeCloudCredentialOp returns a txn.Op that will remove
// a cloud credential.
func removeCloudCredentialOps(tag names.CloudCredentialTag) []txn.Op {
	return []txn.Op{{
		C:      cloudCredentialsC,
		Id:     cloudCredentialDocID(tag),
		Assert: txn.DocExists,
		Remove: true,
	}}
}

func cloudCredentialDocID(tag names.CloudCredentialTag) string {
	return fmt.Sprintf("%s#%s#%s", tag.Cloud().Id(), tag.Owner().Canonical(), tag.Name())
}

func (c cloudCredentialDoc) cloudCredentialTag() (names.CloudCredentialTag, error) {
	ownerTag := names.NewUserTag(c.Owner)
	id := fmt.Sprintf("%s/%s/%s", c.Cloud, ownerTag.Canonical(), c.Name)
	if !names.IsValidCloudCredential(id) {
		return names.CloudCredentialTag{}, errors.NotValidf("cloud credential ID")
	}
	return names.NewCloudCredentialTag(id), nil
}

func (c cloudCredentialDoc) toCredential() cloud.Credential {
	out := cloud.NewCredential(cloud.AuthType(c.AuthType), c.Attributes)
	out.Revoked = c.Revoked
	out.Label = c.Name
	return out
}

// validateCloudCredentials checks that the supplied cloud credentials are
// valid for use with the controller's cloud, and returns a set of txn.Ops
// to assert the same in a transaction. The map keys are the cloud credential
// IDs.
//
// TODO(rogpeppe) We're going to a lot of effort here to assert that a
// cloud's auth types haven't changed since we looked at them a moment
// ago, but we don't support changing a cloud's definition currently and
// it's not clear that doing so would be a good idea, as changing a
// cloud's auth type would invalidate all existing credentials and would
// usually involve a new provider version and juju binary too, so
// perhaps all this code is unnecessary.
func validateCloudCredentials(
	cloud cloud.Cloud,
	cloudName string,
	credentials map[names.CloudCredentialTag]cloud.Credential,
) ([]txn.Op, error) {
	requiredAuthTypes := make(set.Strings)
	for tag, credential := range credentials {
		if tag.Cloud().Id() != cloudName {
			return nil, errors.NewNotValid(nil, fmt.Sprintf(
				"credential %q for non-matching cloud is not valid (expected %q)",
				tag.Id(), cloudName,
			))
		}
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
				tag.Id(), credential.AuthType(), cloud.AuthTypes,
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

// WatchCredential returns a new NotifyWatcher watching for
// changes to the specified credential.
func (st *State) WatchCredential(cred names.CloudCredentialTag) NotifyWatcher {
	filter := func(rawId interface{}) bool {
		id, ok := rawId.(string)
		if !ok {
			return false
		}
		return id == cloudCredentialDocID(cred)
	}
	return newNotifyCollWatcher(st, cloudCredentialsC, filter)
}
