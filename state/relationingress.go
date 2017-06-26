// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"
	"net"

	"github.com/juju/errors"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"
)

// RelationIngress instances describe the ingress networks
// required for a cross model relation.
type RelationIngress interface {
	RelationKey() string
	CIDRS() []string
}

type relationIngressDoc struct {
	Id    string   `bson:"_id"`
	CIDRS []string `bson:"cidrs"`
}

type relationIngress struct {
	st  *State
	doc relationIngressDoc
}

// String returns r as a user-readable string.
func (r *relationIngress) String() string {
	return fmt.Sprintf("cidrs for relation %q", r.RelationKey())
}

// RelationKey is the key of the relation.
func (r *relationIngress) RelationKey() string {
	return r.st.localID(r.doc.Id)
}

// CIDRS returns the ingress networks for the relation.
func (r *relationIngress) CIDRS() []string {
	return r.doc.CIDRS
}

// RelationIngressNetworks instances provide access to relation ingress in state.
type RelationIngressNetworks interface {
	Save(relationKey string, cidrs []string) (RelationIngress, error)
}

type relationIngressNetworks struct {
	st *State
}

// RelationIngressNetworks creates a RelationIngressNetworks instance backed by a state.
func NewRelationIngressNetworks(st *State) *relationIngressNetworks {
	return &relationIngressNetworks{st: st}
}

// Save stores the specified ingress networks for the relation.
func (rin *relationIngressNetworks) Save(relationKey string, cidrs []string) (RelationIngress, error) {
	logger.Debugf("save ingress networks for %v: %v", relationKey, cidrs)
	for _, cidr := range cidrs {
		if _, _, err := net.ParseCIDR(cidr); err != nil {
			return nil, errors.NotValidf("CIDR %q", cidr)
		}
	}
	doc := relationIngressDoc{
		Id:    rin.st.docID(relationKey),
		CIDRS: cidrs,
	}
	buildTxn := func(int) ([]txn.Op, error) {
		model, err := rin.st.Model()
		if err != nil {
			return nil, errors.Annotate(err, "failed to load model")
		}
		if err := checkModelActive(rin.st); err != nil {
			return nil, errors.Trace(err)
		}
		if _, err := rin.st.KeyRelation(relationKey); err != nil {
			return nil, errors.Trace(err)
		}

		relationExistsAssert := txn.Op{
			C:      relationsC,
			Id:     doc.Id,
			Assert: txn.DocExists,
		}

		existing, err := rin.ingressNetworks(relationKey)
		if err != nil && !errors.IsNotFound(err) {
			return nil, errors.Trace(err)
		}
		var ops []txn.Op
		if err == nil {
			ops = []txn.Op{{
				C:      relationIngressC,
				Id:     existing.Id,
				Assert: txn.DocExists,
				Update: bson.D{
					{"$set", bson.D{{"cidrs", cidrs}}},
				},
			}, model.assertActiveOp(), relationExistsAssert}
		} else {
			doc.CIDRS = cidrs
			ops = []txn.Op{{
				C:      relationIngressC,
				Id:     doc.Id,
				Assert: txn.DocMissing,
				Insert: doc,
			}, model.assertActiveOp(), relationExistsAssert}
		}
		return ops, nil
	}
	if err := rin.st.db().Run(buildTxn); err != nil {
		return nil, errors.Annotate(err, "failed to create relation ingress networks")
	}

	return &relationIngress{
		st:  rin.st,
		doc: doc,
	}, nil
}

func (rin *relationIngressNetworks) ingressNetworks(relationKey string) (*relationIngressDoc, error) {
	coll, closer := rin.st.db().GetCollection(relationIngressC)
	defer closer()

	var doc relationIngressDoc
	err := coll.FindId(relationKey).One(&doc)
	if err == mgo.ErrNotFound {
		return nil, errors.NotFoundf("ingress networks for relation %v", relationKey)
	}
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &doc, nil
}

func removeRelationIngressNetworksOps(st *State, relationKey string) []txn.Op {
	ops := []txn.Op{{
		C:      relationIngressC,
		Id:     st.docID(relationKey),
		Remove: true,
	}}
	return ops
}
