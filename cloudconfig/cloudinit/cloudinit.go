// Copyright 2015 Canonical Ltd.
// Copyright 2015 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

package cloudinit

import (
	"strings"

	"github.com/juju/utils/packaging/commands"
	"github.com/juju/utils/packaging/config"
	"github.com/juju/utils/shell"
	"github.com/juju/utils/ssh"
)

// cloudConfig represents a set of cloud-init configuration options.
type cloudConfig struct {
	// series is the series for which this cloudConfig is made for.
	series string

	// paccmder is the PackageCommander for this cloudConfig.
	paccmder commands.PackageCommander

	// pacconfer is the PackagingConfigurer for this cloudConfig.
	pacconfer config.PackagingConfigurer

	// renderer is the shell Renderer for this cloudConfig.
	renderer shell.Renderer

	// attrs is the map of options set on this cloudConfig.
	// main attributes used in the options map and their corresponding types:
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
	// apt_sources			[]*packaging.Source
	//
	// instead, the following corresponding options are used temporarily,
	// but are translated to runcmds and removed right before rendering:
	// package_proxy
	// package_mirror
	// package_sources
	// package_preferences
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

// getPackageCommander is defined on the AdvancedPackagingConfig interface.
func (cfg *cloudConfig) getPackageCommander() commands.PackageCommander {
	return cfg.paccmder
}

// getPackagingConfigurer is defined on the AdvancedPackagingConfig interface.
func (cfg *cloudConfig) getPackagingConfigurer() config.PackagingConfigurer {
	return cfg.pacconfer
}

// GetSeries is defined on the CloudConfig interface.
func (cfg *cloudConfig) GetSeries() string {
	return cfg.series
}

// SetAttr is defined on the CloudConfig interface.
func (cfg *cloudConfig) SetAttr(name string, value interface{}) {
	cfg.attrs[name] = value
}

// UnsetAttr is defined on the CloudConfig interface.
func (cfg *cloudConfig) UnsetAttr(name string) {
	delete(cfg.attrs, name)
}

func annotateKeys(rawKeys string) []string {
	cfgKeys := []string{}
	keys := ssh.SplitAuthorisedKeys(rawKeys)
	for _, key := range keys {
		// ensure our keys have "Juju:" prepended to differentiate
		// Juju-managed keys and externally added ones
		jujuKey := ssh.EnsureJujuComment(key)
		cfgKeys = append(cfgKeys, jujuKey)
	}
	return cfgKeys
}

// AddUser is defined on the UsersConfig interface.
func (cfg *cloudConfig) AddUser(user *User) {
	users, _ := cfg.attrs["users"].([]map[string]interface{})
	newUser := map[string]interface{}{
		"name":        user.Name,
		"lock_passwd": true,
	}
	if user.Groups != nil {
		newUser["groups"] = user.Groups
	}
	if user.Shell != "" {
		newUser["shell"] = user.Shell
	}
	if user.SSHAuthorizedKeys != "" {
		newUser["ssh-authorized-keys"] = annotateKeys(user.SSHAuthorizedKeys)
	}
	if user.Sudo != nil {
		newUser["sudo"] = user.Sudo
	}
	cfg.SetAttr("users", append(users, newUser))
}

// UnsetUsers is defined on the UsersConfig interface.
func (cfg *cloudConfig) UnsetUsers() {
	cfg.UnsetAttr("users")
}

// SetSystemUpdate is defined on the SystemUpdateConfig interface.
func (cfg *cloudConfig) SetSystemUpdate(yes bool) {
	cfg.SetAttr("package_update", yes)
}

// UnsetSystemUpdate is defined on the SystemUpdateConfig interface.
func (cfg *cloudConfig) UnsetSystemUpdate() {
	cfg.UnsetAttr("package_update")
}

// SystemUpdate is defined on the SystemUpdateConfig interface.
func (cfg *cloudConfig) SystemUpdate() bool {
	update, _ := cfg.attrs["package_update"].(bool)
	return update
}

// SetSystemUpgrade is defined on the SystemUpgradeConfig interface.
func (cfg *cloudConfig) SetSystemUpgrade(yes bool) {
	cfg.SetAttr("package_upgrade", yes)
}

// UnsetSystemUpgrade is defined on the SystemUpgradeConfig interface.
func (cfg *cloudConfig) UnsetSystemUpgrade() {
	cfg.UnsetAttr("package_upgrade")
}

// SystemUpgrade is defined on the SystemUpgradeConfig interface.
func (cfg *cloudConfig) SystemUpgrade() bool {
	upgrade, _ := cfg.attrs["package_upgrade"].(bool)
	return upgrade
}

// AddPackage is defined on the PackagingConfig interface.
func (cfg *cloudConfig) AddPackage(pack string) {
	cfg.attrs["packages"] = append(cfg.Packages(), pack)
}

// RemovePackage is defined on the PackagingConfig interface.
func (cfg *cloudConfig) RemovePackage(pack string) {
	cfg.attrs["packages"] = removeStringFromSlice(cfg.Packages(), pack)
}

// Packages is defined on the PackagingConfig interface.
func (cfg *cloudConfig) Packages() []string {
	packs, _ := cfg.attrs["packages"].([]string)
	return packs
}

// AddRunCmd is defined on the RunCmdsConfig interface.
func (cfg *cloudConfig) AddRunCmd(args ...string) {
	cfg.attrs["runcmd"] = append(cfg.RunCmds(), strings.Join(args, " "))
}

// AddScripts is defined on the RunCmdsConfig interface.
func (cfg *cloudConfig) AddScripts(script ...string) {
	for _, line := range script {
		cfg.AddRunCmd(line)
	}
}

// RemoveRunCmd is defined on the RunCmdsConfig interface.
func (cfg *cloudConfig) RemoveRunCmd(cmd string) {
	cfg.attrs["runcmd"] = removeStringFromSlice(cfg.RunCmds(), cmd)
}

// RunCmds is defined on the RunCmdsConfig interface.
func (cfg *cloudConfig) RunCmds() []string {
	cmds, _ := cfg.attrs["runcmd"].([]string)
	return cmds
}

// AddBootCmd is defined on the BootCmdsConfig interface.
func (cfg *cloudConfig) AddBootCmd(args ...string) {
	cfg.attrs["bootcmd"] = append(cfg.BootCmds(), strings.Join(args, " "))
}

// RemoveBootCmd is defined on the BootCmdsConfig interface.
func (cfg *cloudConfig) RemoveBootCmd(cmd string) {
	cfg.attrs["bootcmd"] = removeStringFromSlice(cfg.BootCmds(), cmd)
}

// BootCmds is defined on the BootCmdsConfig interface.
func (cfg *cloudConfig) BootCmds() []string {
	cmds, _ := cfg.attrs["bootcmd"].([]string)
	return cmds
}

// SetDisableEC2Metadata is defined on the EC2MetadataConfig interface.
func (cfg *cloudConfig) SetDisableEC2Metadata(set bool) {
	cfg.SetAttr("disable_ec2_metadata", set)
}

// UnsetDisableEC2Metadata is defined on the EC2MetadataConfig interface.
func (cfg *cloudConfig) UnsetDisableEC2Metadata() {
	cfg.UnsetAttr("disable_ec2_metadata")
}

// DisableEC2Metadata is defined on the EC2MetadataConfig interface.
func (cfg *cloudConfig) DisableEC2Metadata() bool {
	disEC2, _ := cfg.attrs["disable_ec2_metadata"].(bool)
	return disEC2
}

// SetFinalMessage is defined on the FinalMessageConfig interface.
func (cfg *cloudConfig) SetFinalMessage(message string) {
	cfg.SetAttr("final_message", message)
}

// UnsetFinalMessage is defined on the FinalMessageConfig interface.
func (cfg *cloudConfig) UnsetFinalMessage() {
	cfg.UnsetAttr("final_message")
}

// FinalMessage is defined on the FinalMessageConfig interface.
func (cfg *cloudConfig) FinalMessage() string {
	message, _ := cfg.attrs["final_message"].(string)
	return message
}

// SetLocale is defined on the LocaleConfig interface.
func (cfg *cloudConfig) SetLocale(locale string) {
	cfg.SetAttr("locale", locale)
}

// UnsetLocale is defined on the LocaleConfig interface.
func (cfg *cloudConfig) UnsetLocale() {
	cfg.UnsetAttr("locale")
}

// Locale is defined on the LocaleConfig interface.
func (cfg *cloudConfig) Locale() string {
	locale, _ := cfg.attrs["locale"].(string)
	return locale
}

// AddMount adds takes arguments for installing a mount point in /etc/fstab
// The options are of the order and format specific to fstab entries:
// <device> <mountpoint> <filesystem> <options> <backup setting> <fsck priority>
func (cfg *cloudConfig) AddMount(mount ...string) {
	mounts, _ := cfg.attrs["mounts"].([][]string)
	cfg.SetAttr("mounts", append(mounts, mount))
}

// SetOutput is defined on the OutputConfig interface.
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

// Output is defined on the OutputConfig interface.
func (cfg *cloudConfig) Output(kind OutputKind) (stdout, stderr string) {
	if out, ok := cfg.attrs["output"].(map[string]interface{}); ok {
		switch out := out[string(kind)].(type) {
		case string:
			stdout = out
		case []string:
			stdout, stderr = out[0], out[1]
		}
	}

	return stdout, stderr
}

// SetSSHAuthorizedKeys is defined on the SSHAuthorizedKeysConfig interface.
func (cfg *cloudConfig) SetSSHAuthorizedKeys(rawKeys string) {
	keys := annotateKeys(rawKeys)
	if len(keys) != 0 {
		cfg.SetAttr("ssh_authorized_keys", keys)
	} else {
		cfg.UnsetAttr("ssh_authorized_keys")
	}
}

// SetDisableRoot is defined on the RootUserConfig interface.
func (cfg *cloudConfig) SetDisableRoot(disable bool) {
	cfg.SetAttr("disable_root", disable)
}

// UnsetDisableRoot is defined on the RootUserConfig interface.
func (cfg *cloudConfig) UnsetDisableRoot() {
	cfg.UnsetAttr("disable_root")
}

// DisableRoot is defined on the RootUserConfig interface.
func (cfg *cloudConfig) DisableRoot() bool {
	disable, _ := cfg.attrs["disable_root"].(bool)
	return disable
}

// AddRunTextFile is defined on the WrittenFilesConfig interface.
func (cfg *cloudConfig) AddRunTextFile(filename, contents string, perm uint) {
	cfg.AddScripts(addFileCmds(filename, []byte(contents), perm, false)...)
}

// AddBootTextFile is defined on the WrittenFilesConfig interface.
func (cfg *cloudConfig) AddBootTextFile(filename, contents string, perm uint) {
	for _, cmd := range addFileCmds(filename, []byte(contents), perm, false) {
		cfg.AddBootCmd(cmd)
	}
}

// AddRunBinaryFile is defined on the WrittenFilesConfig interface.
func (cfg *cloudConfig) AddRunBinaryFile(filename string, data []byte, mode uint) {
	cfg.AddScripts(addFileCmds(filename, data, mode, true)...)
}

// ShellRenderer is defined on the RenderConfig interface.
func (cfg *cloudConfig) ShellRenderer() shell.Renderer {
	return cfg.renderer
}

// RequiresCloudArchiveCloudTools is defined on the AdvancedPackagingConfig
// interface.
func (cfg *cloudConfig) RequiresCloudArchiveCloudTools() bool {
	return config.SeriesRequiresCloudArchiveTools(cfg.series)
}
