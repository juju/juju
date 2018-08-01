package response

import "github.com/juju/go-oracle-cloud/common"

// Backup allows you to create a backup right
// away using a specified backup configuration.
// You can also view scheduled backups and their status,
// or delete a specified backup and the corresponding snapshot.
type Backup struct {

	// BackupConfigurationName is the name of the backup configuration
	BackupConfigurationName string `json:"backupConfigurationName"`

	// Bootable is the volume this Backup is associated with a bootable volume
	Bootable bool `json:"bootable"`

	// Description is the description of the Backup
	Description *string `json:"description,omitempty"`

	// detailedErrorMessage is a human readable detailed error message
	DetailedErrorMessage *string `json:"detailedErrorMessage,omitempty"`

	// ErrorMessage is a human readable error message
	ErrorMessage *string `json:"errorMessage,omitempty"`

	// Name is the name of the backup
	Name string `json:"name"`

	//RunAsUser is any actions on this model will be performed as this user
	RunAsUser string `json:"runAsUser"`

	// Shared ss the volume this Backup is associated with a shared volume
	Shared bool `json:"shared"`

	// SnapshotSize is the size of the snapshot
	SnapshotSize *string `json:"snapshotSize"`

	// SnapshotUri is the snapshot created by this Backup
	SnapshotUri *string `json:"snapshotUri"`

	// State of this resource.
	// Allowed Values:
	// common.Submitted,
	// common.Inprogress,
	// common.Completed,
	// common.Failed,
	// common.Canceling,
	// common.Canceled,
	// common.Timeout,
	// common.DeleteSubmitted,
	// common.Deleting,
	// common.Deleted,
	State common.BackupState `json:"state"`

	// TagID used to tag other cloud resources
	TagID string `json:"tagId"`

	// Uri is the Uniform Resource Identifier
	Uri string `json:"uri"`

	// VolumeUri is the Backup that was created from
	VolumeUri string `json:"volumeUri"`
}

type AllBackups struct {
	Result []Backup `json:"result"`
}
