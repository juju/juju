// Copyright 2015 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package commands

import (
	"fmt"
	"strings"

	"github.com/juju/bundlechanges"
	"github.com/juju/errors"
	"gopkg.in/juju/charm.v6-unstable"
	"gopkg.in/yaml.v1"

	"github.com/juju/juju/api"
	"github.com/juju/juju/constraints"
	"github.com/juju/juju/environs/config"
)

// deploymentLogger is used to notify clients about the bundle deployment
// progress.
type deploymentLogger interface {
	// Infof formats and logs the given message.
	Infof(string, ...interface{})
}

// deployBundle deploys the given bundle data using the given API client and
// charm store client. The deployment is not transactional, and its progress is
// notified using the given deployment logger.
func deployBundle(data *charm.BundleData, client *api.Client, csclient *csClient, repoPath string, conf *config.Config, log deploymentLogger) error {
	if err := data.Verify(func(s string) error {
		_, err := constraints.Parse(s)
		return err
	}); err != nil {
		return errors.Annotate(err, "cannot deploy bundle")
	}

	// Retrieve bundle changes.
	changes := bundlechanges.FromData(data)
	h := &bundleHandler{
		changes:  make(map[string]bundlechanges.Change, len(changes)),
		results:  make(map[string]string, len(changes)),
		client:   client,
		csclient: csclient,
		repoPath: repoPath,
		conf:     conf,
		log:      log,
		data:     data,
	}
	for _, change := range changes {
		h.changes[change.Id()] = change
	}

	// Deploy the bundle.
	var err error
	for _, change := range changes {
		switch change := change.(type) {
		case *bundlechanges.AddCharmChange:
			err = h.addCharm(change.Id(), change.Params)
		case *bundlechanges.AddMachineChange:
			err = h.addMachine(change.Id(), change.Params)
		case *bundlechanges.AddRelationChange:
			err = h.addRelation(change.Id(), change.Params)
		case *bundlechanges.AddServiceChange:
			err = h.addService(change.Id(), change.Params)
		case *bundlechanges.AddUnitChange:
			err = h.addUnit(change.Id(), change.Params)
		case *bundlechanges.SetAnnotationsChange:
			err = h.setAnnotations(change.Id(), change.Params)
		default:
			return errors.Errorf("unknown change type: %T", change)
		}
		if err != nil {
			return errors.Annotate(err, "cannot deploy bundle")
		}
	}
	return nil
}

type bundleHandler struct {
	changes  map[string]bundlechanges.Change
	results  map[string]string
	client   *api.Client
	csclient *csClient
	repoPath string
	conf     *config.Config
	log      deploymentLogger
	data     *charm.BundleData
}

// addCharm adds a charm to the environment.
func (h *bundleHandler) addCharm(id string, p bundlechanges.AddCharmParams) error {
	url, repo, err := resolveCharmStoreEntityURL(p.Charm, h.csclient.params, h.repoPath, h.conf)
	if err != nil {
		return errors.Annotatef(err, "cannot resolve URL %q", p.Charm)
	}
	if url.Series == "bundle" {
		return errors.Errorf("expected charm URL, got bundle URL %q", p.Charm)
	}
	h.log.Infof("adding charm %s", url)
	url, err = addCharmViaAPI(h.client, url, repo, h.csclient)
	if err != nil {
		return errors.Annotatef(err, "cannot add charm %q", p.Charm)
	}
	// TODO frankban: the key here should really be the change id, but in the
	// current bundlechanges format the charm name is included in the service
	// change, not a placeholder pointing to the corresponding charm change, as
	// it should be instead.
	h.results["resolved-"+p.Charm] = url.String()
	return nil
}

// addService deploys or update a service with no units. Service options are
// also set or updated.
func (h *bundleHandler) addService(id string, p bundlechanges.AddServiceParams) error {
	status, err := h.client.Status(nil)
	if err != nil {
		return errors.Annotate(err, "cannot retrieve environment status")
	}
	svcStatus, svcExists := status.Services[p.Service]
	// TODO frankban: the charm should really be resolved using
	// resolve(p.Charm, h.results) at this point: see TODO in addCharm.
	ch := h.results["resolved-"+p.Charm]
	if svcExists {
		// The service is already deployed in the environment: check that its
		// charm is compatible with the one declared in the bundle. If it is,
		// reuse the existing service or upgrade to a specified revision.
		// Exit with an error otherwise.
		if svcStatus.Charm == ch {
			h.log.Infof("reusing service %s (charm: %s)", p.Service, ch)
		} else {
			if err := checkCompatibleCharms(ch, svcStatus.Charm); err != nil {
				return errors.Annotatef(err, "cannot upgrade charm for service %q", p.Service)
			}
			h.log.Infof("upgrading charm for existing service %s (from %s to %s)", p.Service, svcStatus.Charm, ch)
			if err := h.client.ServiceSetCharm(p.Service, ch, false); err != nil {
				return errors.Annotatef(err, "cannot upgrade charm for service %q", p.Service)
			}
		}
	} else {
		// The service does not exist in the environment.
		h.log.Infof("deploying service %s (charm: %s)", p.Service, ch)
		// TODO frankban: handle service constraints in the bundle changes.
		// Note that services are always added without units here, as the units
		// will be added later when handling unit changes in addUnit.
		numUnits, configYAML, cons, toMachineSpec := 0, "", constraints.Value{}, ""
		if err := h.client.ServiceDeploy(ch, p.Service, numUnits, configYAML, cons, toMachineSpec); err != nil {
			return errors.Annotatef(err, "cannot deploy service %q", p.Service)
		}
	}
	if len(p.Options) > 0 {
		h.log.Infof("configuring service %s", p.Service)
		if err := setServiceOptions(h.client, p.Service, p.Options); err != nil {
			return errors.Trace(err)
		}
	}
	h.results[id] = p.Service
	return nil
}

// addMachine creates a new top-level machine or container in the environment.
func (h *bundleHandler) addMachine(id string, p bundlechanges.AddMachineParams) error {
	// TODO frankban: implement this method.
	return nil
}

// addRelation creates a relationship between two services.
func (h *bundleHandler) addRelation(id string, p bundlechanges.AddRelationParams) error {
	ep1 := resolveRelation(p.Endpoint1, h.results)
	ep2 := resolveRelation(p.Endpoint2, h.results)
	// Check whether the given relation already exists.
	status, err := h.client.Status(nil)
	if err != nil {
		return errors.Annotate(err, "cannot retrieve environment status")
	}
	for _, r := range status.Relations {
		if len(r.Endpoints) != 2 {
			continue
		}
		if (r.Endpoints[0].String() == ep1 && r.Endpoints[1].String() == ep2) ||
			(r.Endpoints[1].String() == ep1 && r.Endpoints[0].String() == ep2) {
			h.log.Infof("%s and %s are already related", ep1, ep2)
			return nil
		}
	}
	h.log.Infof("relating %s and %s", ep1, ep2)
	if _, err := h.client.AddRelation(ep1, ep2); err != nil {
		return errors.Annotatef(err, "cannot add relation between %q and %q", ep1, ep2)
	}
	return nil
}

// addUnit adds a single unit to a service already present in the environment.
func (h *bundleHandler) addUnit(id string, p bundlechanges.AddUnitParams) error {
	// TODO frankban: implement this method.
	return nil
}

// setAnnotations sets annotations for a service or a machine.
func (h *bundleHandler) setAnnotations(id string, p bundlechanges.SetAnnotationsParams) error {
	// TODO frankban: implement this method.
	return nil
}

// checkCompatibleCharms ensures that the charms with the given ids are
// compatible, meaning an upgrade from one to the other is allowed.
func checkCompatibleCharms(id1, id2 string) error {
	ref1, err := charm.ParseReference(id1)
	if err != nil {
		return errors.Annotatef(err, "cannot parse charm URL %q", id1)
	}
	ref2, err := charm.ParseReference(id2)
	if err != nil {
		return errors.Annotatef(err, "cannot parse charm URL %q", id2)
	}
	if (ref1.Name != ref2.Name) || (ref1.User != ref2.User) {
		return errors.Errorf("charm %q is incompatible with charm %q", id1, id2)
	}
	return nil
}

// setServiceOptions changes the configuration for the given service.
func setServiceOptions(client *api.Client, service string, options map[string]interface{}) error {
	config, err := yaml.Marshal(map[string]map[string]interface{}{service: options})
	if err != nil {
		return errors.Annotatef(err, "cannot marshal options for service %q", service)
	}
	if err := client.ServiceSetYAML(service, string(config)); err != nil {
		return errors.Annotatef(err, "cannot set options for service %q", service)
	}
	return nil
}

// resolve returns the real entity name for the bundle entity (for instance a
// service or a machine) with the given placeholder id.
func resolve(placeholder string, results map[string]string) string {
	id := placeholder[1:]
	return results[id]
}

// resolveRelation returns the relation name resolving the included service
// placeholder.
func resolveRelation(e string, results map[string]string) string {
	parts := strings.SplitN(e, ":", 2)
	service := resolve(parts[0], results)
	if len(parts) == 1 {
		return service
	}
	return fmt.Sprintf("%s:%s", service, parts[1])
}
