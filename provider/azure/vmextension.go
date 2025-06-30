// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package azure

import (
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/compute/armcompute/v4"
	"github.com/juju/errors"

	"github.com/juju/juju/core/os/ostype"
)

const extensionName = "JujuCustomScriptExtension"

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
func vmExtensionProperties(os ostype.OSType) (*armcompute.VirtualMachineExtensionProperties, error) {
	var commandToExecute, extensionPublisher, extensionType, extensionVersion string

	switch os {
	case ostype.CentOS:
		commandToExecute = linuxExecuteCustomScriptCommand
		extensionPublisher = linuxCustomScriptPublisher
		extensionType = linuxCustomScriptType
		extensionVersion = linuxCustomScriptVersion
	default:
		// Ubuntu renders CustomData as cloud-config, and interprets
		// it with cloud-init. CentOS does not use cloud-init on Azure.
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
