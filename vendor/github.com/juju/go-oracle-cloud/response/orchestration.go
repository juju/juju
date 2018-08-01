// Copyright 2017 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package response

import "github.com/juju/go-oracle-cloud/common"

// Orchestration is an orchestration defines the attributes and interdependencies
// of a collection of compute, networking, and storage resources
// in Oracle Compute Cloud Service. You can use orchestrations to automate
// the provisioning and lifecycle operations of an entire virtual compute topology.
// After creating an orchestration (in a JSON-formatted file) and adding it
// to Oracle Compute Cloud Service, you can trigger the creation and removal
// all the resources defined in the orchestration with a single step.
// An orchestration contains one or more object plans (oplans). The attributes
// that you can specify in an oplan vary depending on the object type (obj_type).
// For detailed information about the object types that you can create by using
// orchestrations and the attributes for each object type, see Attributes in Orchestrations
// in Using Oracle Compute Cloud Service (IaaS).
type Orchestration struct {

	// Relationships holds a slice of relationships that holds every
	// relationship between the objects
	Relationships []Relationship `json:"relationships,omitempty"`

	// Status shows the current status of the orchestration.
	Status string `json:"status"`

	// Account shows the default account for your identity domain.
	Account string `json:"account"`

	// Description is the description of this orchestration plan
	Description string `json:"description,omitempty"`

	// Schedule for an orchestration consists
	// of the start and stop dates and times.
	Schedule Schedule `json:"schedule"`

	// Uri is the Uniform Resource Identifier
	Uri string `json:"uri,omitempty"`

	// List of oplans. An object plan, or oplan,
	// is a top-level orchestration attribute.
	Oplans []Oplan `json:"oplans"`

	// Info the nested parameter errors shows which object
	// in the orchestration has encountered an error.
	// Empty if there are no errors.
	Info Info `json:"info,omitempty"`

	// User is the user of the orchestration
	User string `json:"user"`

	// Status_timestamp this information is generally displayed
	// at the end of the orchestration JSON.
	// It indicates the time that the current view of the
	// orchestration was generated. This information shows only when
	// the orchestration is running.
	Status_timestamp string `json:"status_timestamp"`

	// Name is the name of the orchestration
	Name string `json:"name"`
}

// Relationship type that will describe the relationship
// between objects
type Relationship struct {

	// ToOplan to witch orchestration plan should
	// be the orchestration in a relationship
	ToOplan string `json:"to_oplan,omitempty"`

	// Oplan orchestration plan
	Oplan string `json:"oplan,omitempty"`

	// The type of relationship that this orchestration
	// has with the other one in the ToOplan field
	Type string `json:"type,omitempty"`
}

// AllOrchestrations a holds a slice of all
// orchestrations of a oracle cloud account
type AllOrchestrations struct {
	Result []Orchestration `json:"result,omitmepty"`
}

// Schedule for an orchestration consists of
// the start and stop dates and times
type Schedule struct {
	//Start_time when the orchestration will start
	Start_time *string `json:"start_time,omitempty"`

	// Stop_time when the orchestration will stop
	Stop_time *string `json:"stop_time,omitempty"`
}

// Oplan is an object plan, or oplan,
// a top-level orchestration attribute
type Oplan struct {

	// Status is the most recent status.
	Status string `json:"status"`

	// Info dictionary for the oplan.
	Info Info `json:"info,omitempty"`

	// Obj_type type of the object.
	Obj_type string `json:"obj_type"`

	// Ha_policy indicates that description is not available
	Ha_policy string `json:"ha_policy,omitempty"`

	// Label is the description of this object plan.
	Label string `json:"label"`

	// Objects list of object dictionaries
	// or object names.
	Objects []Object `json:"objects"`

	// Status_timestamp Timestamp of the most-recent status change.
	Status_timestamp string `json:"status_timestamp,omitempty"`
}

type Info struct {
	Errors map[string]string `json:"errors,omitempty"`
}

// Objects
type Object struct {
	// Info is the aditional information about the orchestration object
	Info Info `json:"info,omitempty"`

	//
	// Intances is generally populated when we are dealing with an
	// instance orchestration
	//
	// Instances instances that are attached to this orchestration
	Instances []InstancesOrchestration `json:"instances,omitempty"`

	// Status is the status of the orchestration
	Status string `json:"status,omitempty"`

	// Name is the name of the object
	Name string `json:"name,omitempty"`

	// Status_timestamp is the status when the object was created
	Status_timestamp string `json:"status_timestamp,omitmepty"`

	// Uri is the endpoint uri that the object is located
	Uri *string `json:"uri,omitempty"`

	//
	//
	// Below these fields are populated when we are dealing with an
	// storage orchestration
	//
	//
	// Managed flag true if the storage is managed
	Managed bool `json:"managed,omitempty"`

	// Snapshot_account is the account name that the snapshot was created from
	Snapshot_account *string `json:"snapshot_account,omitempty"`

	// Machineimage_name is the name of the machine image
	// that the storage was/will bot from
	Machineimage_name string `json:"machineimage_name,omitempty"`

	// Snapshot_id is the snapshot id
	Snapshot_id *string `json:"snapshot_id,omitempty"`

	// Imagelist is the image list that the storage was created from
	Imagelist string `json:"imagelist,omitempty"`

	// Writecache flag true if the write cache is enabled on the storage
	Writecache bool `json:"writecache,omitempty"`

	// Size is the storage size in the cloud
	Size uint64 `json:"size,string"`

	// Platform is the platform storage could be linux,windows, etc.
	Platform string `json:"platform"`

	// Readonly flag true if the storage is read only and does not permit
	// write operation on
	Readonly bool `json:"readonly"`

	// Storage_pool is the storage endpoint pool that the storage
	// belongs to
	Storage_pool string `json:"storage_pool,omitempty"`

	// Shared flag true if the storage is shared across multiple instances
	Shared bool `json:"shared,omitempty"`

	// Description is the description of the object
	Description string `json:"description,omitempty"`

	// Tags are the tags, aliases of the object
	Tags []string `json:"tags,omitempty"`

	// Quota this field is not used
	Quota *string `json:"quota,omitempty"`

	// Properties of the storage obejct
	Properties []string `json:"properties,omitempty"`

	// Account is the account name of the object that has been created from
	Account string `json:"account"`

	// Bootable flag represents that the instance can boot from the storage
	Bootable bool `json:"bootable,omitempty"`

	// Hypervisor is the hypervisor name used in the object orchestration
	Hypervisor *string `json:"hypervisor,omitempty"`

	// Imagelist_entry is the index entry of the image that is used
	// in the storage obejct
	Imagelist_entry int `json:"imagelist_entry,omitempty"`

	// Snapshot is the snapshot name of the storage
	Snapshot *string `json:"snapshot,omitempty"`
}

// OType represents the orchestration type
type OType string

const (
	// Instance is the instance orchestration type
	OInstance OType = "Instance"
	// Storage is the storage orchestration type
	OStorage OType = "Storage"
	// Master is the master orchestration type
	OMaster OType = "Master"
)

// OrchestrationType returns the type of response orchestration we are
// dealing with
func (o Orchestration) OrchestrationType() OType {
	for _, oplan := range o.Oplans {
		for _, object := range oplan.Objects {
			if object.Instances != nil && len(object.Instances) > 0 {
				return OInstance
			}

			if object.Properties != nil && len(object.Properties) > 0 {
				return OStorage
			}
		}
	}
	return OMaster
}

// InstancesOrchestration holds information for
// an instances inside the orchestration object
type InstancesOrchestration struct {
	Hostname string `json:"hostname,omitempty"`

	// Networking information of the instance
	Networking common.Networking `json:"networking,omitempty"`

	// Name of the instance
	Name string `json:"name,omitempty"`

	// Boot_order is the number in what order the instance is booting
	Boot_order []int `json:"boot_order,omitempty"`

	Ip string `json:"ip,omitempty"`

	Start_time string `json:"start_time,omitempty"`

	// Storage_attachments list of storages that the instnaces has
	Storage_attachments []StorageOrhcestration `json:"storage_attachments,omitmepty"`

	// Uri of the instnace
	Uri *string `json:"uri,omitempty"`

	// Label is the label of the instance
	Label string `json:"label,omitempty"`

	// Shape is the shape of the instnace
	Shape string `json:"shape,omitempty"`

	State common.InstanceState `json:"state,omitempty"`

	// Attributes list of orchestration attributes
	Attributes AttributesOrchestration `json:"attributes,omitmepty"`

	// Imagelist is the image from what was created
	Imagelist string `json:"imagelist,omitempty"`

	// SSHKeys of the instance
	SSHkeys []string `json:"sshkeys,omitmepty"`

	// Tags are a list of tags, aliases for the instance
	Tags []string `json:"tags,omitmepty"`
}

type StorageOrhcestration struct {
	Volume string `json:"volume,omitempty"`
	Index  int    `json:"index,omitempty"`
}

type AttributesOrchestration struct {
	Userdata              map[string]string `json:"userdata,omitempty"`
	Nimbula_orchestration string            `json:"nimbula_orchestration,omitempty"`
}
