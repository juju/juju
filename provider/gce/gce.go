// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package gce

import (
	"net/http"
	"net/mail"
	"time"

	"code.google.com/p/goauth2/oauth"
	"code.google.com/p/goauth2/oauth/jwt"
	"code.google.com/p/google-api-go-client/compute/v1"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/utils"

	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/network"
	"github.com/juju/juju/provider/common"
)

const (
	driverScopes = "https://www.googleapis.com/auth/compute " +
		"https://www.googleapis.com/auth/devstorage.full_control"

	tokenURL = "https://accounts.google.com/o/oauth2/token"

	authURL = "https://accounts.google.com/o/oauth2/auth"

	storageScratch    = "SCRATCH"
	storagePersistent = "PERSISTENT"

	statusDone         = "DONE"
	statusDown         = "DOWN"
	statusPending      = "PENDING"
	statusProvisioning = "PROVISIONING"
	statusRunning      = "RUNNING"
	statusStaging      = "STAGING"
	statusStopped      = "STOPPED"
	statusStopping     = "STOPPING"
	statusTerminated   = "TERMINATED"
	statusUp           = "UP"

	operationTimeout = 60 // seconds

	// minDiskSize is the minimum/default size (in megabytes) for GCE disks.
	//  TODO(ericsnow) Is there a minimum? What is the default?
	minDiskSize uint64 = 0
)

var (
	logger = loggo.GetLogger("juju.provider.gce")

	errNotImplemented = errors.NotImplementedf("gce provider")

	operationAttempts = utils.AttemptStrategy{
		Total: operationTimeout * time.Second,
		Delay: 10 * time.Second,
	}

	signedImageDataOnly = false
)

type gceAuth struct {
	clientID    string
	clientEmail string
	privateKey  []byte
}

func (ga gceAuth) validate() error {
	if ga.clientID == "" {
		return &config.InvalidConfigValue{Key: osEnvClientID}
	}
	if ga.clientEmail == "" {
		return &config.InvalidConfigValue{Key: osEnvClientEmail}
	} else if _, err := mail.ParseAddress(ga.clientEmail); err != nil {
		err = errors.Trace(err)
		return &config.InvalidConfigValue{osEnvClientEmail, ga.clientEmail, err}
	}
	if len(ga.privateKey) == 0 {
		return &config.InvalidConfigValue{Key: osEnvPrivateKey}
	}
	return nil
}

func (ga gceAuth) newTransport() (*oauth.Transport, error) {
	token, err := newToken(ga, driverScopes)
	if err != nil {
		return nil, errors.Trace(err)
	}

	transport := oauth.Transport{
		Config: &oauth.Config{
			ClientId: ga.clientID,
			Scope:    driverScopes,
			TokenURL: tokenURL,
			AuthURL:  authURL,
		},
		Token: token,
	}
	return &transport, nil
}

var newToken = func(auth gceAuth, scopes string) (*oauth.Token, error) {
	jtok := jwt.NewToken(auth.clientEmail, scopes, auth.privateKey)
	jtok.ClaimSet.Aud = tokenURL

	token, err := jtok.Assert(&http.Client{})
	if err != nil {
		msg := "retrieving auth token for %s"
		return nil, errors.Annotatef(err, msg, auth.clientEmail)
	}
	return token, nil
}

func (ga *gceAuth) newConnection() (*compute.Service, error) {
	transport, err := ga.newTransport()
	if err != nil {
		return nil, errors.Trace(err)
	}
	service, err := newService(transport)
	return service, errors.Trace(err)
}

var newService = func(transport *oauth.Transport) (*compute.Service, error) {
	return compute.New(transport.Client())
}

type gceConnection struct {
	*compute.Service

	region    string
	projectID string
}

func (gce *gceConnection) validate() error {
	if gce.projectID == "" {
		return &config.InvalidConfigValue{Key: osEnvProjectID}
	}
	return nil
}

func (gce *gceConnection) connect(auth gceAuth) error {
	if gce.Service != nil {
		return errors.New("connect() failed (already connected)")
	}

	service, err := auth.newConnection()
	if err != nil {
		return errors.Trace(err)
	}

	gce.Service = service
	return nil
}

func (gce *gceConnection) regionURL() string {
	// TODO(ericsnow) Finish this!
	return "https://www.googleapis.com/compute/v1/projects/my-project/zones/us-central1-a"
}

func (gce *gceConnection) waitOperation(operation *compute.Operation) error {
	opID := operation.ClientOperationId

	for a := operationAttempts.Start(); a.Next(); {
		var err error
		if operation.Status == statusDone {
			return nil
		}
		// TODO(ericsnow) Should projectID be an arg?
		call := gce.GlobalOperations.Get(gce.projectID, opID)
		operation, err = call.Do()
		if err != nil {
			return errors.Annotate(err, "waiting for operation to complete")
		}
	}
	if operation.Status == statusDone {
		return nil
	}

	msg := "timed out after %d seconds waiting for GCE operation to finish"
	return errors.Errorf(msg, operationTimeout)
}

func (gce *gceConnection) instance(zone, id string) (*compute.Instance, error) {
	call := gce.Instances.Get(gce.projectID, zone, id)
	inst, err := call.Do()
	return inst, errors.Trace(err)
}

func (gce *gceConnection) newInstance(inst *compute.Instance, zones []string) error {
	for _, zoneName := range zones {
		call := gce.Instances.Insert(
			gce.projectID,
			zoneName,
			inst,
		)
		operation, err := call.Do()
		if err != nil {
			// XXX Handle zone-is-full error.
			return errors.Annotate(err, "sending new instance request")
		}
		if err := gce.waitOperation(operation); err != nil {
			// TODO(ericsnow) Handle zone-is-full error here?
			return errors.Trace(err)
		}

		// Get the instance here.
		// TODO(ericsnow) Do we really need to get it?
		updated, err := gce.instance(zoneName, inst.Name)
		if err != nil {
			return errors.Trace(err)
		}
		*inst = *updated
		// Success!
		return nil
	}
	return errors.Errorf("not able to provision in any zone")
}

func (gce *gceConnection) instances(env environs.Environ) ([]*compute.Instance, error) {
	// env won't be nil.
	prefix := common.MachineFullName(env, "")

	// TODO(ericsnow) MaxResults arg defaults to 500... (call.MaxResults()).
	call := gce.Instances.AggregatedList(gce.projectID)
	call = call.Filter("name eq " + prefix + ".*")
	// TODO(ericsnow) If we can use multiple filters, filter on status here.
	raw, err := call.Do()
	if err != nil {
		return nil, errors.Trace(err)
	}

	var results []*compute.Instance
	for _, item := range raw.Items {
		for _, inst := range item.Instances {
			results = append(results, inst)
		}
	}
	return results, nil
}

func (gce *gceConnection) availabilityZones() ([]*compute.Zone, error) {
	//TODO(wwtizel3) support paging requests if we receive a truncated result.
	call := gce.Zones.List(gce.projectID)
	raw, err := call.Do()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return raw.Items, nil
}

func (gce *gceConnection) firewall(machineId string) (*compute.Firewall, error) {
	call := gce.Firewalls.Get(gce.projectID, machineId)
	firewall, err := call.Do()
	if err != nil {
		return nil, errors.Annotate(err, "while getting firewall from GCE")
	}
	return firewall, nil
}

func (gce *gceConnection) setFirewall(machineId string, firewall *compute.Firewall) error {
	var err error
	var operation *compute.Operation
	if firewall == nil {
		call := gce.Firewalls.Delete(gce.projectID, machineId)
		operation, err = call.Do()
		if err != nil {
			return errors.Trace(err)
		}
	} else if machineId == "" {
		call := gce.Firewalls.Insert(gce.projectID, firewall)
		operation, err = call.Do()
		if err != nil {
			return errors.Trace(err)
		}
	} else {
		call := gce.Firewalls.Update(gce.projectID, machineId, firewall)
		operation, err = call.Do()
		if err != nil {
			return errors.Trace(err)
		}
	}
	if err := gce.waitOperation(operation); err != nil {
		return errors.Trace(err)
	}
	return nil
}

func filterInstances(instances []*compute.Instance, statuses ...string) []*compute.Instance {
	// TODO(ericsnow) Filter in-place?
	// TODO(ericsnow) Also filter metadata (or tags)? While highly
	// unlikely (due to our choice of instance ID), it is possible that
	// the filter in gce.instances() results in a false positive.
	// An additional filter on the metadata would address that
	// possibility.
	var results []*compute.Instance
	for _, inst := range instances {
		if !checkInstStatus(inst, statuses...) {
			continue
		}
		results = append(results, inst)
	}
	return results
}

func checkInstStatus(inst *compute.Instance, statuses ...string) bool {
	for _, status := range statuses {
		if inst.Status == status {
			return true
		}
	}
	return false
}

func diskSpec(sizeReq *uint64, image string, boot bool) (*compute.AttachedDisk, uint64) {
	size := minDiskSize
	if sizeReq != nil && *sizeReq >= minDiskSize {
		size = *sizeReq
	}

	// TODO(ericsnow) what happens if there is not attached disk?
	disk := compute.AttachedDisk{
		// TODO(ericsnow) Set other fields too?
		Type: "SCRATCH", // Could be "PERSISTENT".
		Boot: boot,
		Mode: "READ_WRITE", // not needed?
		InitializeParams: &compute.AttachedDiskInitializeParams{
			// DiskName (defaults to instance name)
			DiskSizeGb: int64(common.MiBToGiB(size)),
			// DiskType (???)
			SourceImage: image, // needed?
		},
		// Interface (???)
		// DeviceName (persistent disk only)
		// Source (persistent disk only)
	}
	return &disk, size
}

// firewallSpec expands a port range set in to compute.FirewallAllowed
// and returns a compute.Firewall for the machineId.
func firewallSpec(machineId string, ps network.PortSet) *compute.Firewall {
	firewall := compute.Firewall{
		// Allowed is set below.
		// Description is not set.
		Name: machineId,
		// TODO(ericsnow) Does Network need to be set?
		// Network: "",
		// SourceRanges is not set.
		// SourceTags is not set.
		// TargetTags is not set.
	}

	for _, protocol := range ps.Protocols() {
		allowed := compute.FirewallAllowed{
			IPProtocol: protocol,
			Ports:      ps.PortStrings(protocol),
		}
		firewall.Allowed = append(firewall.Allowed, &allowed)
	}
	return &firewall
}
