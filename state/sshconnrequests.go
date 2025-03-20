// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"strings"
	"time"

	"github.com/juju/errors"
	"github.com/juju/mgo/v3"
	"github.com/juju/mgo/v3/txn"

	"github.com/juju/juju/core/network"
)

type sshConnRequestDoc struct {
	DocId               string    `bson:"_id"`
	UnitName            string    `bson:"unit_name"`
	Expires             time.Time `bson:"expires"`
	Username            string    `bson:"username"`
	Password            string    `bson:"password"`
	ControllerAddresses []address `bson:"address"`
	UnitPort            int       `bson:"unit_port"`
	EphemeralPublicKey  []byte    `bson:"controller_public_key"`
}

// SSHConnRequest represents a ssh connection request.
type SSHConnRequest struct {
	*sshConnRequestDoc
}

// SSHConnRequestArg holds the necessary info to create a ssh connection requests.
type SSHConnRequestArg struct {
	TunnelID           string
	ModelUUID          string
	UnitName           string
	Expires            time.Time
	Username           string
	Password           string
	ControllerAddress  network.SpaceAddresses
	UnitPort           int
	EphemeralPublicKey []byte
}

// SSHConnRequestRemoveArg holds the necessary info to remove a ssh connection requests.
type SSHConnRequestRemoveArg struct {
	TunnelID  string
	ModelUUID string
	UnitName  string
}

func sshReqConnKeyID(unitName string, tunnelID string) string {
	return "unit" + "-" + unitName + "-" + "sshreqconn" + "-" + tunnelID
}

func newSSHConnRequestDoc(arg SSHConnRequestArg) (sshConnRequestDoc, error) {
	return sshConnRequestDoc{
		DocId:               ensureModelUUID(arg.ModelUUID, sshReqConnKeyID(arg.UnitName, arg.TunnelID)),
		UnitName:            arg.UnitName,
		UnitPort:            arg.UnitPort,
		Expires:             arg.Expires,
		Username:            arg.Username,
		Password:            arg.Password,
		ControllerAddresses: fromNetworkAddresses(arg.ControllerAddress, network.OriginProvider),
		EphemeralPublicKey:  arg.EphemeralPublicKey,
	}, nil
}

func insertSSHConnReqOp(arg SSHConnRequestArg) ([]txn.Op, error) {
	doc, err := newSSHConnRequestDoc(arg)
	if err != nil {
		return nil, err
	}
	cleanupOp := newCleanupAtOp(doc.Expires, cleanupExpiredSSHConnRequests, doc.DocId)
	return []txn.Op{
		{
			C:      sshConnRequestsC,
			Id:     doc.DocId,
			Assert: txn.DocMissing,
			Insert: doc,
		},
		cleanupOp,
	}, nil
}

// InsertSSHConnRequest inserts a new ssh connection request.
func (st *State) InsertSSHConnRequest(arg SSHConnRequestArg) error {
	txs, err := insertSSHConnReqOp(arg)
	if err != nil {
		return errors.Trace(err)
	}
	err = st.db().RunTransaction(txs)
	if err != nil {
		return errors.Trace(err)
	}
	return nil
}

func removeSSHConnRequestOps(arg SSHConnRequestRemoveArg) []txn.Op {
	return []txn.Op{{
		C:      sshConnRequestsC,
		Id:     ensureModelUUID(arg.ModelUUID, sshReqConnKeyID(arg.UnitName, arg.TunnelID)),
		Remove: true,
	}}
}

// RemoveSSHConnRequest removes a ssh connection request from the collection.
func (st *State) RemoveSSHConnRequest(arg SSHConnRequestRemoveArg) error {
	txs := removeSSHConnRequestOps(arg)
	err := st.db().RunTransaction(txs)
	if err != nil {
		return errors.Trace(err)
	}
	return nil
}

// GetSSHConnRequest returns a ssh connection request by its document ID.
func (st *State) GetSSHConnRequest(docID string) (SSHConnRequest, error) {
	vhkeys, closer := st.db().GetCollection(sshConnRequestsC)
	defer closer()

	doc := sshConnRequestDoc{}
	err := vhkeys.FindId(st.docID(docID)).One(&doc)
	if err == mgo.ErrNotFound {
		return SSHConnRequest{}, errors.NotFoundf("sshreqconn key %q", docID)
	}
	if err != nil {
		return SSHConnRequest{}, errors.Annotatef(err, "getting sshreqconn key %q", docID)
	}
	return SSHConnRequest{&doc}, nil
}

// WatchSSHConnRequest creates a watcher to get notified on documents being
// added/modified to the ssh request collection.
func (st *State) WatchSSHConnRequest(unitName string) StringsWatcher {
	return newCollectionWatcher(st, colWCfg{
		col: sshConnRequestsC,
		// -1 is for document deleted, 0 added, 1 modified.
		// So we are watching for added/modified.
		revnoThreshold: -1,
		filter: func(key any) bool {
			sKey, ok := key.(string)
			if !ok {
				return false
			}
			id := st.localID(sKey)
			if !strings.Contains(id, unitName) {
				return false
			}
			return true
		},
	})
}

// cleanupExpiredSSHConnReqRecord removes the expired ssh connection request record.
func (st *State) cleanupExpiredSSHConnReqRecord(docId string) error {
	txs := []txn.Op{{
		C:      sshConnRequestsC,
		Id:     docId,
		Remove: true,
	}}
	err := st.db().RunTransaction(txs)
	if err != nil {
		return errors.Trace(err)
	}
	return nil
}
