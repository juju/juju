// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package gce

import (
	"net/http"
	"net/mail"
	"path"
	"time"

	"code.google.com/p/goauth2/oauth"
	"code.google.com/p/goauth2/oauth/jwt"
	"code.google.com/p/google-api-go-client/compute/v1"
	"github.com/juju/errors"
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

	partialMachineType = "zones/%s/machineTypes/%s"

	diskTypeScratch    = "SCRATCH"
	diskTypePersistent = "PERSISTENT"
	diskModeRW         = "READ_WRITE"
	diskModeRO         = "READ_ONLY"

	networkDefaultName       = "default"
	networkPathRoot          = "global/networks/"
	networkAccessOneToOneNAT = "ONE_TO_ONE_NAT"

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

	// minDiskSize is the minimum/default size (in megabytes) for GCE
	// disks. GCE does not currently have a minimum disk size.
	minDiskSize int64 = 0
)

var (
	operationAttempts = utils.AttemptStrategy{
		Total: operationTimeout * time.Second,
		Delay: 10 * time.Second,
	}
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
	if gce.region == "" {
		return &config.InvalidConfigValue{Key: osEnvRegion}
	}
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

func (gce *gceConnection) verifyCredentials() error {
	call := gce.Projects.Get(gce.projectID)
	if _, err := call.Do(); err != nil {
		return errors.Trace(err)
	}
	return nil
}

type operationDoer interface {
	Do() (*compute.Operation, error)
}

func (gce *gceConnection) waitOperation(operation *compute.Operation) error {
	opName := operation.Name

	logger.Infof("GCE operation %q, waiting...", opName)
	for a := operationAttempts.Start(); a.Next(); {
		var err error
		if operation.Status == statusDone {
			break
		}

		var call operationDoer
		if operation.Zone != "" {
			call = gce.ZoneOperations.Get(gce.projectID, path.Base(operation.Zone), opName)
		} else if operation.Region != "" {
			call = gce.RegionOperations.Get(gce.projectID, path.Base(operation.Region), opName)
		} else {
			call = gce.GlobalOperations.Get(gce.projectID, opName)
		}
		operation, err = call.Do()
		if err != nil {
			return errors.Annotate(err, "waiting for operation to complete")
		}
	}
	if operation.Status != statusDone {
		msg := "timed out after %d seconds waiting for GCE operation to finish"
		return errors.Errorf(msg, operationTimeout)
	}
	if operation.Error != nil {
		for _, err := range operation.Error.Errors {
			logger.Errorf("GCE operation failed: (%s) %s", err.Code, err.Message)
		}
		return errors.Errorf("GCE operation %q failed", opName)
	}

	logger.Infof("GCE operation %q finished", opName)
	return nil
}

func (gce *gceConnection) instance(zone, id string) (*compute.Instance, error) {
	call := gce.Instances.Get(gce.projectID, zone, id)
	inst, err := call.Do()
	return inst, errors.Trace(err)
}

func (gce *gceConnection) addInstance(inst *compute.Instance, machineType string, zones []string) error {
	for _, zoneName := range zones {
		inst.MachineType = resolveMachineType(zoneName, machineType)
		call := gce.Instances.Insert(
			gce.projectID,
			zoneName,
			inst,
		)
		operation, err := call.Do()
		if err != nil {
			// We are guaranteed the insert failed at the point.
			return errors.Annotate(err, "sending new instance request")
		}
		waitErr := gce.waitOperation(operation)

		// Check if the instance was created.
		realized, err := gce.instance(zoneName, inst.Name)
		if err != nil {
			if waitErr == nil {
				return errors.Trace(err)
			}
			// Try the next zone.
			logger.Errorf("failed to get new instance in zone %q: %v", zoneName, waitErr)
			continue
		}

		// Success!
		*inst = *realized
		return nil
	}
	return errors.Errorf("not able to provision in any zone")
}

func (gce *gceConnection) instances(env environs.Environ) ([]*compute.Instance, error) {
	// env won't be nil.
	prefix := common.MachineFullName(env, "")

	call := gce.Instances.AggregatedList(gce.projectID)
	call = call.Filter("name eq " + prefix + ".*")

	// TODO(ericsnow) Add a timeout?
	var results []*compute.Instance
	for {
		raw, err := call.Do()
		if err != nil {
			return results, errors.Trace(err)
		}

		for _, item := range raw.Items {
			results = append(results, item.Instances...)
		}

		if raw.NextPageToken == "" {
			break
		}
		call = call.PageToken(raw.NextPageToken)
	}

	return results, nil
}

func (gce *gceConnection) availabilityZones(region string) ([]*compute.Zone, error) {
	call := gce.Zones.List(gce.projectID)
	if region != "" {
		call = call.Filter("name eq " + region + "-.*")
	}
	// TODO(ericsnow) Add a timeout?
	var results []*compute.Zone
	for {
		raw, err := call.Do()
		if err != nil {
			return nil, errors.Trace(err)
		}

		results = append(results, raw.Items...)

		if raw.NextPageToken == "" {
			break
		}
		call = call.PageToken(raw.NextPageToken)
	}

	return results, nil
}

func (gce *gceConnection) removeInstance(id, zone string) error {
	call := gce.Instances.Delete(gce.projectID, zone, id)
	operation, err := call.Do()
	if err != nil {
		return errors.Trace(err)
	}
	if err := gce.waitOperation(operation); err != nil {
		return errors.Trace(err)
	}

	// Clean up the instance's root disk.
	// TODO(ericsnow) Set the instance's disk auto-delete instead?
	err = gce.removeDisk(id, zone)
	return errors.Trace(err)
}

func (gce *gceConnection) removeInstances(env environs.Environ, ids ...string) error {
	if len(ids) == 0 {
		return nil
	}

	instances, err := gce.instances(env)
	if err != nil {
		return errors.Annotatef(err, "while removing instances %v", ids)
	}

	var failed []string
	for _, instID := range ids {
		for _, inst := range instances {
			if inst.Name == instID {
				if err := gce.removeInstance(instID, path.Base(inst.Zone)); err != nil {
					failed = append(failed, instID)
					logger.Errorf("while removing instance %q: %v", instID, err)
				}
				break
			}
		}
	}
	if len(failed) != 0 {
		return errors.Errorf("some instance removals failed: %v", failed)
	}
	return nil
}

func (gce *gceConnection) removeDisk(id, zone string) error {
	call := gce.Disks.Delete(gce.projectID, zone, id)
	operation, err := call.Do()
	if err != nil {
		return errors.Trace(err)
	}
	err = gce.waitOperation(operation)
	return errors.Trace(err)
}

func (gce *gceConnection) firewall(name string) (*compute.Firewall, error) {
	call := gce.Firewalls.Get(gce.projectID, name)
	firewall, err := call.Do()
	if err != nil {
		return nil, errors.Annotate(err, "while getting firewall from GCE")
	}
	return firewall, nil
}

func (gce *gceConnection) setFirewall(name string, firewall *compute.Firewall) error {
	var err error
	var operation *compute.Operation
	if firewall == nil {
		call := gce.Firewalls.Delete(gce.projectID, name)
		operation, err = call.Do()
		if err != nil {
			return errors.Trace(err)
		}
	} else if name == "" {
		call := gce.Firewalls.Insert(gce.projectID, firewall)
		operation, err = call.Do()
		if err != nil {
			return errors.Trace(err)
		}
	} else {
		call := gce.Firewalls.Update(gce.projectID, name, firewall)
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

type diskSpec struct {
	// sizeHint is the requested disk size in Gigabytes.
	sizeHint int64
	imageURL string
	boot     bool
	scratch  bool
	readonly bool
}

func (ds *diskSpec) size() int64 {
	size := minDiskSize
	if ds.sizeHint >= minDiskSize {
		size = ds.sizeHint
	}
	return size
}

func (ds *diskSpec) newAttached() *compute.AttachedDisk {
	diskType := diskTypePersistent // The default.
	if ds.scratch {
		diskType = diskTypeScratch
	}
	mode := diskModeRW // The default.
	if ds.readonly {
		mode = diskModeRO
	}

	disk := compute.AttachedDisk{
		Type: diskType,
		Boot: ds.boot,
		Mode: mode,
		InitializeParams: &compute.AttachedDiskInitializeParams{
			// DiskName (defaults to instance name)
			DiskSizeGb: ds.size(),
			// DiskType (defaults to pd-standard, pd-ssd, local-ssd)
			SourceImage: ds.imageURL,
		},
		// Interface (defaults to SCSI)
		// DeviceName (GCE sets this, persistent disk only)
	}
	return &disk
}

type networkSpec struct {
	name string
	// TODO(ericsnow) support a CIDR for internal IP addr range?
}

func (ns *networkSpec) path() string {
	name := ns.name
	if name == "" {
		name = networkDefaultName
	}
	return networkPathRoot + name
}

func (ns *networkSpec) newInterface(name string) *compute.NetworkInterface {
	var access []*compute.AccessConfig
	if name != "" {
		// This interface has an internet connection.
		access = append(access, &compute.AccessConfig{
			Name: name,
			Type: networkAccessOneToOneNAT, // the default
			// NatIP (only set if using a reserved public IP)
		})
		// TODO(ericsnow) Will we need to support more access configs?
	}
	return &compute.NetworkInterface{
		Network:       ns.path(),
		AccessConfigs: access,
	}
}

// firewallSpec expands a port range set in to compute.FirewallAllowed
// and returns a compute.Firewall for the provided name.
func firewallSpec(name string, ps network.PortSet) *compute.Firewall {
	firewall := compute.Firewall{
		// Allowed is set below.
		// Description is not set.
		Name: name,
		// Network: (defaults to global)
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

func packMetadata(data map[string]string) *compute.Metadata {
	var items []*compute.MetadataItems
	for key, value := range data {
		item := compute.MetadataItems{
			Key:   key,
			Value: value,
		}
		items = append(items, &item)
	}
	return &compute.Metadata{Items: items}
}

func unpackMetadata(data *compute.Metadata) map[string]string {
	if data == nil {
		return nil
	}

	result := make(map[string]string)
	for _, item := range data.Items {
		result[item.Key] = item.Value
	}
	return result
}
