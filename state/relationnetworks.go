// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"
	"net"

	"github.com/juju/errors"
	"github.com/juju/mgo/v3"
	"github.com/juju/mgo/v3/bson"
	"github.com/juju/mgo/v3/txn"
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
	Save(relationKey string, adminOverride bool, cidrs []string) (RelationNetworks, error)
	Networks(relationKey string) (RelationNetworks, error)
}

// RelationNetworkDirection represents a type that describes the direction of
// the network, either ingress or egress.
type RelationNetworkDirection string

func (r RelationNetworkDirection) String() string {
	return string(r)
}

const (
	// IngressDirection for a ingress relation network direction
	IngressDirection RelationNetworkDirection = "ingress"
	// EgressDirection for a egress relation network direction
	EgressDirection RelationNetworkDirection = "egress"
)

const (
	// relationNetworkDefault is a default, non-override network.
	relationNetworkDefault = relationNetworkType("default")

	// relationNetworkAdmin is a network that has been overridden by an admin.
	relationNetworkAdmin = relationNetworkType("override")
)

type relationNetworkType string

type rootRelationNetworksState struct {
	st *State
}

// NewRelationNetworks creates a root RelationNetworks without a direction, so
// accessing RelationNetworks is possible agnostically.
func NewRelationNetworks(st *State) *rootRelationNetworksState {
	return &rootRelationNetworksState{st: st}
}

// AllRelationNetworks returns all the relation networks for the model.
func (rin *rootRelationNetworksState) AllRelationNetworks() ([]RelationNetworks, error) {
	relationNetworksCollection, closer := rin.st.db().GetCollection(relationNetworksC)
	defer closer()

	var docs []relationNetworksDoc
	if err := relationNetworksCollection.Find(nil).All(&docs); err != nil {
		return nil, errors.Annotatef(err, "cannot get all relation networks")
	}
	entities := make([]RelationNetworks, len(docs))
	for i, doc := range docs {
		id, err := rin.st.strictLocalID(doc.Id)
		if err != nil {
			return nil, errors.Trace(err)
		}
		entities[i] = &relationNetworks{
			st: rin.st,
			doc: relationNetworksDoc{
				Id:          id,
				RelationKey: doc.RelationKey,
				CIDRS:       doc.CIDRS,
			},
		}
	}
	return entities, nil
}

type relationNetworksState struct {
	st        *State
	direction string
}

// NewRelationIngressNetworks creates a RelationNetworks instance for ingress
// CIDRS backed by a state.
func NewRelationIngressNetworks(st *State) *relationNetworksState {
	return &relationNetworksState{st: st, direction: IngressDirection.String()}
}

// NewRelationEgressNetworks creates a RelationNetworks instance for egress
// CIDRS backed by a state.
func NewRelationEgressNetworks(st *State) *relationNetworksState {
	return &relationNetworksState{st: st, direction: EgressDirection.String()}
}

func relationNetworkDocID(relationKey, direction string, label relationNetworkType) string {
	return fmt.Sprintf("%v:%v:%v", relationKey, direction, label)
}

// Save stores the specified networks for the relation.
func (rin *relationNetworksState) Save(relationKey string, adminOverride bool, cidrs []string) (RelationNetworks, error) {
	logger.Debugf("save %v networks for %v: %v", rin.direction, relationKey, cidrs)
	for _, cidr := range cidrs {
		if _, _, err := net.ParseCIDR(cidr); err != nil {
			return nil, errors.NotValidf("CIDR %q", cidr)
		}
	}
	label := relationNetworkDefault
	if adminOverride {
		label = relationNetworkAdmin
	}
	doc := relationNetworksDoc{
		Id:          rin.st.docID(relationNetworkDocID(relationKey, rin.direction, label)),
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
		if err != nil && !errors.Is(err, errors.NotFound) {
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

// Networks returns the networks for the specified relation.
func (rin *relationNetworksState) Networks(relationKey string) (RelationNetworks, error) {
	coll, closer := rin.st.db().GetCollection(relationNetworksC)
	defer closer()

	var doc relationNetworksDoc
	err := coll.FindId(relationNetworkDocID(relationKey, rin.direction, relationNetworkAdmin)).One(&doc)
	if err == mgo.ErrNotFound {
		err = coll.FindId(relationNetworkDocID(relationKey, rin.direction, relationNetworkDefault)).One(&doc)
	}
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
		Id:     st.docID(relationNetworkDocID(relationKey, IngressDirection.String(), relationNetworkAdmin)),
		Remove: true,
	}, {
		C:      relationNetworksC,
		Id:     st.docID(relationNetworkDocID(relationKey, IngressDirection.String(), relationNetworkDefault)),
		Remove: true,
	}, {
		C:      relationNetworksC,
		Id:     st.docID(relationNetworkDocID(relationKey, EgressDirection.String(), relationNetworkAdmin)),
		Remove: true,
	}, {
		C:      relationNetworksC,
		Id:     st.docID(relationNetworkDocID(relationKey, EgressDirection.String(), relationNetworkDefault)),
		Remove: true,
	}}
	return ops
}
