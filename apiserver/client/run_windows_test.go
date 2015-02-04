// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build windows

package client_test

var expectedCommand = []string{
	"juju-run --no-context 'hostname'\r\n",
	"juju-run magic/0 'hostname'\r\n",
	"juju-run magic/1 'hostname'\r\n",
}

var echoInputShowArgs = `@echo off
echo %* 1>&2

setlocal
for /F "tokens=*" %%a in ('more') do (
  echo %%a
)
`

var echoInput = `
@echo off
setlocal
for /F "tokens=*" %%a in ('more') do (
  echo %%a
)
`
