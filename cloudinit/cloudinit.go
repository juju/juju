// Copyright 2011, 2013, 2015 Canonical Ltd.
// Copyright 2015 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

// The cloudinit package implements a way of creating
// a cloud-init configuration file.
// See https://help.ubuntu.com/community/CloudInit.
package cloudinit

import (
	"github.com/juju/errors"
	"github.com/juju/juju/cloudinit/packaging"
	"github.com/juju/juju/version"
)

// Config is the interface of a cloud-init specific config struct
type Config interface {
	// SetAttr sets an arbitrary attribute in the cloudinit config
	// The value will be marshalled according to the rules
	// of the goyaml.Marshal function if it has been set
	SetAttr(string, interface{})

	// UnsetAttr unsets the attribute given from the cloudinit config
	// If the attribute has not bee previously set, no error occurs
	UnsetAttr(string)

	// getAttrs returns the attributes map of this particular cloudinit config
	getAttrs() map[string]interface{}

	// SetUser sets the user name that will be used for some other options.
	// The user will be assumed to already exist in the machine image.
	// The default user is "ubuntu".
	SetUser(string)

	// UnsetUser unsets the "user" cloudinit config attribute set with SetUser
	// If the attribute has not been previously set, no error occurs
	UnsetUser()

	// SetSystemUpdate sets wether the system should refresh the local package
	// database on first boot. This option is active in cloudinit by default
	SetSystemUpdate(bool)

	// UnsetSystemUpdate unsets the package list updating option set by
	// SetSystemUpdate, returning it to the cloudinit *default of true*
	UnsetSystemUpdate()

	// SystemUpdate returns the value set with SetSystemUpdate or false
	// NOTE: even though not set, cloudinit-defined default is true
	SystemUpdate() bool

	// SetSystemUpgrade sets whether cloud-init runs the process of upgrading
	// all the packages on the newly-booted machine
	SetSystemUpgrade(bool)

	// UnsetSystemUpgrade unsets the value set by SetSystemUpgrade
	// If not previously set, no error occurs
	UnsetSystemUpgrade()

	// SystemUpgrade returns the value set by SetSystemUpgrade, or
	// false if no call to SetSystemUpgrade has been made.
	SystemUpgrade() bool

	// SetPackageProxy sets the URL to be used as a proxy by the
	// specific package manager
	SetPackageProxy(string)

	// UnsetPackageProxy unsets the option set by SetPackageProxy
	// If it has not been previously set, no error occurs
	UnsetPackageProxy()

	// PackageProxy returns the URL of the proxy server set using
	// SetPackageProxy or an empty string if it has not been set
	PackageProxy() string

	// SetPackageMirror sets the URL to be used as the mirror for
	// pulling packages by the system's specific package manager
	SetPackageMirror(string)

	// UnsetPackageMirror unsets the value set by SetPackageMirror
	// If it has not been previously set, no error occurs
	UnsetPackageMirror()

	// PackageMirror returns the URL of the package mirror set by
	// SetPackageMirror or an empty string if not previously set
	PackageMirror() string

	// SetPackageManagerSource adds a new repository and optional key to be
	// used as a package source by the system's specific package manager
	AddPackageSource(*packaging.Source)

	// PackageManagerSources returns all sources set with AddPackageSource
	PackageSources() []*packaging.Source

	// AddPackagePreferences adds the necessary options and/or bootcmds to
	// enable the given packaging.PackagePreferences
	AddPackagePreferences(*packaging.PackagePreferences)

	// AddPackage adds a package to be installed on first boot.
	// If any packages are specified, the local package list is refreshed
	// before the installation is executed, regardless of SetSystemUpdate
	AddPackage(string)

	// RemovePackage removes a package from the list of to be installed packages
	// It will not return an error if the package has not been previously added
	RemovePackage(string)

	// Packages returns a list of all packages that will be installed
	Packages() []string

	// AddRunCmd adds a command to be executed on *first* boot
	// It can recieve any number of string arguments, which will be joined into
	// a single command and passed to cloudinit to be executed
	// NOTE: metacharacters will not be escaped
	AddRunCmd(...string)

	// AddScript simply calls AddRunCmd on every string passed to it
	// NOTE: this means that each given string must be a full command plus
	// all of its arguments
	// NOTE: metacharacters will not be escaped
	AddScript(...string)

	// RemoveRunCmd removes the given command from the list of commands to be
	// run. If it has not been previously added, no error occurs
	RemoveRunCmd(string)

	// RunCmds returns all the commands added with AddRunCmd
	RunCmds() []string

	// AddBootCmd adds a command to be executed on *every* boot
	// It can recieve any number of string arguments, which will be joined into
	// a single command.
	// NOTE: metacharecters will not be escaped
	AddBootCmd(...string)

	// RemoveBootCmd removes the given command from the list of commands to be
	// run every boot. If it has not been previously added, no error occurs
	RemoveBootCmd(string)

	// BootCmds returns all the commands added with AddBootCmd
	BootCmds() []string

	// SetDisableEC2Metadata sets whether access to the EC2 metadata service is
	// disabled early in boot via a null route. The default value is false
	// (route del -host 169.254.169.254 reject)
	SetDisableEC2Metadata(bool)

	// UnsetDisableEC2Metadata unsets the value set by SetDisableEC2Metadata,
	// returning it to the cloudinit-defined value of false
	// If the option has not been previously set, no error occurs
	UnsetDisableEC2Metadata()

	// DisableEC2Metadata returns the value set by SetDisableEC2Metadata or
	// false if it has not been previously set
	DisableEC2Metadata() bool

	// SetFinalMessage sets to message that will be written when the system has
	// finished booting for the first time. By default, the message is:
	// "cloud-init boot finished at $TIMESTAMP. Up $UPTIME seconds"
	SetFinalMessage(string)

	// UnsetFinalMessage unsets the value set by SetFinalMessage
	// If it has not been previously set, no error occurs
	UnsetFinalMessage()

	// FinalMessage returns the value set using SetFinalMessage or an empty
	// string if it has not previously been set
	FinalMessage() string

	// SetLocale sets the locale; it defaults to en_US.UTF-8
	SetLocale(string)

	// UnsetLocale unsets the option set by SetLocale, returning it to the
	// cloudinit-defined default of en_US.UTF-8
	// If it has not been previously set, no error occurs
	UnsetLocale()

	// Locale returns the locale set with SetLocale
	// If it has not been previously set, an empty string is returned
	Locale() string

	// AddMount adds takes arguments for installing a mount point in /etc/fstab
	// The options are of the oder and format specific to fstab entries:
	// <device> <mountpoint> <filesystem> <options> <backup setting> <fsck priority>
	AddMount(...string)

	// SetOutput specifies destination for command output
	// Valid values for the kind: "init", "config", "final" and "all"
	// Each of stdout and stderr can take one of the following forms:
	//   >>file
	//       appends to file
	//   >file
	//       overwrites file
	//   |command
	//       pipes to the given command
	SetOutput(OutputKind, string, string)

	// Output returns the destination set by SetOutput for the given OutputKind
	Output(OutputKind) (string, string)

	// AddSSHKey adds a pre-generated ssh key to the server keyring.
	// Valid SSHKeyType options are: rsa_{public,private}, dsa_{public,private}
	// Added keys will be written to /etc/ssh
	// As a result, new random keys are prevented from being generated
	AddSSHKey(SSHKeyType, string)

	// AddSSHAuthorizedKeys adds a set of keys in ssh authorized_keys format
	// that will be added to ~/.ssh/authorized_keys for the configured user (see SetUser).
	AddSSHAuthorizedKeys(string)

	// SetDisableRoot sets whether ssh login to the root account of the new server
	// through the ssh authorized key provided with the config should be disabled
	// This option is set to true (ie. disabled) by default
	SetDisableRoot(bool)

	// UnsetDisable unsets the value set with SetDisableRoot, returning it to the
	// cloudinit-defined default of true
	UnsetDisableRoot()

	// DisableRoot returns the value set by SetDisableRoot or false if the
	// option had not been previously set
	DisableRoot() bool

	// AddTextFile simply issues some AddRunCmd's to set the contents of a
	// given file with the specified file permissions on *first* boot
	// NOTE: if the file already exists, it will be truncated
	AddRunTextFile(string, string, uint)

	// AddBootTextFile simply issues some AddBootCmd's to set the contents of a
	// given file with the specified file permissions on *every* boot
	// NOTE: if the file already exists, it will be truncated
	AddBootTextFile(string, string, uint)

	// AddBinaryFile simply issues some AddRunCmd's to set the binary contents
	// of a given file with the specified file permissions on *first* boot
	// NOTE: if the file already exists, it will be truncated
	AddRunBinaryFile(string, []byte, uint)
}

// New returns a new Config with no options set.
func New() Config {
	return nil
}

type Renderer interface {
	// Mkdir returns an OS specific script for creating a directory
	Mkdir(path string) []string

	// WriteFile returns a command to write data
	WriteFile(filename string, contents string, permission int) []string

	// Render renders the userdata script for a particular OS type
	Render(conf Config) ([]byte, error)

	// FromSlash returns the result of replacing each slash ('/') character
	// in path with a separator character. Multiple slashes are replaced by
	// multiple separators.
	FromSlash(path string) string

	// PathJoin will join a path using OS specific path separator.
	// This works for selected OS instead of current OS
	PathJoin(path ...string) string
}

// NewRenderer returns a Renderer interface for selected series
func NewRenderer(series string) (Renderer, error) {
	operatingSystem, err := version.GetOSFromSeries(series)
	if err != nil {
		return nil, err
	}

	switch operatingSystem {
	case version.Windows:
		return &WindowsRenderer{}, nil
	case version.Ubuntu:
		return &UbuntuRenderer{}, nil
	default:
		return nil, errors.Errorf("No renderer could be found for %s", series)
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
