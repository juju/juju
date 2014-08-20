// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package networker

import (
	"path/filepath"

	"github.com/juju/names"
)

const (
	DoNone   = doNone
	DoWrite  = doWrite
	DoRemove = doRemove
)

var (
	ConfigDirName           = configDirName
	ConfigFileName          = configFileName
	ConfigSubDirName        = configSubDirName
	ReadAll                 = (*ConfigFiles).readAll
	WriteOrRemove           = (ConfigFiles).writeOrRemove
	FixMAAS                 = (ConfigFiles).fixMAAS
	IfaceConfigFileName     = ifaceConfigFileName
	SplitByInterfaces       = splitByInterfaces
	SourceCommentAndCommand = sourceCommentAndCommand
)

func IsRunningInLXC(machineTag names.MachineTag) bool {
	nw := &networker{tag: machineTag}
	return nw.isRunningInLXC()
}

func ChangeConfigDirName(dirName string) {
	configDirName = dirName
	configFileName = filepath.Join(configDirName, "interfaces")
	configSubDirName = filepath.Join(configDirName, "interfaces.d")
	ConfigDirName = configDirName
	ConfigFileName = configFileName
	ConfigSubDirName = configSubDirName
}
