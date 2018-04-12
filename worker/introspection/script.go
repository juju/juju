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
  juju-introspect --agent=$agent $path
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

juju-cpu-profile () {
  N=30
  if test -n "$1"; then
    N=$1
    shift
  fi
  echo "Sampling CPU for $N seconds." >&2
  jujuMachineOrUnit "debug/pprof/profile?debug=1&seconds=$N" $@
}

juju-heap-profile () {
  jujuMachineOrUnit debug/pprof/heap?debug=1 $@
}

juju-engine-report () {
  jujuMachineOrUnit depengine $@
}

juju-statepool-report () {
  jujuMachineOrUnit statepool $@
}

juju-pubsub-report () {
  jujuMachineOrUnit pubsub $@
}

juju-metrics () {
  jujuMachineOrUnit metrics $@
}

juju-statetracker-report () {
  jujuMachineOrUnit debug/pprof/juju/state/tracker?debug=1 $@
}

export -f jujuAgentCall
export -f jujuMachineAgentName
export -f jujuMachineOrUnit
export -f juju-goroutines
export -f juju-cpu-profile
export -f juju-heap-profile
export -f juju-engine-report
export -f juju-metrics
export -f juju-statepool-report
export -f juju-statetracker-report
export -f juju-pubsub-report
`
