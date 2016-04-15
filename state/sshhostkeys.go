// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/errors"
	"github.com/juju/names"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"
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
///
// NOTE: Currently only machines are supported. This can be
// generalised to take other tag types later, if and when we need it.
func (st *State) GetSSHHostKeys(tag names.MachineTag) (SSHHostKeys, error) {
	coll, closer := st.getCollection(sshHostKeysC)
	defer closer()

	var doc sshHostKeysDoc
	err := coll.FindId(machineGlobalKey(tag.Id())).One(&doc)
	if err == mgo.ErrNotFound {
		return nil, errors.NotFoundf("SSH host keys for %s", tag)
	} else if err != nil {
		return nil, errors.Annotate(err, "SSH host key lookup failed")
	}
	return SSHHostKeys(doc.Keys), nil
}

// SetSSHHostKeys updates the stored SSH host keys for an entity.
//
// See the note for GetSSHHostKeys regarding supported entities.
func (st *State) SetSSHHostKeys(tag names.MachineTag, keys SSHHostKeys) error {
	id := machineGlobalKey(tag.Id())
	doc := sshHostKeysDoc{
		Keys: keys,
	}
	err := st.runTransaction([]txn.Op{
		{
			C:      sshHostKeysC,
			Id:     id,
			Insert: doc,
		}, {
			C:      sshHostKeysC,
			Id:     id,
			Update: bson.M{"$set": doc},
		},
	})
	return errors.Annotate(err, "SSH host key update failed")
}

// removeSSHHostKeyOp returns the operation needed to remove the SSH
// host key document associated with the given globalKey.
func removeSSHHostKeyOp(st *State, globalKey string) txn.Op {
	return txn.Op{
		C:      sshHostKeysC,
		Id:     globalKey,
		Remove: true,
	}
}
