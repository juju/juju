// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"sort"

	"github.com/juju/errors"
	"github.com/juju/mgo/v3"
	"github.com/juju/mgo/v3/bson"
	"github.com/juju/mgo/v3/txn"
	"github.com/juju/names/v6"
	jujutxn "github.com/juju/txn/v3"
)

// SSHHostKeys holds the public SSH host keys for an entity (almost
// certainly a machine).
//
// The host keys are one line each and are stored in the same format
// as the SSH authorized_keys and ssh_host_key*.pub files.
type SSHHostKeys []string

// sshHostKeysDoc represents the MongoDB document that stores the SSH
// host keys for an entity.
//
// Note that the document id hasn't been included because we don't
// need to read it or (directly) write it.
type sshHostKeysDoc struct {
	Keys []string `bson:"keys"`
}

// GetSSHHostKeys retrieves the SSH host keys stored for an entity.
// /
// NOTE: Currently only machines are supported. This can be
// generalised to take other tag types later, if and when we need it.
func (st *State) GetSSHHostKeys(tag names.MachineTag) (SSHHostKeys, error) {
	coll, closer := st.db().GetCollection(sshHostKeysC)
	defer closer()

	var doc sshHostKeysDoc
	err := coll.FindId(machineGlobalKey(tag.Id())).One(&doc)
	if err == mgo.ErrNotFound {
		return nil, errors.NotFoundf("keys")
	} else if err != nil {
		return nil, errors.Annotate(err, "key lookup failed")
	}
	return SSHHostKeys(doc.Keys), nil
}

// keysEqual checks if the ssh host keys are the same between two sets.
// we shouldn't care about the order of the keys.
func keysEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	a = a[:]
	b = b[:]
	sort.Strings(a)
	sort.Strings(b)
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// SetSSHHostKeys updates the stored SSH host keys for an entity.
//
// See the note for GetSSHHostKeys regarding supported entities.
func (st *State) SetSSHHostKeys(tag names.MachineTag, keys SSHHostKeys) error {
	coll, closer := st.db().GetCollection(sshHostKeysC)
	defer closer()
	id := machineGlobalKey(tag.Id())
	doc := sshHostKeysDoc{
		Keys: keys,
	}
	var dbDoc sshHostKeysDoc
	buildTxn := func(attempt int) ([]txn.Op, error) {
		err := coll.FindId(id).One(&dbDoc)
		if err != nil {
			if err == mgo.ErrNotFound {
				return []txn.Op{{
					C:      sshHostKeysC,
					Id:     id,
					Insert: doc,
				}}, nil
			}
			return nil, err
		}
		if keysEqual(dbDoc.Keys, keys) {
			return nil, jujutxn.ErrNoOperations
		}
		return []txn.Op{{
			C:      sshHostKeysC,
			Id:     id,
			Update: bson.M{"$set": doc},
		}}, nil
	}

	if err := st.db().Run(buildTxn); err != nil {
		return errors.Annotate(err, "SSH host key update failed")
	}
	return nil
}
