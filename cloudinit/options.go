package cloudinit

func (cfg *Config) SetUser(user string) {
	cfg.set("user", user != "", user)
}

func (cfg *Config) SetAptUpgrade(yes bool) {
	cfg.set("apt_upgrade", yes, yes)
}

func (cfg *Config) SetAptUpdate(yes bool) {
	cfg.set("apt_update", yes, yes)
}

func (cfg *Config) SetAptMirror(url string) {
	cfg.set("apt_mirror", url != "", url)
}

func (cfg *Config) SetAptPreserveSourcesList(yes bool) {
	cfg.set("apt_mirror", yes, yes)
}

func (cfg *Config) SetAptOldMirror(url string) {
	cfg.set("apt_old_mirror", url != "", url)
}

func (cfg *Config) AddAptSource(name, key string) {
	src, _ := cfg.attrs["apt_sources"].([]*source)
	cfg.attrs["apt_sources"] = append(src,
		&source{
			Source: name,
			Key:    key,
		})
}

func (cfg *Config) AddAptSourceWithKeyId(name, keyId, keyServer string) {
	src, _ := cfg.attrs["apt_sources"].([]*source)
	cfg.attrs["apt_sources"] = append(src,
		&source{
			Source:    name,
			KeyId:     keyId,
			KeyServer: keyServer,
		})
}

func (cfg *Config) SetDebconfSelections(yes bool) {
	cfg.set("debconf_selections", yes, yes)
}

func (cfg *Config) AddPackage(name string) {
	pkgs, _ := cfg.attrs["packages"].([]string)
	cfg.attrs["packages"] = append(pkgs, name)
}

func (cfg *Config) addBootCmd(c *command) {
	cmds, _ := cfg.attrs["bootcmd"].([]*command)
	cfg.attrs["bootcmd"] = append(cmds, c)
}

func (cfg *Config) AddBootCmd(cmd string) {
	cfg.addBootCmd(&command{literal: cmd})
}

func (cfg *Config) AddBootCmdArgs(args ...string) {
	cfg.addBootCmd(&command{args: args})
}

func (cfg *Config) SetDisableEC2Metadata(yes bool) {
	cfg.set("disable_ec2_metadata", yes, yes)
}

func (cfg *Config) SetFinalMessage(msg string) {
	cfg.set("final_message", msg != "", msg)
}

func (cfg *Config) SetLocale(locale string) {
	cfg.set("locale", locale != "", locale)
}

func (cfg *Config) AddMount(mountArgs ...string) {
	mounts, _ := cfg.attrs["mounts"].([][]string)
	cfg.attrs["mounts"] = append(mounts, mountArgs)
}

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
// >>file
//	appends to file
// >file
//	overwrites file
// |command
//	pipes to the given command.
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

func (cfg *Config) AddSSHKey(alg Alg, private bool, keyData string) {
	keys, _ := cfg.attrs["ssh_keys"].([]key)
	cfg.attrs["ssh_keys"] = append(keys, key{alg, private, keyData})
}

func (cfg *Config) SetDisableRoot(yes bool) {
	// note that disable_root defaults to true, so we include
	// the option only if yes is false.
	cfg.set("disable_root", !yes, yes)
}

func (cfg *Config) AddSSHAuthorizedKey(yes string) {
	keys, _ := cfg.attrs["ssh_authorized_keys"].([]string)
	cfg.attrs["ssh_authorized_keys"] = append(keys, yes)
}

func (cfg *Config) addRunCmd(c *command) {
	cmds, _ := cfg.attrs["runcmd"].([]*command)
	cfg.attrs["runcmd"] = append(cmds, c)
}

func (cfg *Config) AddRunCmd(cmd string) {
	cfg.addRunCmd(&command{literal: cmd})
}

func (cfg *Config) AddRunCmdArgs(args ...string) {
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
