// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package introspection

import (
	"io/ioutil"
	"path"
	"runtime"

	"github.com/juju/errors"
)

var (
	profileDir        = "/etc/profile.d"
	bashFuncsFilename = "juju-introspection.sh"
)

// WriteProfileFunctions writes the bashFuncs below to a file in the
// /etc/profile.d directory so all bash terminals can easily access the
// introspection worker.
func WriteProfileFunctions() error {
	if runtime.GOOS != "linux" {
		logger.Debugf("skipping profile funcs install")
		return nil
	}
	filename := profileFilename()
	if err := ioutil.WriteFile(filename, []byte(bashFuncs), 0644); err != nil {
		return errors.Annotate(err, "writing introspection bash funcs")
	}
	return nil
}

func profileFilename() string {
	return path.Join(profileDir, bashFuncsFilename)
}

const bashFuncs = `
jujuAgentCall () {
  local agent=$1
  shift
  local path=
  for i in "$@"; do
    path="$path/$i"
  done
  echo -e "GET $path HTTP/1.0\r\n" | socat abstract-connect:jujud-$agent STDIO
}

jujuMachineAgentName () {
  local machine=` + "`ls -d /var/lib/juju/agents/machine*`" + `
  machine=` + "`basename $machine`" + `
  echo $machine
}

jujuMachineOrUnit () {
  # First arg is the path, second is optional agent name.
  if [ "$#" -gt 2 ]; then
    echo "expected no args (for machine agent) or one (unit agent)"
    return 1
  fi
  local agent=$(jujuMachineAgentName)
  if [ "$#" -eq 2 ]; then
    agent=$2
  fi
  jujuAgentCall $agent $1
}

juju-goroutines () {
  jujuMachineOrUnit debug/pprof/goroutine?debug=1 $@
}

juju-heap-profile () {
  jujuMachineOrUnit debug/pprof/heap?debug=1 $@
}

juju-engine-report () {
  jujuMachineOrUnit depengine/ $@
}

export -f jujuAgentCall
export -f jujuMachineAgentName
export -f jujuMachineOrUnit
export -f juju-goroutines
export -f juju-heap-profile
export -f juju-engine-report
`
