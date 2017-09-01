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

// RelationNetworks instances describe the ingress or egress
// networks required for a cross model relation.
type RelationNetworks interface {
	Id() string
	RelationKey() string
	CIDRS() []string
}

type relationNetworksDoc struct {
	Id          string   `bson:"_id"`
	RelationKey string   `bson:"relation-key"`
	CIDRS       []string `bson:"cidrs"`
}

type relationNetworks struct {
	st  *State
	doc relationNetworksDoc
}

// Id returns the id for the relation networks entity.
func (r *relationNetworks) Id() string {
	return r.doc.Id
}

// String returns r as a user-readable string.
func (r *relationNetworks) String() string {
	return fmt.Sprintf("cidrs for relation %q", r.RelationKey())
}

// RelationKey is the key of the relation.
func (r *relationNetworks) RelationKey() string {
	return r.doc.RelationKey
}

// CIDRS returns the networks for the relation.
func (r *relationNetworks) CIDRS() []string {
	return r.doc.CIDRS
}

// RelationNetworker instances provide access to relation networks in state.
type RelationNetworker interface {
	Save(relationKey string, cidrs []string) (RelationNetworks, error)
	Networks(relationKey string) (RelationNetworks, error)
}

const (
	ingress = "ingress"
	egress  = "egress"
)

type relationNetworksState struct {
	st        *State
	direction string
}

// NewRelationIngressNetworks creates a RelationNetworks instance for ingress
// CIDRS backed by a state.
func NewRelationIngressNetworks(st *State) *relationNetworksState {
	return &relationNetworksState{st: st, direction: ingress}
}

// RelationEgressNetworks creates a RelationNetworks instance for egress
// CIDRS backed by a state.
func NewRelationEgressNetworks(st *State) *relationNetworksState {
	return &relationNetworksState{st: st, direction: egress}
}

func relationNetworkDocID(relationKey, direction string) string {
	return relationKey + ":" + direction
}

// Save stores the specified networks for the relation.
func (rin *relationNetworksState) Save(relationKey string, cidrs []string) (RelationNetworks, error) {
	logger.Debugf("save %v networks for %v: %v", rin.direction, relationKey, cidrs)
	for _, cidr := range cidrs {
		if _, _, err := net.ParseCIDR(cidr); err != nil {
			return nil, errors.NotValidf("CIDR %q", cidr)
		}
	}
	doc := relationNetworksDoc{
		Id:          rin.st.docID(relationNetworkDocID(relationKey, rin.direction)),
		RelationKey: relationKey,
		CIDRS:       cidrs,
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
			Id:     rin.st.docID(relationKey),
			Assert: txn.DocExists,
		}

		existing, err := rin.Networks(relationKey)
		if err != nil && !errors.IsNotFound(err) {
			return nil, errors.Trace(err)
		}
		var ops []txn.Op
		if err == nil {
			ops = []txn.Op{{
				C:      relationNetworksC,
				Id:     existing.Id(),
				Assert: txn.DocExists,
				Update: bson.D{
					{"$set", bson.D{{"cidrs", cidrs}}},
				},
			}, model.assertActiveOp(), relationExistsAssert}
		} else {
			doc.CIDRS = cidrs
			ops = []txn.Op{{
				C:      relationNetworksC,
				Id:     doc.Id,
				Assert: txn.DocMissing,
				Insert: doc,
			}, model.assertActiveOp(), relationExistsAssert}
		}
		return ops, nil
	}
	if err := rin.st.db().Run(buildTxn); err != nil {
		return nil, errors.Annotatef(err, "failed to create relation %v networks", rin.direction)
	}

	return &relationNetworks{
		st:  rin.st,
		doc: doc,
	}, nil
}

// Networks returns the networks for the specfied relation.
func (rin *relationNetworksState) Networks(relationKey string) (RelationNetworks, error) {
	coll, closer := rin.st.db().GetCollection(relationNetworksC)
	defer closer()

	var doc relationNetworksDoc
	err := coll.FindId(relationNetworkDocID(relationKey, rin.direction)).One(&doc)
	if err == mgo.ErrNotFound {
		return nil, errors.NotFoundf("%v networks for relation %v", rin.direction, relationKey)
	}
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &relationNetworks{
		st:  rin.st,
		doc: doc,
	}, nil
}

func removeRelationNetworksOps(st *State, relationKey string) []txn.Op {
	ops := []txn.Op{{
		C:      relationNetworksC,
		Id:     st.docID(relationNetworkDocID(relationKey, ingress)),
		Remove: true,
	}, {
		C:      relationNetworksC,
		Id:     st.docID(relationNetworkDocID(relationKey, egress)),
		Remove: true,
	}}
	return ops
}
