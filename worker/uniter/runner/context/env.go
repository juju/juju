// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package context

import (
	"path/filepath"

	jujuos "github.com/juju/os"
	"github.com/juju/os/series"
)

// GetEnvFunc is passed to OSDependentEnvVars and called
// when environment variables need to be appended or otherwise
// based off existing variables.
type GetEnvFunc func(key string) string

// OSDependentEnvVars returns the OS-dependent environment variables that
// should be set for a hook context.
func OSDependentEnvVars(paths Paths, getEnv GetEnvFunc) []string {
	switch jujuos.HostOS() {
	case jujuos.Windows:
		return windowsEnv(paths, getEnv)
	case jujuos.Ubuntu:
		return ubuntuEnv(paths, getEnv)
	case jujuos.CentOS:
		return centosEnv(paths, getEnv)
	case jujuos.OpenSUSE:
		return opensuseEnv(paths, getEnv)
	case jujuos.GenericLinux:
		return genericLinuxEnv(paths, getEnv)
	}
	return nil
}

func appendPath(paths Paths, getEnv GetEnvFunc) []string {
	return []string{
		"PATH=" + paths.GetToolsDir() + ":" + getEnv("PATH"),
	}
}

func ubuntuEnv(paths Paths, getEnv GetEnvFunc) []string {
	path := appendPath(paths, getEnv)
	env := []string{
		"APT_LISTCHANGES_FRONTEND=none",
		"DEBIAN_FRONTEND=noninteractive",
		"LANG=C.UTF-8",
	}

	env = append(env, path...)

	if series.MustHostSeries() == "trusty" {
		// Trusty is in ESM at the time of writing and it does not have patch 20150502 for ncurses 5.9
		// with terminal definitions for "tmux" and "tmux-256color"
		env = append(env, "TERM=screen-256color")
	} else {
		env = append(env, "TERM=tmux-256color")
	}

	return env
}

func centosEnv(paths Paths, getEnv GetEnvFunc) []string {
	path := appendPath(paths, getEnv)

	env := []string{
		"LANG=C.UTF-8",
	}

	env = append(env, path...)

	// versions older than 7 are not supported and centos7 does not have patch 20150502 for ncurses 5.9
	// with terminal definitions for "tmux" and "tmux-256color"
	if series.MustHostSeries() == "centos7" {
		env = append(env, "TERM=screen-256color")
	} else {
		env = append(env, "TERM=tmux-256color")
	}

	return env
}

func opensuseEnv(paths Paths, getEnv GetEnvFunc) []string {
	path := appendPath(paths, getEnv)

	env := []string{
		"LANG=C.UTF-8",
	}

	env = append(env, path...)

	// OpenSUSE 42 does not include patch 20150502 for ncurses 5.9 with
	// with terminal definitions for "tmux" and "tmux-256color"
	if series.MustHostSeries() == "opensuseleap" {
		env = append(env, "TERM=screen-256color")
	} else {
		env = append(env, "TERM=tmux-256color")
	}

	return env
}

func genericLinuxEnv(paths Paths, getEnv GetEnvFunc) []string {
	path := appendPath(paths, getEnv)

	env := []string{
		"LANG=C.UTF-8",
	}

	env = append(env, path...)

	// use the "screen" terminal definition (added to ncurses in 1997) on a generic Linux to avoid
	// any ncurses version discovery code. tmux documentation suggests that the "screen" terminal is supported.
	env = append(env, "TERM=screen")

	return env
}

// windowsEnv adds windows specific environment variables. PSModulePath
// helps hooks use normal imports instead of dot sourcing modules
// its a convenience variable. The PATH variable delimiter is
// a semicolon instead of a colon
func windowsEnv(paths Paths, getEnv GetEnvFunc) []string {
	charmDir := paths.GetCharmDir()
	charmModules := filepath.Join(charmDir, "lib", "Modules")
	return []string{
		"Path=" + paths.GetToolsDir() + ";" + getEnv("Path"),
		"PSModulePath=" + getEnv("PSModulePath") + ";" + charmModules,
	}
}
