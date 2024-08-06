// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"
	"sort"

	"github.com/juju/errors"
	"github.com/juju/mgo/v3"
	"github.com/juju/mgo/v3/bson"
	"github.com/juju/mgo/v3/txn"
	"github.com/juju/names/v5"
	jujutxn "github.com/juju/txn/v3"
	"golang.org/x/crypto/ssh"

	pkissh "github.com/juju/juju/pki/ssh"
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
	Keys            []string `bson:"keys"`
	ProxyPublicKey  string   `bson:"proxy-public-key,omitempty"`
	ProxyPrivateKey string   `bson:"proxy-private-key,omitempty"`
}

// GetSSHHostKeys retrieves the SSH host keys stored for an entity.
// /
// NOTE: Currently only machines are supported. This can be
// generalised to take other tag types later, if and when we need it.
func (st *State) GetSSHHostKeys(tag names.Tag) (SSHHostKeys, error) {
	doc, err := st.getSSHHostKeysDoc(tag)
	if err != nil {
		return nil, err
	}
	keys := SSHHostKeys(doc.Keys)
	if doc.ProxyPublicKey != "" {
		keys = append(keys, doc.ProxyPublicKey)
	}
	return keys, nil
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
	var dbDoc sshHostKeysDoc
	buildTxn := func(attempt int) ([]txn.Op, error) {
		err := coll.FindId(id).One(&dbDoc)
		if err == mgo.ErrNotFound {
			doc := sshHostKeysDoc{
				Keys: keys,
			}
			return []txn.Op{{
				C:      sshHostKeysC,
				Id:     id,
				Assert: txn.DocMissing,
				Insert: doc,
			}}, nil
		} else if err != nil {
			return nil, err
		}
		if keysEqual(dbDoc.Keys, keys) {
			return nil, jujutxn.ErrNoOperations
		}
		return []txn.Op{{
			C:      sshHostKeysC,
			Id:     id,
			Update: bson.M{"$set": bson.D{{"keys", keys}}},
		}}, nil
	}

	if err := st.db().Run(buildTxn); err != nil {
		return errors.Annotate(err, "SSH host key update failed")
	}
	return nil
}

// removeSSHHostKeyOp returns the operation needed to remove the SSH
// host key document associated with the given globalKey.
func removeSSHHostKeyOp(globalKey string) txn.Op {
	return txn.Op{
		C:      sshHostKeysC,
		Id:     globalKey,
		Remove: true,
	}
}

func (st *State) GetSSHProxyHostKeys(unit names.Tag) (ssh.Signer, error) {
	doc, err := st.getSSHHostKeysDoc(unit)
	if err != nil && !errors.Is(err, errors.NotFound) {
		return nil, err
	}
	if doc != nil && doc.ProxyPrivateKey != "" {
		privateKey, err := ssh.ParsePrivateKey([]byte(doc.ProxyPrivateKey))
		if err != nil {
			return nil, fmt.Errorf("cannot parse private proxy ssh host key for %v: %w", unit, err)
		}
		return privateKey, nil
	}

	newPrivateKey, err := pkissh.ED25519()
	if err != nil {
		return nil, fmt.Errorf("cannot generate ed25519 ssh key pair for %v: %w", unit, err)
	}
	private, public, _, err := pkissh.FormatKey(newPrivateKey, fmt.Sprintf("%s@%s", unit.String(), st.modelTag.Id()))
	if err != nil {
		return nil, fmt.Errorf("cannot format ed25519 ssh key pair for %v: %w", unit, err)
	}

	coll, closer := st.db().GetCollection(sshHostKeysC)
	defer closer()

	reload := false
	id := unitGlobalKey(unit.Id())
	buildTxn := func(attempt int) ([]txn.Op, error) {
		var dbDoc sshHostKeysDoc
		err := coll.FindId(id).One(&dbDoc)
		if err == mgo.ErrNotFound {
			doc := sshHostKeysDoc{
				ProxyPublicKey:  public,
				ProxyPrivateKey: private,
			}
			return []txn.Op{{
				C:      sshHostKeysC,
				Id:     id,
				Assert: txn.DocMissing,
				Insert: doc,
			}}, nil
		} else if err != nil {
			return nil, err
		}
		if doc.ProxyPrivateKey != "" {
			reload = true
			return nil, jujutxn.ErrNoOperations
		}
		return []txn.Op{{
			C:      sshHostKeysC,
			Id:     id,
			Assert: bson.M{"proxy-private-key": bson.M{"$exists": false}},
			Update: bson.M{"$set": bson.M{
				"proxy-public-key":  public,
				"proxy-private-key": private,
			}},
		}}, nil
	}
	if err := st.db().Run(buildTxn); err != nil {
		return nil, fmt.Errorf("cannot update ssh proxy host keys for %v: %w", unit, err)
	}

	if !reload {
		return ssh.NewSignerFromKey(newPrivateKey)
	}

	doc, err = st.getSSHHostKeysDoc(unit)
	if err != nil {
		return nil, fmt.Errorf("cannot load ssh proxy host keys for %v: %w", unit, err)
	}
	privateKey, err := ssh.ParsePrivateKey([]byte(doc.ProxyPrivateKey))
	if err != nil {
		return nil, fmt.Errorf("cannot parse private proxy ssh host key for %v: %w", unit, err)
	}
	return privateKey, nil
}

func (st *State) getSSHHostKeysDoc(tag names.Tag) (*sshHostKeysDoc, error) {
	coll, closer := st.db().GetCollection(sshHostKeysC)
	defer closer()

	id := ""
	switch t := tag.(type) {
	case names.MachineTag:
		id = machineGlobalKey(t.Id())
	case names.UnitTag:
		id = unitGlobalKey(t.Id())
	default:
		return nil, errors.NotFoundf("keys")
	}

	var doc sshHostKeysDoc
	err := coll.FindId(id).One(&doc)
	if err == mgo.ErrNotFound {
		return nil, errors.NotFoundf("keys")
	} else if err != nil {
		return nil, errors.Annotate(err, "key lookup failed")
	}

	return &doc, nil
}
