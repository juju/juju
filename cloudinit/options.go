package cloudinit

func User(x string) Option {
	// common
	return Option{"user", maybe(x != "", x)}
}

func AptUpgrade(yes bool) Option {
	// apt_update_upgrade
	return Option{"apt_upgrade", maybe(yes, yes)}
}

func AptUpdate(yes bool) Option {
	// apt_update_upgrade
	return Option{"apt_update", maybe(yes, yes)}
}

func AptMirror(url string) Option {
	// apt_update_upgrade
	return Option{"apt_mirror", maybe(url != "", url)}
}

func AptPreserveSourcesList(yes bool) Option {
	// apt_update_upgrade
	return Option{"apt_mirror", maybe(yes, yes)}
}

func AptOldMirror(url string) Option {
	// apt_update_upgrade
	return Option{"apt_old_mirror", maybe(url != "", url)}
}

func AptSources(x ...*Source) Option {
	// apt_update_upgrade
	if len(x) == 0 {
		return Option{"apt_sources", nil}
	}
	ss := make([]*source, len(x))
	for i, s := range x {
		ss[i] = &s.source
	}
	return Option{"apt_sources", ss}
}

func DebConfSelections(x bool) Option {
	// apt_update_upgrade
	return Option{"debconf_selections", maybe(x, x)}
}

func Packages(x ...string) Option {
	// apt_update_upgrade
	return Option{"packages", maybe(len(x) > 0, x)}
}

func BootCmd(x ...*Command) Option {
	// bootcmd
	return Option{"bootcmd", maybe(len(x) > 0, x)}
}

func DisableEC2Metadata(x bool) Option {
	// disable_ec2_metadata
	return Option{"disable_ec2_metadata", maybe(x, x)}
}

func FinalMessage(x string) Option {
	// final_message
	return Option{"final_message", maybe(x != "", x)}
}

func Locale(x string) Option {
	// locale
	return Option{"locale", maybe(x != "", x)}
}

func Mounts(x [][]string) Option {
	// mounts
	return Option{"mounts", maybe(len(x) > 0, x)}
}

// Output specifies destination for command output.
// Valid values for the string keys are "init", "config", "final" and "all".
func Output(specs map[string]OutputSpec) Option {
	return Option{"output", maybe(len(specs) > 0, specs)}
}

func SSHKeys(x []Key) Option {
	// ssh
	return Option{"ssh_keys", maybe(len(x) > 0, x)}
}

func DisableRoot(x bool) Option {
	// ssh
	// note that disable_root defaults to true, so we include
	// the option only if x is false.
	return Option{"disable_root", maybe(!x, x)}
}

func SSHAuthorizedKeys(x ...string) Option {
	// ssh
	return Option{"ssh_authorized_keys", maybe(len(x) > 0, x)}
}

func RunCmd(x ...*Command) Option {
	// runcmd
	return Option{"runcmd", maybe(len(x) > 0, x)}
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
