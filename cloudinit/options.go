// Copyright 2011, 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloudinit

import (
	"fmt"
	"strings"

	"launchpad.net/juju-core/utils"
)

// SetAttr sets an arbitrary attribute in the cloudinit config.
// If value is nil the attribute will be deleted; otherwise
// the value will be marshalled according to the rules
// of the goyaml Marshal function.
func (cfg *Config) SetAttr(name string, value interface{}) {
	cfg.set(name, value != nil, value)
}

// SetUser sets the user name that will be used for some other options.
// The user will be assumed to already exist in the machine image.
// The default user is "ubuntu".
func (cfg *Config) SetUser(user string) {
	cfg.set("user", user != "", user)
}

// SetAptUpgrade sets whether cloud-init runs "apt-get upgrade"
// on first boot.
func (cfg *Config) SetAptUpgrade(yes bool) {
	cfg.set("apt_upgrade", yes, yes)
}

// AptUpgrade returns the value set by SetAptUpgrade, or
// false if no call to SetAptUpgrade has been made.
func (cfg *Config) AptUpgrade() bool {
	update, _ := cfg.attrs["apt_upgrade"].(bool)
	return update
}

// SetUpdate sets whether cloud-init runs "apt-get update"
// on first boot.
func (cfg *Config) SetAptUpdate(yes bool) {
	cfg.set("apt_update", yes, yes)
}

// AptUpdate returns the value set by SetAptUpdate, or
// false if no call to SetAptUpdate has been made.
func (cfg *Config) AptUpdate() bool {
	update, _ := cfg.attrs["apt_update"].(bool)
	return update
}

// SetAptProxy sets the URL to be used as the apt
// proxy.
func (cfg *Config) SetAptProxy(url string) {
	cfg.set("apt_proxy", url != "", url)
}

// SetAptMirror sets the URL to be used as the apt
// mirror site. If not set, the URL is selected based
// on cloud metadata in EC2 - <region>.archive.ubuntu.com
func (cfg *Config) SetAptMirror(url string) {
	cfg.set("apt_mirror", url != "", url)
}

// SetAptPreserveSourcesList sets whether /etc/apt/sources.list
// is overwritten by the mirror. If true, SetAptMirror above
// will have no effect.
func (cfg *Config) SetAptPreserveSourcesList(yes bool) {
	cfg.set("apt_mirror", yes, yes)
}

// AddAptSource adds an apt source. The key holds the
// public key of the source, in the form expected by apt-key(8).
func (cfg *Config) AddAptSource(name, key string) {
	src, _ := cfg.attrs["apt_sources"].([]*AptSource)
	cfg.attrs["apt_sources"] = append(src,
		&AptSource{
			Source: name,
			Key:    key,
		})
}

// AptSources returns the apt sources added with AddAptSource.
func (cfg *Config) AptSources() []*AptSource {
	srcs, _ := cfg.attrs["apt_sources"].([]*AptSource)
	return srcs
}

// SetDebconfSelections provides preseeded debconf answers
// for the boot process. The given answers will be used as input
// to debconf-set-selections(1).
func (cfg *Config) SetDebconfSelections(answers string) {
	cfg.set("debconf_selections", answers != "", answers)
}

// AddPackage adds a package to be installed on first boot.
// If any packages are specified, "apt-get update"
// will be called.
func (cfg *Config) AddPackage(name string) {
	cfg.attrs["packages"] = append(cfg.Packages(), name)
}

// Packages returns a list of packages that will be
// installed on first boot.
func (cfg *Config) Packages() []string {
	pkgs, _ := cfg.attrs["packages"].([]string)
	return pkgs
}

func (cfg *Config) addCmd(kind string, c *command) {
	cfg.attrs[kind] = append(cfg.getCmds(kind), c)
}

func (cfg *Config) getCmds(kind string) []*command {
	cmds, _ := cfg.attrs[kind].([]*command)
	return cmds
}

// getCmdStrings returns a slice of interface{}, where
// each interface's dynamic value is either a string
// or slice of strings.
func (cfg *Config) getCmdStrings(kind string) []interface{} {
	cmds := cfg.getCmds(kind)
	result := make([]interface{}, len(cmds))
	for i, cmd := range cmds {
		if cmd.args != nil {
			result[i] = append([]string{}, cmd.args...)
		} else {
			result[i] = cmd.literal
		}
	}
	return result
}

// BootCmds returns a list of commands added with
// AddBootCmd*.
//
// Each element in the resultant slice is either a
// string or []string, corresponding to how the command
// was added.
func (cfg *Config) BootCmds() []interface{} {
	return cfg.getCmdStrings("bootcmd")
}

// RunCmds returns a list of commands that will be
// run at first boot.
//
// Each element in the resultant slice is either a
// string or []string, corresponding to how the command
// was added.
func (cfg *Config) RunCmds() []interface{} {
	return cfg.getCmdStrings("runcmd")
}

// AddRunCmd adds a command to be executed
// at first boot. The command will be run
// by the shell with any metacharacters retaining
// their special meaning (that is, no quoting takes place).
func (cfg *Config) AddRunCmd(cmd string) {
	cfg.addCmd("runcmd", &command{literal: cmd})
}

// AddRunCmdArgs is like AddRunCmd except that the command
// will be executed with the given arguments properly quoted.
func (cfg *Config) AddRunCmdArgs(args ...string) {
	cfg.addCmd("runcmd", &command{args: args})
}

// AddBootCmd is like AddRunCmd except that the
// command will run very early in the boot process,
// and it will run on every boot, not just the first time.
func (cfg *Config) AddBootCmd(cmd string) {
	cfg.addCmd("bootcmd", &command{literal: cmd})
}

// AddBootCmdArgs is like AddBootCmd except that the command
// will be executed with the given arguments properly quoted.
func (cfg *Config) AddBootCmdArgs(args ...string) {
	cfg.addCmd("bootcmd", &command{args: args})
}

// SetDisableEC2Metadata sets whether access to the
// EC2 metadata service is disabled early in boot
// via a null route ( route del -host 169.254.169.254 reject).
func (cfg *Config) SetDisableEC2Metadata(yes bool) {
	cfg.set("disable_ec2_metadata", yes, yes)
}

// SetFinalMessage sets to message that will be written
// when the system has finished booting for the first time.
// By default, the message is:
// "cloud-init boot finished at $TIMESTAMP. Up $UPTIME seconds".
func (cfg *Config) SetFinalMessage(msg string) {
	cfg.set("final_message", msg != "", msg)
}

// SetLocale sets the locale; it defaults to en_US.UTF-8.
func (cfg *Config) SetLocale(locale string) {
	cfg.set("locale", locale != "", locale)
}

// AddMount adds a mount point. The given
// arguments will be used as a line in /etc/fstab.
func (cfg *Config) AddMount(args ...string) {
	mounts, _ := cfg.attrs["mounts"].([][]string)
	cfg.attrs["mounts"] = append(mounts, args)
}

// OutputKind represents a destination for command output.
type OutputKind string

const (
	OutInit   OutputKind = "init"
	OutConfig OutputKind = "config"
	OutFinal  OutputKind = "final"
	OutAll    OutputKind = "all"
)

// SetOutput specifies destination for command output.
// Valid values for the kind "init", "config", "final" and "all".
// Each of stdout and stderr can take one of the following forms:
//   >>file
//       appends to file
//   >file
//       overwrites file
//   |command
//       pipes to the given command.
func (cfg *Config) SetOutput(kind OutputKind, stdout, stderr string) {
	out, _ := cfg.attrs["output"].(map[string]interface{})
	if out == nil {
		out = make(map[string]interface{})
	}
	if stderr == "" {
		out[string(kind)] = stdout
	} else {
		out[string(kind)] = []string{stdout, stderr}
	}
	cfg.attrs["output"] = out
}

// AddSSHKey adds a pre-generated ssh key to the
// server keyring. Keys that are added like this will be
// written to /etc/ssh and new random keys will not
// be generated.
func (cfg *Config) AddSSHKey(keyType SSHKeyType, keyData string) {
	keys, _ := cfg.attrs["ssh_keys"].(map[SSHKeyType]string)
	if keys == nil {
		keys = make(map[SSHKeyType]string)
		cfg.attrs["ssh_keys"] = keys
	}
	keys[keyType] = keyData
}

// SetDisableRoot sets whether ssh login is disabled to the root account
// via the ssh authorized key associated with the instance metadata.
// It is true by default.
func (cfg *Config) SetDisableRoot(disable bool) {
	// note that disable_root defaults to true, so we include
	// the option only if disable is false.
	cfg.set("disable_root", !disable, disable)
}

// AddSSHAuthorizedKey adds a set of keys in
// ssh authorized_keys format (see ssh(8) for details)
// that will be added to ~/.ssh/authorized_keys for the
// configured user (see SetUser).
func (cfg *Config) AddSSHAuthorizedKeys(keys string) {
	akeys, _ := cfg.attrs["ssh_authorized_keys"].([]string)
	lines := strings.Split(keys, "\n")
	for _, line := range lines {
		if line == "" || line[0] == '#' {
			continue
		}
		akeys = append(akeys, line)
	}
	cfg.attrs["ssh_authorized_keys"] = akeys
}

// AddScripts is a simple shorthand for calling AddRunCmd multiple times.
func (cfg *Config) AddScripts(scripts ...string) {
	for _, s := range scripts {
		cfg.AddRunCmd(s)
	}
}

// AddFile will add multiple run_cmd entries to safely set the contents of a
// specific file to the requested contents.
func (cfg *Config) AddFile(filename, data string, mode uint) {
	// Note: recent versions of cloud-init have the "write_files"
	// module, which can write arbitrary files. We currently support
	// 12.04 LTS, which uses an older version of cloud-init without
	// this module.
	p := shquote(filename)
	// Don't use the shell's echo builtin here; the interpretation
	// of escape sequences differs between shells, namely bash and
	// dash. Instead, we use printf (or we could use /bin/echo).
	cfg.AddScripts(
		fmt.Sprintf("install -m %o /dev/null %s", mode, p),
		fmt.Sprintf(`printf '%%s\n' %s > %s`, shquote(data), p),
	)
}

func shquote(p string) string {
	return utils.ShQuote(p)
}

// TODO
// byobu
// grub_dpkg
// mcollective
// phone_home
// puppet
// resizefs
// rightscale_userdata
// rsyslog
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
