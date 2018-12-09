// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"github.com/juju/go-oracle-cloud/api"
	"github.com/juju/go-oracle-cloud/common"
	"github.com/juju/go-oracle-cloud/response"

	"github.com/juju/juju/environs"
	"github.com/juju/juju/instance"
)

type OracleInstancer interface {
	environs.Environ

	Details(id instance.Id) (response.Instance, error)
}

// Instancer used to retrieve details from a given instance
// in the oracle cloud infrastructure
type Instancer interface {
	// InstanceDetails retrieves information from the provider
	// about one instance
	InstanceDetails(string) (response.Instance, error)
}

// Composer has the simple task of composing an oracle compatible
// resource name
type Composer interface {
	// ComposeName composes the name for a provider resource. The name
	// for an oracle API resource typically has the following form:
	//
	// /Compute-<Identity Domain>/<username>/<resource name>
	//
	// The Identity Domain in this case equates to what some other providers
	// like OpenStack refer to as tenants or projects.
	// This information is supplied by the user in the cloud configuration
	// information.
	// This function is generally needed
	ComposeName(string) string
}

// InstanceAPI uses to retrieve all instances, delete and create
// in the oracle cloud infrastructure
type InstanceAPI interface {
	CreateInstance(api.InstanceParams) (response.LaunchPlan, error)
	AllInstances([]api.Filter) (response.AllInstances, error)
	DeleteInstance(string) error
}

// Authenticater authenticates the oracle client in the
// oracle IAAS api
type Authenticater interface {
	Authenticate() error
}

// Shaper used to retrieve all shapes from the oracle cloud
// environ
type Shaper interface {
	AllShapes([]api.Filter) (response.AllShapes, error)
}

// Imager used to retrieve all images iso meta data format from the
// oracle cloud environment
type Imager interface {
	AllImageLists([]api.Filter) (response.AllImageLists, error)
	CreateImageList(def int, description string, name string) (resp response.ImageList, err error)
	CreateImageListEntry(name string, attributes map[string]interface{}, version int, machineImages []string) (resp response.ImageListEntryAdd, err error)
	DeleteImageList(name string) (err error)
}

// IpReservationAPI provider methods for retrieving, updating, creating
// and deleting ip reservations inside the oracle cloud infrastructure
type IpReservationAPI interface {
	AllIpReservations([]api.Filter) (response.AllIpReservations, error)
	UpdateIpReservation(string, string,
		common.IPPool, bool, []string) (response.IpReservation, error)
	CreateIpReservation(string, common.IPPool,
		bool, []string) (response.IpReservation, error)
	DeleteIpReservation(string) error
}

// IpAssociationAPI provides methods for creating deleting and listing all
// ip associations inside the oracle cloud environment
type IpAssociationAPI interface {
	CreateIpAssociation(common.IPPool, common.VcableID) (response.IpAssociation, error)
	DeleteIpAssociation(string) error
	AllIpAssociations([]api.Filter) (response.AllIpAssociations, error)
}

// IpNetworkExchanger provides a simple interface for retrieving all
// ip network exchanges inside the oracle cloud environment
type IpNetworkExchanger interface {
	AllIpNetworkExchanges([]api.Filter) (response.AllIpNetworkExchanges, error)
}

// IpNetowrker provides a simple interface for retrieving all
// ip networks inside the oracle cloud environment
type IpNetworker interface {
	AllIpNetworks([]api.Filter) (response.AllIpNetworks, error)
}

// VnicSetAPI provides methods for deleting, retrieving details and creating
// virtual nics for providing access for instances to different subnets inside
// the oracle cloud environment
type VnicSetAPI interface {
	DeleteVnicSet(string) error
	VnicSetDetails(string) (response.VnicSet, error)
	CreateVnicSet(api.VnicSetParams) (response.VnicSet, error)
}

// RulesAPI defines methods for retrieving, creating and deleting
// Sec rules under the oracle cloud endpoint
// For more information on sec rules, please see:
// https://docs.oracle.com/cloud/latest/stcomputecs/STCSA/api-SecRules.html
type RulesAPI interface {
	// AllSecRules returns all sec rules matching a filter. A nil valued
	// filter will return all entries in the API.
	AllSecRules([]api.Filter) (response.AllSecRules, error)
	// DeleteSecRule deletes the security rule with the given name
	DeleteSecRule(string) error
	// CreateSecRule creates the security rule inside oracle cloud
	// given the sec rule parameters
	CreateSecRule(api.SecRuleParams) (response.SecRule, error)
}

// AclAPI defines methods for retrieving, creating and deleting
// access control lists under the oracle cloud endpoint
type AclAPI interface {
	// AclDetails retrieves the access control list details for one list
	AclDetails(string) (response.Acl, error)
	// CreateAcl creates the access control list
	CreateAcl(string, string, bool, []string) (response.Acl, error)
	// DeleteAcl deletes one access control list
	DeleteAcl(string) error
	// AllAcls fetches a list of all ACLs matching the given filter.
	AllAcls([]api.Filter) (response.AllAcls, error)
}

// SecIpAPI defines methods for retrieving creating sec IP lists
// in the oracle cloud
// For more information about sec ip lists, please see:
// https://docs.oracle.com/cloud/latest/stcomputecs/STCSA/api-SecIPLists.html
type SecIpAPI interface {
	// AllSecIpLists returns all sec IP lists that match a filter. A nil valued
	// filter will return all entries in the API.
	AllSecIpLists([]api.Filter) (response.AllSecIpLists, error)
	// CreateSecIpList creates the sec IP list under the oracle cloud endpoint
	CreateSecIpList(string, string, []string) (response.SecIpList, error)
	// AllDefaultSecIpLists retrieves all default sec IP lists from the
	// oracle cloud account. Default lists are defined by the cloud and cannot
	// be changed in any way.
	AllDefaultSecIpLists([]api.Filter) (response.AllSecIpLists, error)
}

// IpAddressPrefixSetAPI defines methods for creating and listing
// IP address prefix sets under the oracle cloud endpoint
// For information about IP address prefix sets, please see:
// https://docs.oracle.com/cloud/latest/stcomputecs/STCSA/api-IPAddressPrefixSets.html
type IpAddressPrefixSetAPI interface {
	// CreateIpAddressPrefixSet creates a new IP address prefix set inside the oracle
	// cloud, for the current user
	CreateIpAddressPrefixSet(
		api.IpAddressPrefixSetParams) (response.IpAddressPrefixSet, error)

	// AllIpAddressPrefixSets returns all IP address prefix sets that match the given filter.
	// A nil valued filter will return all entries in the API.
	AllIpAddressPrefixSets([]api.Filter) (response.AllIpAddressPrefixSets, error)
}

// SecListAPI defines methods for retrieving, creating and deleting
// sec lists under the oracle cloud endpoint
// For more information about sec lists, please see:
// https://docs.oracle.com/cloud/latest/stcomputecs/STCSA/api-SecLists.html
type SecListAPI interface {
	// SecListDetails retrieves sec list details for the given list
	SecListDetails(string) (response.SecList, error)
	// DeleteSecList deletes one sec list
	DeleteSecList(string) error
	// CreateSecList creates a sec list
	CreateSecList(string, string,
		common.SecRuleAction, common.SecRuleAction) (response.SecList, error)
}

// SecRulesAPI defines methods for retrieving, deleting and creating
// security rules under the oracle cloud endpoint
// For more details on sec rules, please see:
// https://docs.oracle.com/cloud/latest/stcomputecs/STCSA/api-SecRules.html
type SecRulesAPI interface {
	// AllSecurityRules returns all security rules matching a filter. A nil valued
	// filter will return all entries in the API.
	AllSecurityRules([]api.Filter) (response.AllSecurityRules, error)
	// DeleteSecurityRule deletes the security rule with the given name
	DeleteSecurityRule(string) error
	// CreateSecurityRule creates a security rule based on the security rule
	// parameters under the oracle cloud endpoint
	CreateSecurityRule(api.SecurityRuleParams) (response.SecurityRule, error)
}

// ApplicationsAPI also named protocols in the oracle cloud defines methods
// for retriving and creating applications rules/protocol rules
// under the oracle cloud endpoint
// For more information about sec applications, please see:
// https://docs.oracle.com/cloud/latest/stcomputecs/STCSA/api-SecApplications.html
type ApplicationsAPI interface {
	// AllSecApplications returns all sec applications matching a filter. A nil valued
	// filter will return all entries in the API.
	AllSecApplications([]api.Filter) (response.AllSecApplications, error)
	// DefaultSecApplications returns all default security applications matching a filter.
	// A nil valued filter will return all entries in the API.
	DefaultSecApplications([]api.Filter) (response.AllSecApplications, error)
	// CreateSecApplications creates a security applications
	CreateSecApplication(api.SecApplicationParams) (response.SecApplication, error)
}

// AssociationAPI defines a rule for listing, retrieving all security
// associations under the oracle cloud API
// For more details about sec associations, please see:
// https://docs.oracle.com/cloud/latest/stcomputecs/STCSA/api-SecAssociations.html
type AssociationAPI interface {
	// AllSecAssociations returns all security associations matching a filter. A nil valued
	// filter will return all entries in the API.
	AllSecAssociations([]api.Filter) (response.AllSecAssociations, error)
}

// StorageVolumeAPI defines methods for retrieving, creating, deleting and
// updating storage volumes under the oracle cloud endpoint
// For more details about storage volumes, please see:
// https://docs.oracle.com/cloud/latest/stcomputecs/STCSA/api-StorageVolumes.html
type StorageVolumeAPI interface {
	// CreateStorageVolume creates a storage volume
	CreateStorageVolume(p api.StorageVolumeParams) (resp response.StorageVolume, err error)
	// DeleteStorageVolume deletes the storage volume
	DeleteStorageVolume(name string) (err error)
	// StorageVolumeDetails retrieves storage volume details
	StorageVolumeDetails(name string) (resp response.StorageVolume, err error)
	// AllStoragevolumes retrieves all storage volumes matching a filter. A nil valued
	// filter will return all entries in the API.
	AllStorageVolumes(filter []api.Filter) (resp response.AllStorageVolumes, err error)
	// UpdateStorageVolume updates the state of the storage volume
	UpdateStorageVolume(p api.StorageVolumeParams, currentName string) (resp response.StorageVolume, err error)
}

// StorageAttachmentAPI defines methods for attaching, detaching storages to
// instances under the oracle cloud endpoint
// For more information on storage attachments, please see:
// https://docs.oracle.com/cloud/latest/stcomputecs/STCSA/api-StorageAttachments.html
type StorageAttachmentAPI interface {
	// CreateStorageAttachment creates a storage attachment
	CreateStorageAttachment(p api.StorageAttachmentParams) (response.StorageAttachment, error)
	// DeleteStorageAttachment deletes the storage attachment
	DeleteStorageAttachment(name string) error
	// StorageAttachmentDetails retrieves details of the storage attachment
	StorageAttachmentDetails(name string) (response.StorageAttachment, error)
	// AllStorageAttachments retrieves all storage attachments matching a filter. A nil valued
	// filter will return all entries in the API.
	AllStorageAttachments(filter []api.Filter) (response.AllStorageAttachments, error)
}
