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
	MachineId           string    `bson:"machine_id"`
	Expires             time.Time `bson:"expires"`
	Username            string    `bson:"username"`
	Password            string    `bson:"password"`
	ControllerAddresses []address `bson:"address"`
	UnitPort            int       `bson:"unit_port"`
	EphemeralPublicKey  []byte    `bson:"controller_public_key"`
}

// SSHConnRequest represents a ssh connection request. This is persisted statefully such
// that it may be watched by the machine agent to know when to perform a reverse SSH call
// back to the controller and contains the details to perform such connection.
type SSHConnRequest struct {
	*sshConnRequestDoc
}

// SSHConnRequestArg holds the necessary info to create a ssh connection requests.
type SSHConnRequestArg struct {
	// TunnelID holds the ID to associate the connection back to the incoming client.
	TunnelID string
	// ModelUUID holds the model UUID.
	ModelUUID string
	// MachineId holds the ID of the machine to which this request is addressed.
	MachineId string
	// Expires holds the time when the request will expire.
	Expires time.Time
	// Username holds the username to be used by the machine agent when opening an ssh connection to the controller.
	Username string
	// Password holds the password to be used by the machine agent when opening an ssh connection to the controller.
	Password string
	// ControllerAddress holds the IP of the controller unit to be used by the machine agent when opening an ssh connection.
	ControllerAddress network.SpaceAddresses
	// UnitPort holds the unit port, to be used in remote forwarding.
	UnitPort int
	// EphemeralPublicKey holds the public key to be added to machine's authorized_keys for the lifetime of the ssh connection.
	EphemeralPublicKey []byte
}

// SSHConnRequestRemoveArg holds the necessary info to remove a ssh connection requests.
type SSHConnRequestRemoveArg struct {
	TunnelID  string
	ModelUUID string
	MachineId string
}

func sshReqConnKeyID(machineId string, tunnelID string) string {
	return "machine" + "-" + machineId + "-" + "sshreqconn" + "-" + tunnelID
}

func newSSHConnRequestDoc(arg SSHConnRequestArg) (sshConnRequestDoc, error) {
	return sshConnRequestDoc{
		DocId:               ensureModelUUID(arg.ModelUUID, sshReqConnKeyID(arg.MachineId, arg.TunnelID)),
		MachineId:           arg.MachineId,
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
		Id:     ensureModelUUID(arg.ModelUUID, sshReqConnKeyID(arg.MachineId, arg.TunnelID)),
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
// added/modified to the ssh request collection. The machine agent will be the
// consumer of this, determining when to initate reverse SSH.
func (st *State) WatchSSHConnRequest(machineId string) StringsWatcher {
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

			return strings.HasPrefix(id, "machine-"+machineId)
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
