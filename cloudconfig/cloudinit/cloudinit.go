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
	"github.com/juju/utils/packaging/commander"
	"github.com/juju/utils/packaging/configurer"
	"github.com/juju/utils/shell"
)

// Config represents a set of cloud-init configuration options.
type cloudConfig struct {
	// series is the series for which this cloudConfig is made for.
	series string

	// paccmder is the PackageCommander for this cloudConfig.
	paccmder commander.PackageCommander

	// pacconfer is the PackagingConfigurer for this cloudConfig.
	pacconfer configurer.PackagingConfigurer

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

// getPackageCommander implements AdvancedPackagingConfig.
func (cfg *cloudConfig) getPackageCommander() commander.PackageCommander {
	return cfg.paccmder
}

// getPackagingConfigurer implements AdvancedPackagingConfig.
func (cfg *cloudConfig) getPackagingConfigurer() configurer.PackagingConfigurer {
	return cfg.pacconfer
}

// GetSeries implements CloudConfig.
func (cfg *cloudConfig) GetSeries() string {
	return cfg.series
}

// SetAttr implements CloudConfig.
func (cfg *cloudConfig) SetAttr(name string, value interface{}) {
	cfg.attrs[name] = value
}

// UnsetAttr implements CloudConfig.
func (cfg *cloudConfig) UnsetAttr(name string) {
	delete(cfg.attrs, name)
}

// SetUser implements UserConfig.
func (cfg *cloudConfig) SetUser(user string) {
	cfg.SetAttr("user", user)
}

// UnsetUser implements UserConfig.
func (cfg *cloudConfig) UnsetUser() {
	cfg.UnsetAttr("user")
}

// User implements UserConfig.
func (cfg *cloudConfig) User() string {
	user, _ := cfg.attrs["user"].(string)
	return user
}

// SetSystemUpdate implements SystemUpdateConfig.
func (cfg *cloudConfig) SetSystemUpdate(yes bool) {
	cfg.SetAttr("package_update", yes)
}

// UnsetSystemUpdate implements SystemUpdateConfig.
func (cfg *cloudConfig) UnsetSystemUpdate() {
	cfg.UnsetAttr("package_update")
}

// SystemUpdate implements SystemUpdateConfig.
func (cfg *cloudConfig) SystemUpdate() bool {
	update, _ := cfg.attrs["package_update"].(bool)
	return update
}

// SetSystemUpgrade implements SystemUpgradeConfig.
func (cfg *cloudConfig) SetSystemUpgrade(yes bool) {
	cfg.SetAttr("package_upgrade", yes)
}

// UnsetSystemUpgrade implements SystemUpgradeConfig.
func (cfg *cloudConfig) UnsetSystemUpgrade() {
	cfg.UnsetAttr("package_upgrade")
}

// SystemUpgrade implements SystemUpgradeConfig.
func (cfg *cloudConfig) SystemUpgrade() bool {
	upgrade, _ := cfg.attrs["package_upgrade"].(bool)
	return upgrade
}

// AddPackage implements PackagingConfig.
func (cfg *cloudConfig) AddPackage(pack string) {
	cfg.attrs["packages"] = append(cfg.Packages(), pack)
}

// RemovePackage implements PackagingConfig.
func (cfg *cloudConfig) RemovePackage(pack string) {
	cfg.attrs["packages"] = removeStringFromSlice(cfg.Packages(), pack)
}

// Packages implements PackagingConfig.
func (cfg *cloudConfig) Packages() []string {
	packs, _ := cfg.attrs["packages"].([]string)
	return packs
}

// AddRunCmd implements RunCmdsConfig.
func (cfg *cloudConfig) AddRunCmd(args ...string) {
	cfg.attrs["runcmd"] = append(cfg.RunCmds(), strings.Join(args, " "))
}

// AddScripts implements RunCmdsConfig.
func (cfg *cloudConfig) AddScripts(script ...string) {
	for _, line := range script {
		cfg.AddRunCmd(line)
	}
}

// RemoveRunCmd implements RunCmdsConfig.
func (cfg *cloudConfig) RemoveRunCmd(cmd string) {
	cfg.attrs["runcmd"] = removeStringFromSlice(cfg.RunCmds(), cmd)
}

// RunCmds implements RunCmdsConfig.
func (cfg *cloudConfig) RunCmds() []string {
	cmds, _ := cfg.attrs["runcmd"].([]string)
	return cmds
}

// AddBootCmd implements BootCmdsConfig.
func (cfg *cloudConfig) AddBootCmd(args ...string) {
	cfg.attrs["bootcmd"] = append(cfg.BootCmds(), strings.Join(args, " "))
}

// RemoveBootCmd implements BootCmdsConfig.
func (cfg *cloudConfig) RemoveBootCmd(cmd string) {
	cfg.attrs["bootcmd"] = removeStringFromSlice(cfg.BootCmds(), cmd)
}

// BootCmds implements BootCmdsConfig.
func (cfg *cloudConfig) BootCmds() []string {
	cmds, _ := cfg.attrs["bootcmd"].([]string)
	return cmds
}

// SetDisableEC2Metadata implements EC2MetadataConfig.
func (cfg *cloudConfig) SetDisableEC2Metadata(set bool) {
	cfg.SetAttr("disable_ec2_metadata", set)
}

// UnsetDisableEC2Metadata implements EC2MetadataConfig.
func (cfg *cloudConfig) UnsetDisableEC2Metadata() {
	cfg.UnsetAttr("disable_ec2_metadata")
}

// DisableEC2Metadata implements EC2MetadataConfig.
func (cfg *cloudConfig) DisableEC2Metadata() bool {
	disEC2, _ := cfg.attrs["disable_ec2_metadata"].(bool)
	return disEC2
}

// SetFinalMessage implements FinalMessageConfig.
func (cfg *cloudConfig) SetFinalMessage(message string) {
	cfg.SetAttr("final_message", message)
}

// UnsetFinalMessage implements FinalMessageConfig.
func (cfg *cloudConfig) UnsetFinalMessage() {
	cfg.UnsetAttr("final_message")
}

// FinalMessage implements FinalMessageConfig.
func (cfg *cloudConfig) FinalMessage() string {
	message, _ := cfg.attrs["final_message"].(string)
	return message
}

// SetLocale implements LocaleConfig.
func (cfg *cloudConfig) SetLocale(locale string) {
	cfg.SetAttr("locale", locale)
}

// UnsetLocale implements LocaleConfig.
func (cfg *cloudConfig) UnsetLocale() {
	cfg.UnsetAttr("locale")
}

// Locale implements LocaleConfig.
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

// SetOutput implements OutputConfig.
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

// Output implements OutputConfig.
func (cfg *cloudConfig) Output(kind OutputKind) (stdout, stderr string) {
	if out, ok := cfg.attrs["output"].(map[string]interface{}); ok {
		switch out := out[string(kind)].(type) {
		case string:
			//return out, out
			stdout = out
		case []string:
			//return out[0], out[1]
			stdout, stderr = out[0], out[1]
		}
	}

	//return "", ""
	return stdout, stderr
}

// AddSSHKey implements SSHKeyConfig
func (cfg *cloudConfig) AddSSHKey(keyType SSHKeyType, key string) {
	keys, _ := cfg.attrs["ssh_keys"].(map[SSHKeyType]string)
	if keys == nil {
		keys = make(map[SSHKeyType]string)
		cfg.SetAttr("ssh_keys", keys)
	}

	keys[keyType] = key
}

// AddSSHAuthorizedKeys implements SSHKeysConfig.
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

// SetDisableRoot implements RootUserConfig.
func (cfg *cloudConfig) SetDisableRoot(disable bool) {
	cfg.SetAttr("disable_root", disable)
}

// UnsetDisableRoot implements RootUserConfig.
func (cfg *cloudConfig) UnsetDisableRoot() {
	cfg.UnsetAttr("disable_root")
}

// DisableRoot implements RootUserConfig.
func (cfg *cloudConfig) DisableRoot() bool {
	disable, _ := cfg.attrs["disable_root"].(bool)
	return disable
}

// AddRunTextFile implements WrittenFilesConfig.
func (cfg *cloudConfig) AddRunTextFile(filename, contents string, perm uint) {
	cfg.AddScripts(addFileCmds(filename, []byte(contents), perm, false)...)
}

// AddBootTextFile implements WrittenFilesConfig.
func (cfg *cloudConfig) AddBootTextFile(filename, contents string, perm uint) {
	for _, cmd := range addFileCmds(filename, []byte(contents), perm, false) {
		cfg.AddBootCmd(cmd)
	}
}

// AddRunBinaryFile implements WrittenFilesConfig.
func (cfg *cloudConfig) AddRunBinaryFile(filename string, data []byte, mode uint) {
	cfg.AddScripts(addFileCmds(filename, data, mode, true)...)
}

// ShellRenderer implements RenderConfig.
func (cfg *cloudConfig) ShellRenderer() shell.Renderer {
	return cfg.renderer
}

// RequiresCloudArchiveCloudTools implements AdvancedPackagingConfig.
func (cfg *cloudConfig) RequiresCloudArchiveCloudTools() bool {
	return configurer.SeriesRequiresCloudArchiveTools(cfg.series)
}
