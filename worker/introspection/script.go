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
juju_agent_call () {
  local agent=$1
  shift
  local path=
  for i in "$@"; do
    path="$path/$i"
  done
  juju-introspect --agent=$agent $path
}

juju_machine_agent_name () {
  local machine=$(ls -d /var/lib/juju/agents/machine*)
  machine=$(basename $machine)
  echo $machine
}

juju_machine_or_unit () {
  # First arg is the path, second is optional agent name.
  if [ "$#" -gt 2 ]; then
    echo "expected no args (for machine agent) or one (unit agent)"
    return 1
  fi
  local agent=$(juju_machine_agent_name)
  if [ "$#" -eq 2 ]; then
    agent=$2
  fi
  juju_agent_call $agent $1
}

juju_goroutines () {
  juju_machine_or_unit debug/pprof/goroutine?debug=1 $@
}

juju_cpu_profile () {
  N=30
  if test -n "$1"; then
    N=$1
    shift
  fi
  echo "Sampling CPU for $N seconds." >&2
  juju_machine_or_unit "debug/pprof/profile?debug=1&seconds=$N" $@
}

juju_heap_profile () {
  juju_machine_or_unit debug/pprof/heap?debug=1 $@
}

juju_engine_report () {
  juju_machine_or_unit depengine $@
}

juju_statepool_report () {
  juju_machine_or_unit statepool $@
}

juju_pubsub_report () {
  juju_machine_or_unit pubsub $@
}

juju_metrics () {
  juju_machine_or_unit metrics $@
}

juju_statetracker_report () {
  juju_machine_or_unit debug/pprof/juju/state/tracker?debug=1 $@
}

juju_machine_lock () {
  for agent in $(ls /var/lib/juju/agents); do
    juju_machine_or_unit machinelock $agent 2> /dev/null
  done
}

# This asks for the command of the current pid.
# Can't use $0 nor $SHELL due to this being wrong in various situations.
shell=$(ps -p "$$" -o comm --no-headers)
if [ "$shell" = "bash" ]; then
  export -f juju_agent_call
  export -f juju_machine_agent_name
  export -f juju_machine_or_unit
  export -f juju_goroutines
  export -f juju_cpu_profile
  export -f juju_heap_profile
  export -f juju_engine_report
  export -f juju_metrics
  export -f juju_statepool_report
  export -f juju_statetracker_report
  export -f juju_pubsub_report
  export -f juju_machine_lock
fi
`
