// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"net"

	"github.com/juju/errors"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"
)

// FirewallRule instances describe the ingress networks
// whitelist/blacklist for a given well known service.
// Primarily, whitelisting allowed ingress subnets is the
// main use case. However, blacklisting subnets is useful
// to allow restrictions placed on incoming traffic for
// cross model relations, where the source of traffic is
// requested from the consuming side.
// WellKnownService is either a well known internet service
// (currently just SSH) or a Juju defined value.
// Supported values are:
// - ssh
// - juju-controller
// - juju-application-offer
type FirewallRule struct {
	// WellKnownService is the known service for the firewall rules entity.
	WellKnownService WellKnownServiceType

	// BlacklistCIDR is the blacklist CIDRs for the rule.
	BlacklistCIDRs []string

	// WhitelistCIDRS is the whitelist CIDRs for the rule.
	WhitelistCIDRs []string
}

type firewallRulesDoc struct {
	Id               string   `bson:"_id"`
	WellKnownService string   `bson:"known-service"`
	BlacklistCIDRS   []string `bson:"blacklist-cidrs"`
	WhitelistCIDRS   []string `bson:"whitelist-cidrs"`
}

func (r *firewallRulesDoc) toRule() *FirewallRule {
	return &FirewallRule{
		WellKnownService: WellKnownServiceType(r.WellKnownService),
		WhitelistCIDRs:   r.WhitelistCIDRS,
		BlacklistCIDRs:   r.BlacklistCIDRS,
	}
}

// FirewallRuler instances provide access to firewall rules in state.
type FirewallRuler interface {
	Save(service WellKnownServiceType, whiteListCidrs, blackListCidrs []string) (FirewallRule, error)
	Rule(service WellKnownServiceType) (FirewallRule, error)
	AllRules() ([]FirewallRule, error)
}

const (
	// SSHRule is a rule for SSH connections.
	SSHRule = WellKnownServiceType("ssh")

	// JujuControllerRule is a rule for connections to the Juju controller.
	JujuControllerRule = WellKnownServiceType("juju-controller")

	// JujuApplicationOfferRule is a rule for connections to a Juju offer.
	JujuApplicationOfferRule = WellKnownServiceType("juju-application-offer")
)

// WellKnownServiceType defines a service for which firewall rules may be applied.
type WellKnownServiceType string

func (v WellKnownServiceType) validate() error {
	switch v {
	case SSHRule, JujuControllerRule, JujuApplicationOfferRule:
		return nil
	}
	return errors.NotValidf("well known service type %q", v)
}

type firewallRulesState struct {
	st *State
}

// NewFirewallRules creates a FirewallRule instance backed by a state.
func NewFirewallRules(st *State) *firewallRulesState {
	return &firewallRulesState{st: st}
}

// Save stores the specified firewall rules.
func (fw *firewallRulesState) Save(service WellKnownServiceType, whiteListCidrs, blackListCidrs []string) (*FirewallRule, error) {
	if err := service.validate(); err != nil {
		return nil, errors.Trace(err)
	}
	for _, cidr := range append(whiteListCidrs, blackListCidrs...) {
		if _, _, err := net.ParseCIDR(cidr); err != nil {
			return nil, errors.NotValidf("CIDR %q", cidr)
		}
	}
	doc := firewallRulesDoc{
		Id:               string(service),
		WellKnownService: string(service),
		WhitelistCIDRS:   whiteListCidrs,
		BlacklistCIDRS:   blackListCidrs,
	}
	buildTxn := func(int) ([]txn.Op, error) {
		model, err := fw.st.Model()
		if err != nil {
			return nil, errors.Annotate(err, "failed to load model")
		}
		if err := checkModelActive(fw.st); err != nil {
			return nil, errors.Trace(err)
		}

		_, err = fw.Rule(service)
		if err != nil && !errors.IsNotFound(err) {
			return nil, errors.Trace(err)
		}
		var ops []txn.Op
		if err == nil {
			ops = []txn.Op{{
				C:      firewallRulesC,
				Id:     string(service),
				Assert: txn.DocExists,
				Update: bson.D{
					{"$set", bson.D{{"whitelist-cidrs", whiteListCidrs}}},
					{"$set", bson.D{{"blacklist-cidrs", blackListCidrs}}},
				},
			}, model.assertActiveOp()}
		} else {
			doc.WhitelistCIDRS = whiteListCidrs
			doc.BlacklistCIDRS = blackListCidrs
			ops = []txn.Op{{
				C:      firewallRulesC,
				Id:     doc.Id,
				Assert: txn.DocMissing,
				Insert: doc,
			}, model.assertActiveOp()}
		}
		return ops, nil
	}
	if err := fw.st.db().Run(buildTxn); err != nil {
		return nil, errors.Annotate(err, "failed to create firewall rules")
	}

	return doc.toRule(), nil
}

// Rule returns the firewall rule for the specified service.
func (fw *firewallRulesState) Rule(service WellKnownServiceType) (*FirewallRule, error) {
	coll, closer := fw.st.db().GetCollection(firewallRulesC)
	defer closer()

	var doc firewallRulesDoc
	err := coll.FindId(string(service)).One(&doc)
	if err == mgo.ErrNotFound {
		return nil, errors.NotFoundf("firewall rules for service %v", service)
	}
	if err != nil {
		return nil, errors.Trace(err)
	}
	return doc.toRule(), nil
}

// AllRules returns all the firewall rules.
func (fw *firewallRulesState) AllRules() ([]*FirewallRule, error) {
	coll, closer := fw.st.db().GetCollection(firewallRulesC)
	defer closer()

	var docs []firewallRulesDoc
	err := coll.Find(nil).All(&docs)
	if err != nil {
		return nil, errors.Trace(err)
	}
	result := make([]*FirewallRule, len(docs))
	for i, doc := range docs {
		result[i] = doc.toRule()
	}
	return result, nil
}
