// Copyright 2011, 2013, 2015 Canonical Ltd.
// Copyright 2015 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

// The cloudinit package implements a way of creating
// a cloud-init configuration file.
// See https://help.ubuntu.com/community/CloudInit.
package cloudinit

import (
	"github.com/juju/errors"
	"github.com/juju/utils/packaging"
	"github.com/juju/utils/packaging/commands"
	"github.com/juju/utils/packaging/config"
	"github.com/juju/utils/proxy"
	"github.com/juju/utils/shell"

	"github.com/juju/juju/version"
)

// CloudConfig is the interface of all cloud-init cloudconfig options.
type CloudConfig interface {
	// SetAttr sets an arbitrary attribute in the cloudinit config.
	// The value will be marshalled according to the rules
	// of the goyaml.Marshal.
	SetAttr(string, interface{})

	// UnsetAttr unsets the attribute given from the cloudinit config.
	// If the attribute has not been previously set, no error occurs.
	UnsetAttr(string)

	// GetSeries returns the series this CloudConfig was made for.
	GetSeries() string

	// CloudConfig also contains all the smaller interfaces for config
	// management:
	UserConfig
	SystemUpdateConfig
	SystemUpgradeConfig
	PackageProxyConfig
	PackageMirrorConfig
	PackageSourcesConfig
	PackagingConfig
	RunCmdsConfig
	BootCmdsConfig
	EC2MetadataConfig
	FinalMessageConfig
	LocaleConfig
	DeviceMountConfig
	OutputConfig
	SSHKeysConfig
	RootUserConfig
	WrittenFilesConfig
	RenderConfig
	AdvancedPackagingConfig
}

// UserConfig is the interface for managing all user-related settings.
type UserConfig interface {
	// SetUser sets the username to be written in the config.
	// NOTE: the user must exist beforehand, as no steps are taken to create it.
	// NOTE: if not set, cloud-init defaults to using "ubuntu"
	SetUser(string)

	// UnsetUser unsets the "user" cloudinit config attribute set with SetUser.
	// If the attribute has not been previously set, no error occurs.
	UnsetUser()

	// User returns the value set with SetUser or an empty string.
	User() string
}

// SystemUpdateConfig is the interface for managing all system update options.
type SystemUpdateConfig interface {
	// SetSystemUpdate sets whether the system should refresh the local package
	// database on first boot.
	// NOTE: This option is active in cloudinit by default and must be
	// explicitly set to false if it is not desired.
	SetSystemUpdate(bool)

	// UnsetSystemUpdate unsets the package list updating option set by
	// SetSystemUpdate, returning it to the cloudinit *default of true*.
	// If the option has not previously been set, no error occurs.
	UnsetSystemUpdate()

	// SystemUpdate returns the value set with SetSystemUpdate or false
	// NOTE: even though not set, the cloudinit-defined default is true.
	SystemUpdate() bool
}

// SystemUpgradeConfig is the interface for managing all system upgrade settings.
type SystemUpgradeConfig interface {
	// SetSystemUpgrade sets whether cloud-init should run the process of upgrading
	// all the packages with available newer versions on the machine's *first* boot.
	SetSystemUpgrade(bool)

	// UnsetSystemUpgrade unsets the value set by SetSystemUpgrade.
	// If the option has not previously been set, no error occurs.
	UnsetSystemUpgrade()

	// SystemUpgrade returns the value set by SetSystemUpgrade or
	// false if no call to SetSystemUpgrade has been made.
	SystemUpgrade() bool
}

// PackageProxyConfig is the interface for packaging proxy settings on a cloudconfig
type PackageProxyConfig interface {
	// SetPackageProxy sets the URL to be used as a proxy by the
	// specific package manager
	SetPackageProxy(string)

	// UnsetPackageProxy unsets the option set by SetPackageProxy
	// If it has not been previously set, no error occurs
	UnsetPackageProxy()

	// PackageProxy returns the URL of the proxy server set using
	// SetPackageProxy or an empty string if it has not been set
	PackageProxy() string
}

// PackageMirrorConfig is the interface for package mirror settings on a cloudconfig.
type PackageMirrorConfig interface {
	// SetPackageMirror sets the URL to be used as the mirror for
	// pulling packages by the system's specific package manager.
	SetPackageMirror(string)

	// UnsetPackageMirror unsets the value set by SetPackageMirror
	// If it has not been previously set, no error occurs.
	UnsetPackageMirror()

	// PackageMirror returns the URL of the package mirror set by
	// SetPackageMirror or an empty string if not previously set.
	PackageMirror() string
}

// PackageSourceConfig is the interface for package source settings on a cloudconfig.
type PackageSourcesConfig interface {
	// AddPackageSource adds a new repository and optional key to be
	// used as a package source by the system's specific package manager.
	AddPackageSource(packaging.PackageSource)

	// PackageSources returns all sources set with AddPackageSource.
	PackageSources() []packaging.PackageSource

	// AddPackagePreferences adds the necessary options and/or bootcmds to
	// enable the given packaging.PackagePreferences.
	AddPackagePreferences(packaging.PackagePreferences)

	// PackagePreferences returns the previously-added PackagePreferences.
	PackagePreferences() []packaging.PackagePreferences
}

// PackagingConfig is the interface for all packaging-related operations.
type PackagingConfig interface {
	// AddPackage adds a package to be installed on *first* boot.
	AddPackage(string)

	// RemovePackage removes a package from the list of to be installed packages
	// If the package has not been previously installed, no error occurs.
	RemovePackage(string)

	// Packages returns a list of all packages that will be installed.
	Packages() []string
}

// RunCmdsConfig is the interface for all operations on first-boot commands.
type RunCmdsConfig interface {
	// AddRunCmd adds a command to be executed on *first* boot.
	// It can recieve any number of string arguments, which will be joined into
	// a single command and passed to cloudinit to be executed.
	// NOTE: metacharacters will *not* be escaped!
	AddRunCmd(...string)

	// AddScripts simply calls AddRunCmd on every string passed to it.
	// NOTE: this means that each given string must be a full command plus
	// all of its arguments.
	// NOTE: metacharacters will not be escaped.
	AddScripts(...string)

	// RemoveRunCmd removes the given command from the list of commands to be
	// run on first boot. If it has not been previously added, no error occurs.
	RemoveRunCmd(string)

	// RunCmds returns all the commands added with AddRunCmd or AddScript.
	RunCmds() []string
}

// BootCmdsConfig is the interface for all operations on early-boot commands.
type BootCmdsConfig interface {
	// AddBootCmd adds a command to be executed on *every* boot.
	// It can recieve any number of string arguments, which will be joined into
	// a single command.
	// NOTE: metacharecters will not be escaped.
	AddBootCmd(...string)

	// RemoveBootCmd removes the given command from the list of commands to be
	// run every boot. If it has not been previously added, no error occurs.
	RemoveBootCmd(string)

	// BootCmds returns all the commands added with AddBootCmd.
	BootCmds() []string
}

// EC2MetadataConfig is the interface for all EC2-metadata related settings.
type EC2MetadataConfig interface {
	// SetDisableEC2Metadata sets whether access to the EC2 metadata service is
	// disabled early in boot via a null route. The default value is false.
	// (route del -host 169.254.169.254 reject).
	SetDisableEC2Metadata(bool)

	// UnsetDisableEC2Metadata unsets the value set by SetDisableEC2Metadata,
	// returning it to the cloudinit-defined value of false.
	// If the option has not been previously set, no error occurs.
	UnsetDisableEC2Metadata()

	// DisableEC2Metadata returns the value set by SetDisableEC2Metadata or
	// false if it has not been previously set.
	DisableEC2Metadata() bool
}

// FinalMessageConfig is the interface for all settings related to the
// cloudinit final message.
type FinalMessageConfig interface {
	// SetFinalMessage sets to message that will be written when the system has
	// finished booting for the first time. By default, the message is:
	// "cloud-init boot finished at $TIMESTAMP. Up $UPTIME seconds".
	SetFinalMessage(string)

	// UnsetFinalMessage unsets the value set by SetFinalMessage.
	// If it has not been previously set, no error occurs.
	UnsetFinalMessage()

	// FinalMessage returns the value set using SetFinalMessage or an empty
	// string if it has not previously been set.
	FinalMessage() string
}

// LocaleConfig is the interface for all locale-related setting operations.
type LocaleConfig interface {
	// SetLocale sets the locale; it defaults to en_US.UTF-8
	SetLocale(string)

	// UnsetLocale unsets the option set by SetLocale, returning it to the
	// cloudinit-defined default of en_US.UTF-8
	// If it has not been previously set, no error occurs
	UnsetLocale()

	// Locale returns the locale set with SetLocale
	// If it has not been previously set, an empty string is returned
	Locale() string
}

// DeviceMountConfig is the interface for all device mounting settings.
type DeviceMountConfig interface {
	// AddMount adds takes arguments for installing a mount point in /etc/fstab
	// The options are of the order and format specific to fstab entries:
	// <device> <mountpoint> <filesystem> <options> <backup setting> <fsck priority>
	AddMount(...string)
}

// OutputConfig is the interface for all stdout and stderr setting options.
type OutputConfig interface {
	// SetOutput specifies the destinations of standard output and standard error of
	// particular kinds of an output stream.
	// Valid values include:
	//	- init:		the output of cloudinit itself
	//	- config:	cloud-config caused output
	//	- final:	the final output of cloudinit (plus that set with SetFinalMessage)
	//	- all:		all of the above
	// Both stdout and stderr can take the following forms:
	//	- > file:	write to given file. Will truncate of file exists
	//	- >>file:	append to given file
	//	- | command:	pipe output to given command
	SetOutput(OutputKind, string, string)

	// Output returns the destination set by SetOutput for the given OutputKind.
	// If it has not been previously set, empty strings are returned.
	Output(OutputKind) (string, string)
}

// SSHKeysConfig is the interface for all ssh key-related settings.
type SSHKeysConfig interface {
	// AddSSHKey adds a pre-generated ssh key to the server keyring.
	// Valid SSHKeyType options are: rsa_{public,private}, dsa_{public,private}
	// Added keys will be written to /etc/ssh.
	// As a result, new random keys are prevented from being generated.
	AddSSHKey(SSHKeyType, string)

	// AddSSHAuthorizedKeys adds a set of keys in ssh authorized_keys format
	// that will be added to ~/.ssh/authorized_keys for the configured user (see SetUser).
	AddSSHAuthorizedKeys(string)
}

// RootUserConfig is the interface for all root user-related settings.
type RootUserConfig interface {
	// SetDisableRoot sets whether ssh login to the root account of the new server
	// through the ssh authorized key provided with the config should be disabled.
	// This option is set to true (ie. disabled) by default.
	SetDisableRoot(bool)

	// UnsetDisable unsets the value set with SetDisableRoot, returning it to the
	// cloudinit-defined default of true.
	UnsetDisableRoot()

	// DisableRoot returns the value set by SetDisableRoot or false if the
	// option had not been previously set.
	DisableRoot() bool
}

// WrittenFilesConfig is the interface for all file writing operaions.
type WrittenFilesConfig interface {
	// AddRunTextFile simply issues some AddRunCmd's to set the contents of a
	// given file with the specified file permissions on *first* boot.
	// NOTE: if the file already exists, it will be truncated.
	AddRunTextFile(string, string, uint)

	// AddBootTextFile simply issues some AddBootCmd's to set the contents of a
	// given file with the specified file permissions on *every* boot.
	// NOTE: if the file already exists, it will be truncated.
	AddBootTextFile(string, string, uint)

	// AddRunBinaryFile simply issues some AddRunCmd's to set the binary contents
	// of a given file with the specified file permissions on *first* boot.
	// NOTE: if the file already exists, it will be truncated.
	AddRunBinaryFile(string, []byte, uint)
}

// RenderConfig provides various ways to render a CloudConfig.
type RenderConfig interface {
	// Renders the current cloud config as valid YAML
	RenderYAML() ([]byte, error)

	// Renders a script that will execute the cloud config
	// It is eiher used over ssh for bootstrapping and manual or locally by
	// the local provider
	RenderScript() (string, error)

	// ShellRenderer renturns the shell renderer of this particular instance.
	ShellRenderer() shell.Renderer

	// getCommandsForAddingPackages is a helper function which returns all the
	// necessary shell commands for adding all the configured package settings.
	getCommandsForAddingPackages() ([]string, error)
}

// Makes two more advanced package commands available
type AdvancedPackagingConfig interface {
	// Adds the necessary commands for installing the required packages for
	// each OS is they are necessary.
	AddPackageCommands(
		aptProxySettings proxy.Settings,
		aptMirror string,
		addUpdateScripts bool,
		addUpgradeScripts bool,
	)

	// getPackageCommander returns the PackageCommander of the CloudConfig.
	getPackageCommander() commands.PackageCommander

	// getPackagingConfigurer returns the PackagingConfigurer of the CloudConfig.
	getPackagingConfigurer() config.PackagingConfigurer

	// addRequiredPackages is a helper to add packages that juju requires in
	// order to operate.
	addRequiredPackages()

	//TODO(bogdanteleaga): this might be the same as the exported proxy setting up above, need
	//to investigate how they're used
	updateProxySettings(proxy.Settings)

	// RequiresCloudArchiveCloudTools determines whether the cloudconfig
	// requires the configuration of the cloud archive depending on its series.
	RequiresCloudArchiveCloudTools() bool

	// AddCloudArchiveCloudTools configures the cloudconfig to set up the cloud
	// archive if it is required (eg: LTS'es).
	AddCloudArchiveCloudTools()
}

// New returns a new Config with no options set.
func New(series string) (CloudConfig, error) {
	os, err := version.GetOSFromSeries(series)
	if err != nil {
		return nil, err
	}
	switch os {
	case version.Windows:
		renderer, _ := shell.NewRenderer("powershell")
		return &windowsCloudConfig{
			&cloudConfig{
				series:   series,
				renderer: renderer,
				attrs:    make(map[string]interface{}),
			},
		}, nil
	case version.Ubuntu:
		renderer, _ := shell.NewRenderer("bash")
		return &ubuntuCloudConfig{
			&cloudConfig{
				series:    series,
				paccmder:  commands.NewAptPackageCommander(),
				pacconfer: config.NewAptPackagingConfigurer(series),
				renderer:  renderer,
				attrs:     make(map[string]interface{}),
			},
		}, nil
	case version.CentOS:
		renderer, _ := shell.NewRenderer("bash")
		return &centOSCloudConfig{
			&cloudConfig{
				series:    series,
				paccmder:  commands.NewYumPackageCommander(),
				pacconfer: config.NewYumPackagingConfigurer(series),
				renderer:  renderer,
				attrs:     make(map[string]interface{}),
			},
		}, nil
	default:
		return nil, errors.NotFoundf("cloudconfig for series %q", series)
	}
}

// SSHKeyType is the type of the four used key types passed to cloudinit
// through the cloudconfig
type SSHKeyType string

// The constant SSH key types sent to cloudinit through the cloudconfig
const (
	RSAPrivate SSHKeyType = "rsa_private"
	RSAPublic  SSHKeyType = "rsa_public"
	DSAPrivate SSHKeyType = "dsa_private"
	DSAPublic  SSHKeyType = "dsa_public"
)

// OutputKind represents the available destinations for command output as sent
// through the cloudnit cloudconfig
type OutputKind string

// The constant output redirection options available to be passed to cloudinit
const (
	// the output of cloudinit iself
	OutInit OutputKind = "init"
	// cloud-config caused output
	OutConfig OutputKind = "config"
	// the final output of cloudinit
	OutFinal OutputKind = "final"
	// all of the above
	OutAll OutputKind = "all"
)
