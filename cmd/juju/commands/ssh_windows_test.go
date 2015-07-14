// Copyright 2014 Canonical Ltd.
// Copyright 2014 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

// +build windows

package commands

// Commands to patch
var patchedCommands = []string{"scp.cmd", "ssh.cmd"}

// fakecommand outputs its arguments to stdout for verification
var fakecommand = `@echo off
setlocal enabledelayedexpansion
set list=%1
set argCount=0
for %%x in (%*) do (
set /A argCount+=1
set "argVec[!argCount!]=%%~x"
)
for /L %%i in (2,1,%argCount%) do set list=!list! !argVec[%%i]!
echo %list%
`
