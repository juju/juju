package cloudinit

func (cfg *Config) SetUser(x string) {
	// common
	cfg.set("user", x != "", x)
}

func (cfg *Config) SetAptUpgrade(yes bool) {
	// apt_update_upgrade
	cfg.set("apt_upgrade", yes, yes)
}

func (cfg *Config) SetAptUpdate(yes bool) {
	// apt_update_upgrade
	cfg.set("apt_update", yes, yes)
}

func (cfg *Config) SetAptMirror(url string) {
	// apt_update_upgrade
	cfg.set("apt_mirror", url != "", url)
}

func (cfg *Config) SetAptPreserveSourcesList(yes bool) {
	// apt_update_upgrade
	cfg.set("apt_mirror", yes, yes)
}

func (cfg *Config) SetAptOldMirror(url string) {
	// apt_update_upgrade
	cfg.set("apt_old_mirror", url != "", url)
}

func (cfg *Config) AddAptSource(name, key string) {
	// apt_update_upgrade
	src, _ := cfg.attrs["apt_sources"].([]*source)
	cfg.attrs["apt_sources"] = append(src,
		&source{
			Source: name,
			Key: key,
		})
}

func (cfg *Config) AddAptSourceWithKeyId(name, keyId, keyServer string) {
	// apt_update_upgrade
	src, _ := cfg.attrs["apt_sources"].([]*source)
	cfg.attrs["apt_sources"] = append(src,
		&source{
			Source:    name,
			KeyId:     keyId,
			KeyServer: keyServer,
		})
}

func (cfg *Config) SetDebConfSelections(x bool) {
	// apt_update_upgrade
	cfg.set("debconf_selections", x, x)
}

func (cfg *Config) AddPackage(x string) {
	// apt_update_upgrade
	pkgs, _ := cfg.attrs["packages"].([]string)
	cfg.attrs["packages"] = append(pkgs, x)
}

func (cfg *Config) addBootCmd(c *command) {
	// bootcmd
	cmds, _ := cfg.attrs["bootcmd"].([]*command)
	cfg.attrs["bootcmd"] = append(cmds, c)
}

func (cfg *Config) AddBootCmd(cmd string) {
	// bootcmd
	cfg.addBootCmd(&command{literal: cmd})
}

func (cfg *Config) AddBootCmdArgs(args ...string) {
	// bootcmd
	cfg.addBootCmd(&command{args: args})
}

func (cfg *Config) SetDisableEC2Metadata(x bool) {
	// disable_ec2_metadata
	cfg.set("disable_ec2_metadata", x, x)
}

func (cfg *Config) SetFinalMessage(x string) {
	// final_message
	cfg.set("final_message", x != "", x)
}

func (cfg *Config) SetLocale(x string) {
	// locale
	cfg.set("locale", x != "", x)
}

func (cfg *Config) AddMount(x ...string) {
	// mounts
	mounts, _ := cfg.attrs["mounts"].([][]string)
	cfg.attrs["mounts"] = append(mounts, x)
}

// SetOutput specifies destination for command output.
// Valid values for the kind "init", "config", "final" and "all".
// Each of stdout and stderr can take one of the following forms:
// >>file
//	appends to file
// >file
//	overwrites file
// |command
//	pipes to the given command.
// If stderr is "&1" or empty, it will be directed to the same
// place as Stdout.
func (cfg *Config) SetOutput(kind, stdout, stderr string) {
	out, _ := cfg.attrs["output"].(map[string] interface{})
	if out == nil {
		out = make(map[string]interface{})
	}
	if stderr == "" {
		out[kind] = stdout
	} else {
		out[kind] = []string{stdout, stderr}
	}
	cfg.attrs["output"] = out
}

func (cfg *Config) AddSSHKey(alg Alg, private bool, keyData string) {
	keys, _ := cfg.attrs["ssh_keys"].([]key)
	cfg.attrs["ssh_keys"] = append(keys, key{alg, private, keyData})
}

func (cfg *Config) SetDisableRoot(x bool) {
	// ssh
	// note that disable_root defaults to true, so we include
	// the option only if x is false.
	cfg.set("disable_root", !x, x)
}

func (cfg *Config) AddSSHAuthorizedKey(x string) {
	// ssh
	keys, _ := cfg.attrs["ssh_authorized_keys"].([]string)
	cfg.attrs["ssh_authorized_keys"] = append(keys, x)
}

func (cfg *Config) addRunCmd(c *command) {
	// ssh
	cmds, _ := cfg.attrs["runcmd"].([]*command)
	cfg.attrs["runcmd"] = append(cmds, c)
}

func (cfg *Config) AddRunCmd(cmd string) {
	// runcmd
	cfg.addRunCmd(&command{literal: cmd})
}

func (cfg *Config) AddRunCmdArgs(args ...string) {
	// runcmd
	cfg.addRunCmd(&command{args: args})
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
