// Copyright 2015 Canonical Ltd.
// Copyright 2015 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

// The cloudinit package implements a way of creating
// a cloud-init configuration file.
// See https://help.ubuntu.com/community/CloudInit.
package cloudinit

import (
	"strings"

	"github.com/juju/juju/utils/ssh"
)

// Config represents a set of cloud-init configuration options.
type cloudConfig struct {
	// main used attributes:
	//
	// user					string
	// package_update		bool
	// package_upgrade		bool
	// packages				[]string
	// runcmd				[]string
	// bootcmd				[]string
	// disable_ec2_metadata	bool
	// final_message		string
	// locale				string
	// mounts				[][]string
	// output				map[OutputKind]string
	// shh_keys				map[SSHKeyType]string
	// ssh_authorized_keys	[]string
	// disable_root			bool
	//
	// used only for Ubuntu but implemented as runcmds on CentOS:
	// apt_proxy			string
	// apt_mirror			string/bool
	// apt_sources			[]*AptSource
	//
	// old TODO's:
	// byobu
	// grub_dpkg
	// mcollective
	// phone_home
	// puppet
	// resizefs
	// rightscale_userdata
	// scripts_per_boot
	// scripts_per_instance
	// scripts_per_once
	// scripts_user
	// set_hostname
	// set_passwords
	// ssh_import_id
	// timezone
	// update_etc_hosts
	// update_hostname
	attrs map[string]interface{}
}

// SetAttr sets the attribute with the given name to the given value in the
// cloudinit config. In the resulting config file, the attribute will be
// marshalled according to the goyaml Marshal function
func (cfg *cloudConfig) SetAttr(name string, value interface{}) {
	cfg.attrs[name] = value
}

// UnsetAttr unsets the attribute with the given name from the cloudinit config
// If the attribute has not been previously set, no error occurs
func (cfg *cloudConfig) UnsetAttr(name string) {
	delete(cfg.attrs, name)
}

// getAttrs returns the attribute map of this particular cloud config
func (cfg *cloudConfig) getAttrs() map[string]interface{} {
	return cfg.attrs
}

// SetUser sets the username that will be written in the cloudinit config
// NOTE: the user must exist beforehand, as no steps to create it are taken
// If not set, the cloudinit-default value of "ubuntu" is used
func (cfg *cloudConfig) SetUser(user string) {
	cfg.SetAttr("user", user)
}

// UnsetUser unsets the user cloudinit config attribute
// If the user has not been previously set, no error occurs
func (cfg *cloudConfig) UnsetUser() {
	cfg.UnsetAttr("user")
}

// SetSystemUpdate sets wether the system should refresh the local package
// database on first boot. This option is active in cloudinit by default
func (cfg *cloudConfig) SetSystemUpdate(yes bool) {
	cfg.SetAttr("package_update", yes)
}

// UnsetSystemUpdate unsets the system upgrade cloudinit config option,
// returning it to the cloudinit-defined default of it being active
func (cfg *cloudConfig) UnsetSystemUpdate() {
	cfg.UnsetAttr("package_update")
}

// SystemUpdate returns the value set by SetSystemUpdate or false if not called
// NOTE: even if it was not manually set, this option will be active by default
func (cfg *cloudConfig) SystemUpdate() bool {
	update, _ := cfg.attrs["package_update"].(bool)
	return update
}

// SetSystemUpgrade sets wether the system should upgrade all packages with
// available newer versions on first boot.
func (cfg *cloudConfig) SetSystemUpgrade(yes bool) {
	cfg.SetAttr("package_upgrade", yes)
}

// UnsetSystemUpgrade unsets the system upgrade cloudinit config option
func (cfg *cloudConfig) UnsetSystemUpgrade() {
	cfg.UnsetAttr("package_upgrade")
}

// SystemUpgrade returns the value set by SetSystemUpgrade or the default of
// false if it has not been called
func (cfg *cloudConfig) SystemUpgrade() bool {
	upgrade, _ := cfg.attrs["package_upgrade"].(bool)
	return upgrade
}

// AddPackage adds a package to be installed on first boot
// NOTE: will refresh the local package list by default if set
func (cfg *cloudConfig) AddPackage(pack string) {
	cfg.attrs["packages"] = append(cfg.Packages(), pack)
}

// RemovePackage removes the given entry from the to be installed package list
// It will not return an error if the package has not been previously added
func (cfg *cloudConfig) RemovePackage(pack string) {
	cfg.attrs["packages"] = removeStringFromSlice(cfg.Packages(), pack)
}

// Packages returns a list of all packages set using AddPackage
// or AddPackageWithVersion to be installed on first boot
func (cfg *cloudConfig) Packages() []string {
	packs, _ := cfg.attrs["packages"].([]string)
	return packs
}

// AddRunCmd adds a standard shell command to be executed on *first* boot
// It can take any number of arguments which will be joined into a single
// command to be passed to cloudinit
// NOTE: metacharacters will not be escaped
func (cfg *cloudConfig) AddRunCmd(args ...string) {
	cfg.attrs["runcmd"] = append(cfg.RunCmds(), strings.Join(args, " "))
}

// AddScript simply calls AddRunCmd on all given lines of script
func (cfg *cloudConfig) AddScript(script ...string) {
	for _, line := range script {
		cfg.AddRunCmd(line)
	}
}

// RemoveRunCmd removes the given command from the list of commands to be run
func (cfg *cloudConfig) RemoveRunCmd(cmd string) {
	cfg.attrs["runcmd"] = removeStringFromSlice(cfg.RunCmds(), cmd)
}

// RunCmds returns a list of all the commands to be run on *first* boot,
// as they were set using AddRunCmd
func (cfg *cloudConfig) RunCmds() []string {
	cmds, _ := cfg.attrs["runcmd"].([]string)
	return cmds
}

// AddBootCmd adds a standard shell command to be executed *every* boot
// It can take any number of arguments which will be joined into a single
// command to be passed to cloudinit and be properly handled
func (cfg *cloudConfig) AddBootCmd(args ...string) {
	cfg.attrs["bootcmd"] = append(cfg.BootCmds(), strings.Join(args, " "))
}

// RemoveBootCmd removes the given command from the list of commands to be run
// each boot process. If the command is not present, no error occurs
func (cfg *cloudConfig) RemoveBootCmd(cmd string) {
	cfg.attrs["bootcmd"] = removeStringFromSlice(cfg.BootCmds(), cmd)
}

// BootCmds returns a list of all command to be run *every* boot, in the order
// they were set with AddBootCmd
func (cfg *cloudConfig) BootCmds() []string {
	cmds, _ := cfg.attrs["bootcmd"].([]string)
	return cmds
}

// SetDisableEC2Metadata tells cloudinit to disable access to the EC2 metadata
// service during early boot via a null route. The default value is false
// (route del -host 169.254.169.254 reject)
func (cfg *cloudConfig) SetDisableEC2Metadata(set bool) {
	cfg.SetAttr("disable_ec2_metadata", set)
}

// UnsetDisableEC2Metadata unsets the value set by SetDisableEC2Metadata
func (cfg *cloudConfig) UnsetDisableEC2Metadata() {
	cfg.UnsetAttr("disable_ec2_metadata")
}

// DisableEC2Metadata returns the value set by SetDisableEC2Metadata or false
// is it has not been previously set
func (cfg *cloudConfig) DisableEC2Metadata() bool {
	disEC2, _ := cfg.attrs["disable_ec2_metadata"].(bool)
	return disEC2
}

// SetFinalMessage sets the message that will be written after first boot
// The default value is:
// "cloud-init boot finished at $TIMESTAMP. Up $UPTIME seconds"
func (cfg *cloudConfig) SetFinalMessage(message string) {
	cfg.SetAttr("final_message", message)
}

// UnsetFinalMessage unsets the value set with SetFinalMessage
func (cfg *cloudConfig) UnsetFinalMessage() {
	cfg.UnsetAttr("final_message")
}

// FinalMessage returns the value set by SetFinalMessage or an empty string if
// has not been previously set
func (cfg *cloudConfig) FinalMessage() string {
	message, _ := cfg.attrs["final_message"].(string)
	return message
}

// SetLocale sets the locale to be used, the default being en_US.UTF-8
func (cfg *cloudConfig) SetLocale(locale string) {
	cfg.SetAttr("locale", locale)
}

// UnsetLocale unsets the option set using SetLocale, returning it to the
// cloudinit-defined default of "en_US.UTF-8"
// If the locale had not been previously set, no error is returned
func (cfg *cloudConfig) UnsetLocale() {
	cfg.UnsetAttr("locale")
}

// Locale returns the locale set using SetLocale
// If it has not been previously set, an empty string is returned
func (cfg *cloudConfig) Locale() string {
	locale, _ := cfg.attrs["locale"].(string)
	return locale
}

// AddMount adds takes arguments for installing a mount point in /etc/fstab
// The options are of the oder and format specific to fstab entries:
// <device> <mountpoint> <filesystem> <options> <backup setting> <fsck priority>
func (cfg *cloudConfig) AddMount(mount ...string) {
	mounts, _ := cfg.attrs["mounts"].([][]string)
	cfg.SetAttr("mounts", append(mounts, mount))
}

// SetOutput specifies the destinations of standard output and standard error of
// particular kinds of an output stream
// Valid values include:
//	- init:		the output of cloudinit itself
//	- config:	cloud-config caused output
// 	- final:	the final output of cloudinit (plus that set with SetFinalMessage)
// 	- all:		all of the above
// Both stdout and stderr can take the following forms:
// 	- > file:	write to given file. Will truncate of file exists
// 	- >>file:	append to given file
// 	- | command:	pipe output to given command
func (cfg *cloudConfig) SetOutput(kind OutputKind, stdout, stderr string) {
	out, _ := cfg.attrs["output"].(map[string]interface{})
	if out == nil {
		out = make(map[string]interface{})
	}

	if stderr == "" {
		out[string(kind)] = stdout
	} else {
		out[string(kind)] = []string{stdout, stderr}
	}

	cfg.SetAttr("output", out)
}

// Output returns the output destination set using SetOutput for the given
// OutputKind. If it was not previously set, empty strings will be returned
func (cfg *cloudConfig) Output(kind OutputKind) (string, string) {
	if out, ok := cfg.attrs["output"].(map[string]interface{}); ok {
		switch out := out[string(kind)].(type) {
		case string:
			return out, out
		case []string:
			return out[0], out[1]
		}
	}

	return "", ""
}

// AddSSHKey adds an already generated ssh key to the new server's keyring
// Valid SSHKeyType's include: rsa_{public,private}, dsa_{public,private}
// The keys will be written to /etc/ssh and no new ones will be generated
func (cfg *cloudConfig) AddSSHKey(keyType SSHKeyType, key string) {
	keys, _ := cfg.attrs["ssh_keys"].(map[SSHKeyType]string)
	if keys == nil {
		keys = make(map[SSHKeyType]string)
	}

	keys[keyType] = key
	cfg.SetAttr("ssh_Keys", keys)
}

// AddSSHAuthorizedKeys adds a set of keys to ~/.ssh/authorized_keys for the
// user set with SetUser.
func (cfg *cloudConfig) AddSSHAuthorizedKeys(rawKeys string) {
	cfgKeys, _ := cfg.attrs["ssh_authorized_keys"].([]string)
	keys := ssh.SplitAuthorisedKeys(rawKeys)
	for _, key := range keys {
		// ensure our keys have "Juju:" prepended in order to differenciate
		// Juju-managed keys and externally added ones
		jujuKey := ssh.EnsureJujuComment(key)

		cfgKeys = append(cfgKeys, jujuKey)
	}
	cfg.SetAttr("ssh_authorized_keys", cfgKeys)
}

// SetDisableRoot sets whether ssh login to the root account of the new server
// through the ssh authorized key provided with the config should be disabled
// This option is set to true (ie. disabled) by default
func (cfg *cloudConfig) SetDisableRoot(disable bool) {
	cfg.SetAttr("disable_root", disable)
}

// UnsetDisable unsets the value set with SetDisableRoot, returning it to the
// cloudinit-defined default of true
func (cfg *cloudConfig) UnsetDisableRoot() {
	cfg.UnsetAttr("disable_root")
}

// DisableRoot returns the value set by SetDisableRoot or false if it has not
// been previously set
func (cfg *cloudConfig) DisableRoot() bool {
	disable, _ := cfg.attrs["disable_root"].(bool)
	return disable
}

// AddRunTextFile will add multiple commands with AddRunCmd to safely set the
// contents of a specified file with the given permissions on *first* boot
func (cfg *cloudConfig) AddRunTextFile(filename, contents string, perm uint) {
	cfg.AddScript(addFileCmds(filename, []byte(contents), perm, false)...)
}

// AddBootTextFile will add multiple commands with AddBootCmd to safely set the
// contents of a specified file with the given permissions on *every* boot
func (cfg *cloudConfig) AddBootTextFile(filename, contents string, perm uint) {
	for _, cmd := range addFileCmds(filename, []byte(contents), perm, false) {
		cfg.AddBootCmd(cmd)
	}
}

// AddRunBinaryFile will add multiple commands with AddRunCmd to safely set the
// contents of a specified binary file with the given permissions on *first* boot
func (cfg *cloudConfig) AddRunBinaryFile(filename string, data []byte, mode uint) {
	cfg.AddScript(addFileCmds(filename, data, mode, true)...)
}
