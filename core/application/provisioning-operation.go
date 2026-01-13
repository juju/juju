package application

type ProvisioningOperation string

const (
	ScaleOperation         ProvisioningOperation = "scale"
	StorageUpdateOperation ProvisioningOperation = "storage-update"
)
