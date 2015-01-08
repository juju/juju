// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package gceapi

import (
	"fmt"
	"net/http"
	"net/mail"
	"path"
	"strings"
	"time"

	"code.google.com/p/goauth2/oauth"
	"code.google.com/p/goauth2/oauth/jwt"
	"code.google.com/p/google-api-go-client/compute/v1"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/utils"

	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/network"
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

	StatusDone         = "DONE"
	StatusDown         = "DOWN"
	StatusPending      = "PENDING"
	StatusProvisioning = "PROVISIONING"
	StatusRunning      = "RUNNING"
	StatusStaging      = "STAGING"
	StatusStopped      = "STOPPED"
	StatusStopping     = "STOPPING"
	StatusTerminated   = "TERMINATED"
	StatusUp           = "UP"

	// MinDiskSize is the minimum/default size (in megabytes) for GCE
	// disks. GCE does not currently have a minimum disk size.
	MinDiskSizeGB int64 = 0

	// These are not GCE-official environment variable names.
	OSEnvPrivateKey    = "GCE_PRIVATE_KEY"
	OSEnvClientID      = "GCE_CLIENT_ID"
	OSEnvClientEmail   = "GCE_CLIENT_EMAIL"
	OSEnvRegion        = "GCE_REGION"
	OSEnvProjectID     = "GCE_PROJECT_ID"
	OSEnvImageEndpoint = "GCE_IMAGE_URL"
)

var (
	logger = loggo.GetLogger("juju.provider.gce.gceapi")

	// TODO(ericsnow) Tune the timeouts and delays.
	attemptsLong = utils.AttemptStrategy{
		Total: 300 * time.Second, // 5 minutes
		Delay: 2 * time.Second,
	}
	attemptsShort = utils.AttemptStrategy{
		Total: 60 * time.Second,
		Delay: 1 * time.Second,
	}
)

type Auth struct {
	ClientID    string
	ClientEmail string
	PrivateKey  []byte
}

func (ga Auth) Validate() error {
	if ga.ClientID == "" {
		return &config.InvalidConfigValue{Key: OSEnvClientID}
	}
	if ga.ClientEmail == "" {
		return &config.InvalidConfigValue{Key: OSEnvClientEmail}
	} else if _, err := mail.ParseAddress(ga.ClientEmail); err != nil {
		err = errors.Trace(err)
		return &config.InvalidConfigValue{OSEnvClientEmail, ga.ClientEmail, err}
	}
	if len(ga.PrivateKey) == 0 {
		return &config.InvalidConfigValue{Key: OSEnvPrivateKey}
	}
	return nil
}

func (ga Auth) newTransport() (*oauth.Transport, error) {
	token, err := newToken(ga, driverScopes)
	if err != nil {
		return nil, errors.Trace(err)
	}

	transport := oauth.Transport{
		Config: &oauth.Config{
			ClientId: ga.ClientID,
			Scope:    driverScopes,
			TokenURL: tokenURL,
			AuthURL:  authURL,
		},
		Token: token,
	}
	return &transport, nil
}

var newToken = func(auth Auth, scopes string) (*oauth.Token, error) {
	jtok := jwt.NewToken(auth.ClientEmail, scopes, auth.PrivateKey)
	jtok.ClaimSet.Aud = tokenURL

	token, err := jtok.Assert(&http.Client{})
	if err != nil {
		msg := "retrieving auth token for %s"
		return nil, errors.Annotatef(err, msg, auth.ClientEmail)
	}
	return token, nil
}

func (ga Auth) newConnection() (*compute.Service, error) {
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

type Connection struct {
	raw *compute.Service

	Region    string
	ProjectID string
}

func (gc Connection) Validate() error {
	if gc.Region == "" {
		return &config.InvalidConfigValue{Key: OSEnvRegion}
	}
	if gc.ProjectID == "" {
		return &config.InvalidConfigValue{Key: OSEnvProjectID}
	}
	return nil
}

func (gc *Connection) Connect(auth Auth) error {
	if gc.raw != nil {
		return errors.New("connect() failed (already connected)")
	}

	service, err := auth.newConnection()
	if err != nil {
		return errors.Trace(err)
	}

	gc.raw = service
	return nil
}

func (gc Connection) VerifyCredentials() error {
	call := gc.raw.Projects.Get(gc.ProjectID)
	if _, err := call.Do(); err != nil {
		return errors.Trace(err)
	}
	return nil
}

type operationDoer interface {
	// Do starts some operation and returns a description of it. If an
	// error is returned then the operation was not initiated.
	Do() (*compute.Operation, error)
}

func (gce *Connection) checkOperation(op *compute.Operation) (*compute.Operation, error) {
	var call operationDoer
	if op.Zone != "" {
		zone := zoneName(op)
		call = gce.raw.ZoneOperations.Get(gce.ProjectID, zone, op.Name)
	} else if op.Region != "" {
		region := path.Base(op.Region)
		call = gce.raw.RegionOperations.Get(gce.ProjectID, region, op.Name)
	} else {
		call = gce.raw.GlobalOperations.Get(gce.ProjectID, op.Name)
	}

	updated, err := call.Do()
	if err != nil {
		return nil, errors.Annotatef(err, "request for GCE operation %q failed", op.Name)
	}
	return updated, nil
}

func (gce *Connection) waitOperation(op *compute.Operation, attempts utils.AttemptStrategy) error {
	started := time.Now()
	logger.Infof("GCE operation %q, waiting...", op.Name)
	for a := attempts.Start(); a.Next(); {
		if op.Status == StatusDone {
			break
		}

		var err error
		op, err = gce.checkOperation(op)
		if err != nil {
			return errors.Trace(err)
		}
	}
	if op.Status != StatusDone {
		msg := "GCE operation %q failed: timed out after %d seconds"
		return errors.Errorf(msg, op.Name, time.Now().Sub(started)/time.Second)
	}
	if op.Error != nil {
		for _, err := range op.Error.Errors {
			logger.Errorf("GCE operation error: (%s) %s", err.Code, err.Message)
		}
		return errors.Errorf("GCE operation %q failed", op.Name)
	}

	logger.Infof("GCE operation %q finished", op.Name)
	return nil
}

func (gce *Connection) instance(zone, id string) (*compute.Instance, error) {
	call := gce.raw.Instances.Get(gce.ProjectID, zone, id)
	inst, err := call.Do()
	return inst, errors.Trace(err)
}

func (gce *Connection) addInstance(inst *compute.Instance, machineType string, zones []string) error {
	for _, zoneName := range zones {
		inst.MachineType = resolveMachineType(zoneName, machineType)
		call := gce.raw.Instances.Insert(
			gce.ProjectID,
			zoneName,
			inst,
		)
		operation, err := call.Do()
		if err != nil {
			// We are guaranteed the insert failed at the point.
			return errors.Annotate(err, "sending new instance request")
		}
		waitErr := gce.waitOperation(operation, attemptsLong)

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

func (gce *Connection) Instances(prefix string, statuses ...string) ([]Instance, error) {
	call := gce.raw.Instances.AggregatedList(gce.ProjectID)
	call = call.Filter("name eq " + prefix + ".*")

	// TODO(ericsnow) Add a timeout?
	var results []Instance
	for {
		raw, err := call.Do()
		if err != nil {
			return results, errors.Trace(err)
		}

		for _, item := range raw.Items {
			for _, raw := range item.Instances {
				inst := newInstance(raw)
				results = append(results, *inst)
			}
		}

		if raw.NextPageToken == "" {
			break
		}
		call = call.PageToken(raw.NextPageToken)
	}

	return filterInstances(results, statuses...), nil
}

func (gce *Connection) AvailabilityZones(region string) ([]AvailabilityZone, error) {
	call := gce.raw.Zones.List(gce.ProjectID)
	if region != "" {
		call = call.Filter("name eq " + region + "-.*")
	}
	// TODO(ericsnow) Add a timeout?
	var results []AvailabilityZone
	for {
		rawResult, err := call.Do()
		if err != nil {
			return nil, errors.Trace(err)
		}

		for _, raw := range rawResult.Items {
			results = append(results, AvailabilityZone{raw})
		}

		if rawResult.NextPageToken == "" {
			break
		}
		call = call.PageToken(rawResult.NextPageToken)
	}

	return results, nil
}

func (gce *Connection) removeInstance(id, zone string) error {
	call := gce.raw.Instances.Delete(gce.ProjectID, zone, id)
	operation, err := call.Do()
	if err != nil {
		return errors.Trace(err)
	}
	if err := gce.waitOperation(operation, attemptsLong); err != nil {
		return errors.Trace(err)
	}

	if err := gce.deleteFirewall(id); err != nil {
		return errors.Trace(err)
	}

	return nil
}

func (gce *Connection) RemoveInstances(prefix string, ids ...string) error {
	if len(ids) == 0 {
		return nil
	}

	instances, err := gce.Instances(prefix)
	if err != nil {
		return errors.Annotatef(err, "while removing instances %v", ids)
	}

	var failed []string
	for _, instID := range ids {
		for _, inst := range instances {
			if inst.ID == instID {
				if err := gce.removeInstance(instID, zoneName(inst)); err != nil {
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

func (gce *Connection) firewall(name string) (*compute.Firewall, error) {
	call := gce.raw.Firewalls.List(gce.ProjectID)
	call = call.Filter("name eq " + name)
	firewallList, err := call.Do()
	if err != nil {
		return nil, errors.Annotate(err, "while getting firewall from GCE")
	}
	if len(firewallList.Items) == 0 {
		return nil, errors.NotFoundf("firewall %q", name)
	}
	return firewallList.Items[0], nil
}

func (gce *Connection) insertFirewall(firewall *compute.Firewall) error {
	call := gce.raw.Firewalls.Insert(gce.ProjectID, firewall)
	operation, err := call.Do()
	if err != nil {
		return errors.Trace(err)
	}
	if err := gce.waitOperation(operation, attemptsLong); err != nil {
		return errors.Trace(err)
	}
	return nil
}

func (gce *Connection) updateFirewall(name string, firewall *compute.Firewall) error {
	call := gce.raw.Firewalls.Update(gce.ProjectID, name, firewall)
	operation, err := call.Do()
	if err != nil {
		return errors.Trace(err)
	}
	if err := gce.waitOperation(operation, attemptsLong); err != nil {
		return errors.Trace(err)
	}
	return nil
}

func (gce *Connection) deleteFirewall(name string) error {
	call := gce.raw.Firewalls.Delete(gce.ProjectID, name)
	operation, err := call.Do()
	if err != nil {
		return errors.Trace(err)
	}
	if err := gce.waitOperation(operation, attemptsLong); err != nil {
		return errors.Trace(err)
	}
	return nil
}

func (gce Connection) Ports(fwname string) ([]network.PortRange, error) {
	firewall, err := gce.firewall(fwname)
	if errors.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, errors.Annotate(err, "while getting ports from GCE")
	}

	var ports []network.PortRange
	for _, allowed := range firewall.Allowed {
		for _, portRangeStr := range allowed.Ports {
			portRange, err := network.ParsePortRange(portRangeStr)
			if err != nil {
				return ports, errors.Annotate(err, "bad ports from GCE")
			}
			portRange.Protocol = allowed.IPProtocol
			ports = append(ports, *portRange)
		}
	}

	return ports, nil
}

func (gce Connection) OpenPorts(name string, ports []network.PortRange) error {
	// Compose the full set of open ports.
	currentPorts, err := gce.Ports(name)
	if err != nil {
		return errors.Trace(err)
	}
	inputPortsSet := network.NewPortSet(ports...)
	if inputPortsSet.IsEmpty() {
		return nil
	}
	currentPortsSet := network.NewPortSet(currentPorts...)

	// Send the request, depending on the current ports.
	if currentPortsSet.IsEmpty() {
		firewall := firewallSpec(name, inputPortsSet)
		if err := gce.insertFirewall(firewall); err != nil {
			return errors.Annotatef(err, "opening port(s) %+v", ports)
		}

	} else {
		newPortsSet := currentPortsSet.Union(inputPortsSet)
		firewall := firewallSpec(name, newPortsSet)
		if err := gce.updateFirewall(name, firewall); err != nil {
			return errors.Annotatef(err, "opening port(s) %+v", ports)
		}
	}
	return nil
}

func (gce Connection) ClosePorts(name string, ports []network.PortRange) error {
	// Compose the full set of open ports.
	currentPorts, err := gce.Ports(name)
	if err != nil {
		return errors.Trace(err)
	}
	inputPortsSet := network.NewPortSet(ports...)
	if inputPortsSet.IsEmpty() {
		return nil
	}
	currentPortsSet := network.NewPortSet(currentPorts...)
	newPortsSet := currentPortsSet.Difference(inputPortsSet)

	// Send the request, depending on the current ports.
	if newPortsSet.IsEmpty() {
		if err := gce.deleteFirewall(name); err != nil {
			return errors.Annotatef(err, "closing port(s) %+v", ports)
		}
	} else {
		firewall := firewallSpec(name, newPortsSet)
		if err := gce.updateFirewall(name, firewall); err != nil {
			return errors.Annotatef(err, "closing port(s) %+v", ports)
		}
	}
	return nil
}

type InstanceSpec struct {
	ID                string
	Type              string
	Disks             []DiskSpec
	Network           NetworkSpec
	NetworkInterfaces []string
	Metadata          map[string]string
	Tags              []string
}

func (is InstanceSpec) Create(conn *Connection, zones []string) (*Instance, error) {
	raw, err := is.create(conn, zones)
	if err != nil {
		return nil, errors.Trace(err)
	}

	inst := newInstance(raw)
	copied := is
	inst.spec = &copied
	return inst, nil
}

func (is InstanceSpec) create(conn *Connection, zones []string) (*compute.Instance, error) {
	raw := &compute.Instance{
		Name:              is.ID,
		Disks:             is.disks(),
		NetworkInterfaces: is.networkInterfaces(),
		Metadata:          packMetadata(is.Metadata),
		Tags:              &compute.Tags{Items: is.Tags},
		// MachineType is set in the addInstance call.
	}
	err := conn.addInstance(raw, is.Type, zones)
	return raw, errors.Trace(err)
}

func (is InstanceSpec) disks() []*compute.AttachedDisk {
	var result []*compute.AttachedDisk
	for _, spec := range is.Disks {
		result = append(result, spec.newAttached())
	}
	return result
}

func (is InstanceSpec) networkInterfaces() []*compute.NetworkInterface {
	var result []*compute.NetworkInterface
	for _, name := range is.NetworkInterfaces {
		result = append(result, is.Network.newInterface(name))
	}
	return result
}

type Instance struct {
	ID   string
	Zone string
	raw  compute.Instance
	spec *InstanceSpec
}

func newInstance(raw *compute.Instance) *Instance {
	return &Instance{
		ID:   raw.Name,
		Zone: zoneName(raw),
		raw:  *raw,
	}
}

func (gi Instance) RootDiskGB() int64 {
	if gi.spec == nil {
		return 0
	}
	attached := rootDisk(gi.spec)
	// The root disk from a spec will not fail.
	size, _ := diskSizeGB(attached)
	return size
}

func (gi Instance) Status() string {
	return gi.raw.Status
}

func (gi *Instance) Refresh(conn *Connection) error {
	raw, err := conn.instance(gi.Zone, gi.ID)
	if err != nil {
		return errors.Trace(err)
	}

	gi.raw = *raw
	return nil
}

func (gi Instance) Addresses() ([]network.Address, error) {
	var addresses []network.Address

	for _, netif := range gi.raw.NetworkInterfaces {
		// Add public addresses.
		for _, accessConfig := range netif.AccessConfigs {
			if accessConfig.NatIP == "" {
				continue
			}
			address := network.Address{
				Value: accessConfig.NatIP,
				Type:  network.IPv4Address,
				Scope: network.ScopePublic,
			}
			addresses = append(addresses, address)

		}

		// Add private address.
		if netif.NetworkIP == "" {
			continue
		}
		address := network.Address{
			Value: netif.NetworkIP,
			Type:  network.IPv4Address,
			Scope: network.ScopeCloudLocal,
		}
		addresses = append(addresses, address)
	}

	return addresses, nil
}

func (gi Instance) Metadata() map[string]string {
	return unpackMetadata(gi.raw.Metadata)
}

func filterInstances(instances []Instance, statuses ...string) []Instance {
	var results []Instance
	for _, inst := range instances {
		if !checkInstStatus(inst, statuses...) {
			continue
		}
		results = append(results, inst)
	}
	return results
}

func checkInstStatus(inst Instance, statuses ...string) bool {
	for _, status := range statuses {
		if inst.Status() == status {
			return true
		}
	}
	return false
}

type DiskSpec struct {
	// sizeHint is the requested disk size in Gigabytes.
	SizeHintGB int64
	ImageURL   string
	Boot       bool
	Scratch    bool
	Readonly   bool
	AutoDelete bool
}

func (ds *DiskSpec) TooSmall() bool {
	return ds.SizeHintGB < MinDiskSizeGB
}

func (ds *DiskSpec) SizeGB() int64 {
	size := ds.SizeHintGB
	if ds.TooSmall() {
		size = MinDiskSizeGB
	}
	return size
}

func (ds *DiskSpec) newAttached() *compute.AttachedDisk {
	diskType := diskTypePersistent // The default.
	if ds.Scratch {
		diskType = diskTypeScratch
	}
	mode := diskModeRW // The default.
	if ds.Readonly {
		mode = diskModeRO
	}

	disk := compute.AttachedDisk{
		Type:       diskType,
		Boot:       ds.Boot,
		Mode:       mode,
		AutoDelete: ds.AutoDelete,
		InitializeParams: &compute.AttachedDiskInitializeParams{
			// DiskName (defaults to instance name)
			DiskSizeGb: ds.SizeGB(),
			// DiskType (defaults to pd-standard, pd-ssd, local-ssd)
			SourceImage: ds.ImageURL,
		},
		// Interface (defaults to SCSI)
		// DeviceName (GCE sets this, persistent disk only)
	}
	return &disk
}

func rootDisk(inst interface{}) *compute.AttachedDisk {
	switch typed := inst.(type) {
	case *compute.Instance:
		return typed.Disks[0]
	case *Instance:
		if typed.spec == nil {
			return nil
		}
		return typed.spec.Disks[0].newAttached()
	case *InstanceSpec:
		return typed.Disks[0].newAttached()
	default:
		return nil
	}
}

func diskSizeGB(disk interface{}) (int64, error) {
	switch typed := disk.(type) {
	case *compute.Disk:
		return typed.SizeGb, nil
	case *compute.AttachedDisk:
		if typed.InitializeParams == nil {
			return 0, errors.Errorf("attached disk missing init params: %v", disk)
		}
		return typed.InitializeParams.DiskSizeGb, nil
	default:
		return 0, errors.Errorf("disk has unrecognized type: %v", disk)
	}
}

func zoneName(value interface{}) string {
	// We trust that path.Base will always give the right answer
	// when used.
	switch typed := value.(type) {
	case *compute.Instance:
		return path.Base(typed.Zone)
	case *compute.Operation:
		return path.Base(typed.Zone)
	default:
		// TODO(ericsnow) Fail?
		return ""
	}
}

type NetworkSpec struct {
	Name string
	// TODO(ericsnow) support a CIDR for internal IP addr range?
}

func (ns *NetworkSpec) path() string {
	name := ns.Name
	if name == "" {
		name = networkDefaultName
	}
	return networkPathRoot + name
}

func (ns *NetworkSpec) newInterface(name string) *compute.NetworkInterface {
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
		// SourceTags is not set.
		TargetTags:   []string{name},
		SourceRanges: []string{"0.0.0.0/0"},
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

// FormatAuthorizedKeys returns our authorizedKeys with
// the username prepended to it. This is the format that
// GCE uses for its sshKeys metadata.
func FormatAuthorizedKeys(raw, user string) (string, error) {
	var userKeys string
	users := []string{user}
	keys := strings.Split(raw, "\n")
	for _, key := range keys {
		for _, user := range users {
			userKeys += user + ":" + key + "\n"
		}
	}
	return userKeys, nil
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

func resolveMachineType(zone, name string) string {
	return fmt.Sprintf(partialMachineType, zone, name)
}
