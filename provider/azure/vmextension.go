// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package azure

import (
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/compute/armcompute"
	"github.com/juju/errors"

	jujuos "github.com/juju/juju/core/os"
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

// vmExtension creates a CustomScript VM extension for the given VM
// which will execute the CustomData on the machine as a script.
func vmExtensionProperties(os jujuos.OSType) (*armcompute.VirtualMachineExtensionProperties, error) {
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
		return nil, errors.NotSupportedf("CustomScript extension for OS %q", os)
	}

	extensionSettings := map[string]interface{}{
		"commandToExecute": commandToExecute,
	}
	return &armcompute.VirtualMachineExtensionProperties{
		Publisher:               to.Ptr(extensionPublisher),
		Type:                    to.Ptr(extensionType),
		TypeHandlerVersion:      to.Ptr(extensionVersion),
		AutoUpgradeMinorVersion: to.Ptr(true),
		Settings:                &extensionSettings,
	}, nil
}
