package azure

import (
	"github.com/Azure/azure-sdk-for-go/Godeps/_workspace/src/github.com/Azure/go-autorest/autorest/to"
	"github.com/Azure/azure-sdk-for-go/arm/compute"
	"github.com/juju/errors"
	jujuos "github.com/juju/utils/os"
)

const extensionName = "JujuCustomScriptExtension"

const (
	windowsExecuteCustomScriptCommand = `` +
		`move C:\AzureData\CustomData.bin C:\AzureData\CustomData.ps1 && ` +
		`powershell.exe -ExecutionPolicy Unrestricted -File C:\AzureData\CustomData.ps1 && ` +
		`del /q C:\AzureData\CustomData.ps1`
	windowsCustomScriptPublisher = "Microsoft.Compute"
	windowsCustomScriptType      = "CustomScriptExtension"
	windowsCustomScriptVersion   = "1.4"
)

const (
	// The string will be split and executed by Python's
	// subprocess.call, not interpreted as a shell command.
	linuxExecuteCustomScriptCommand = `bash -c 'base64 -d /var/lib/waagent/CustomData | bash'`
	linuxCustomScriptPublisher      = "Microsoft.OSTCExtensions"
	linuxCustomScriptType           = "CustomScriptForLinux"
	linuxCustomScriptVersion        = "1.4"
)

// createVMExtension creates a CustomScript VM extension for the given VM
// which will execute the CustomData on the machine as a script.
func createVMExtension(
	vmExtensionClient compute.VirtualMachineExtensionsClient,
	os jujuos.OSType, resourceGroup, vmName, location string, vmTags map[string]string,
) error {
	var commandToExecute, extensionPublisher, extensionType, extensionVersion string

	switch os {
	case jujuos.Windows:
		commandToExecute = windowsExecuteCustomScriptCommand
		extensionPublisher = windowsCustomScriptPublisher
		extensionType = windowsCustomScriptType
		extensionVersion = windowsCustomScriptVersion
	case jujuos.CentOS:
		commandToExecute = linuxExecuteCustomScriptCommand
		extensionPublisher = linuxCustomScriptPublisher
		extensionType = linuxCustomScriptType
		extensionVersion = linuxCustomScriptVersion
	default:
		// Ubuntu renders CustomData as cloud-config, and interprets
		// it with cloud-init. Windows and CentOS do not use cloud-init
		// on Azure.
		return errors.NotSupportedf("CustomScript extension for OS %q", os)
	}

	extensionSettings := map[string]*string{
		"commandToExecute": to.StringPtr(commandToExecute),
	}
	extension := compute.VirtualMachineExtension{
		Location: to.StringPtr(location),
		Tags:     toTagsPtr(vmTags),
		Properties: &compute.VirtualMachineExtensionProperties{
			Publisher:               to.StringPtr(extensionPublisher),
			Type:                    to.StringPtr(extensionType),
			TypeHandlerVersion:      to.StringPtr(extensionVersion),
			AutoUpgradeMinorVersion: to.BoolPtr(true),
			Settings:                &extensionSettings,
		},
	}
	_, err := vmExtensionClient.CreateOrUpdate(
		resourceGroup, vmName, extensionName, extension,
	)
	return err
}
