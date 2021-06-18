// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"sync"

	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
)

// Server implements an EC2 simulator for use in testing.
type Server struct {
	mu               sync.Mutex
	createRootDisks  bool
	apiCallErrors    map[string]error
	apiCallModifiers map[string][]func(interface{})

	attributes           map[string][]types.AccountAttributeValue // attr name -> values
	instances            map[string]*Instance                     // id -> instance
	reservations         map[string]*reservation                  // id -> reservation
	groups               map[string]*securityGroup                // id -> group
	zones                map[string]availabilityZone              // name -> availabilityZone
	vpcs                 map[string]*vpc                          // id -> vpc
	internetGateways     map[string]*internetGateway              // id -> igw
	routeTables          map[string]*routeTable                   // id -> table
	subnets              map[string]*subnet                       // id -> subnet
	ifaces               map[string]*iface                        // id -> iface
	volumes              map[string]*volume                       // id -> volume
	volumeAttachments    map[string]*volumeAttachment             // id -> volumeAttachment
	maxId                counter
	reqId                counter
	reservationId        counter
	groupId              counter
	vpcId                counter
	igwId                counter
	rtbId                counter
	rtbassocId           counter
	dhcpOptsId           counter
	subnetId             counter
	volumeId             counter
	ifaceId              counter
	attachId             counter
	initialInstanceState types.InstanceState
}

// NewServer returns a new server.
func NewServer() (*Server, error) {
	srv := &Server{}
	srv.Reset(false)

	return srv, nil
}

// SetAPIError causes an error to be returned for the named function.
func (srv *Server) SetAPIError(apiName string, err error) {
	srv.apiCallErrors[apiName] = err
}

// SetAPIModifiers calls the specified functions with the result of an api call.
func (srv *Server) SetAPIModifiers(apiName string, f ...func(interface{})) {
	srv.apiCallModifiers[apiName] = f
}

// Reset is a convenient helper to remove all test entities (e.g.
// VPCs, subnets, instances) from the test server and reset all id
// counters. The, if withoutZonesOrGroups is false, a default AZ and
// security group will be created.
func (srv *Server) Reset(withoutZonesOrGroups bool) {
	srv.mu.Lock()
	defer srv.mu.Unlock()

	srv.maxId.reset()
	srv.reqId.reset()
	srv.reservationId.reset()
	srv.groupId.reset()
	srv.vpcId.reset()
	srv.igwId.reset()
	srv.rtbId.reset()
	srv.rtbassocId.reset()
	srv.dhcpOptsId.reset()
	srv.subnetId.reset()
	srv.volumeId.reset()
	srv.ifaceId.reset()
	srv.attachId.reset()

	srv.apiCallErrors = make(map[string]error)
	srv.apiCallModifiers = make(map[string][]func(interface{}))
	srv.attributes = make(map[string][]types.AccountAttributeValue)
	srv.instances = make(map[string]*Instance)
	srv.groups = make(map[string]*securityGroup)
	srv.vpcs = make(map[string]*vpc)
	srv.zones = make(map[string]availabilityZone)
	srv.internetGateways = make(map[string]*internetGateway)
	srv.routeTables = make(map[string]*routeTable)
	srv.subnets = make(map[string]*subnet)
	srv.ifaces = make(map[string]*iface)
	srv.volumes = make(map[string]*volume)
	srv.volumeAttachments = make(map[string]*volumeAttachment)
	srv.reservations = make(map[string]*reservation)

	if !withoutZonesOrGroups {
		srv.addDefaultZonesAndGroups()
	}
}
