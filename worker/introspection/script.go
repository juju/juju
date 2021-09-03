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
	// ProfileDir is the directory where the profile script is written.
	ProfileDir         = "/etc/profile.d"
	shellFuncsFilename = "juju-introspection.sh"
)

// WriteProfileFunctions writes the shellFuncs below to a file in the
// /etc/profile.d directory so all bash terminals can easily access the
// introspection worker.
func WriteProfileFunctions(profileDir string) error {
	if runtime.GOOS != "linux" {
		logger.Debugf("skipping profile funcs install")
		return nil
	}
	filename := profileFilename(profileDir)
	if err := ioutil.WriteFile(filename, []byte(shellFuncs), 0644); err != nil {
		return errors.Annotate(err, "writing introspection bash funcs")
	}
	return nil
}

func profileFilename(profileDir string) string {
	return path.Join(profileDir, shellFuncsFilename)
}

// WARNING: This code MUST be compatible with all POSIX shells including
// /bin/sh and MUST NOT include bash-isms.
const shellFuncs = `
juju_agent_call () {
  local agent=$1
  shift
  juju-introspect --agent=$agent $@
}

juju_machine_agent_name () {
  local machine=$(find /var/lib/juju/agents -type d -name 'machine*' -printf %f)
  echo $machine
}

juju_controller_agent_name () {
  local controller=$(find /var/lib/juju/agents -type d -name 'controller*' -printf %f)
  echo $controller
}

juju_application_agent_name () {
  local application=$(find /var/lib/juju/agents -type d -name 'application*' -printf %f)
  echo $application
}

juju_agent () {
  local agent=$(juju_machine_agent_name)
  if [ -z "$agent" ]; then
    agent=$(juju_controller_agent_name)
  fi
  if [ -z "$agent" ]; then
    agent=$(juju_application_agent_name)
  fi
  juju_agent_call $agent $@
}

juju_goroutines () {
  juju_agent debug/pprof/goroutine?debug=1
}

juju_cpu_profile () {
  N=30
  if test -n "$1"; then
    N=$1
    shift
  fi
  echo "Sampling CPU for $N seconds." >&2
  juju_agent "debug/pprof/profile?debug=1&seconds=$N"
}

juju_heap_profile () {
  juju_agent debug/pprof/heap?debug=1
}

juju_engine_report () {
  juju_agent depengine
}

juju_statepool_report () {
  juju_agent statepool
}

juju_pubsub_report () {
  juju_agent pubsub
}

juju_metrics () {
  juju_agent metrics
}

juju_presence_report () {
  juju_agent presence
}

juju_statetracker_report () {
  juju_agent debug/pprof/juju/state/tracker?debug=1
}

juju_machine_lock () {
  for agent in $(ls /var/lib/juju/agents); do
    juju_agent machinelock $agent 2> /dev/null
  done
}

juju_unit_status () {
  juju_agent units?action=status
}

juju_stop_unit () {
  # This requires some arguments.
  if [ "$#" -lt 1 ]; then
    echo "usage: juju_stop_unit <unit-name> [<unit-name>...]"
    return 1
  fi
  args=""
  for i in "$@"; do
    args="$args unit=$i"
  done
  juju_agent --post units action=stop $args
}

juju_start_unit () {
  # This requires some arguments.
  if [ "$#" -lt 1 ]; then
    echo "usage: juju_start_unit <unit-name> [<unit-name>...]"
    return 1
  fi
  args=""
  for i in "$@"; do
    args="$args unit=$i"
  done
  juju_agent --post units action=start $args
}

juju_leases () {
  # This requires some arguments.
  local query
  local model
  if [ "$1" = "-m" ]; then
    if [ "$#" -lt 2 ]; then
      echo "usage: juju_leases [-m <partial-model-uuid>] [<partial-app-name>...]"
      return 1
    fi
    shift
    model=$1
    shift
    query="&model=$model"
  fi
  for i in "$@"; do
    query="$query&app=$i"
  done
  if [ -z "$query" ]; then
    juju_agent leases
  else
    juju_agent "leases?q=y&$query"
  fi
}

juju_revoke_lease () {
  # This requires some arguments.
  local model
  local lease
  local ns

  while [[ $# -gt 0 ]]
  do
    key="$1"

    case $key in
      -m|--model)
        model="$2"; shift; shift
      ;;
      -n|--ns)
        ns="$2"; shift; shift
      ;;
      -l|--lease)
        lease="$2"; shift; shift
      ;;
      *)
        echo "usage: juju_revoke_lease -m <model-uuid> -l <lease> [-n <namespace>]"
        return 1
      ;;
    esac
  done

  if [ -z "$model" ] | [ -z "$lease" ]; then
    echo "usage: juju_revoke_lease -m <model-uuid> -l <lease> [-n <namespace>]"
    return 1
  fi  

  juju_agent --post leases/revoke model="$model" lease="$lease" ns="$ns"
}


# This asks for the command of the current pid.
# Can't use $0 nor $SHELL due to this being wrong in various situations.
shell=$(ps -p "$$" -o comm --no-headers)
if [ "$shell" = "bash" ]; then
  export -f juju_agent_call
  export -f juju_machine_agent_name
  export -f juju_controller_agent_name
  export -f juju_application_agent_name
  export -f juju_agent
  export -f juju_goroutines
  export -f juju_cpu_profile
  export -f juju_heap_profile
  export -f juju_engine_report
  export -f juju_metrics
  export -f juju_statepool_report
  export -f juju_statetracker_report
  export -f juju_pubsub_report
  export -f juju_presence_report
  export -f juju_machine_lock
  export -f juju_unit_status
  export -f juju_start_unit
  export -f juju_stop_unit
  export -f juju_leases
  export -f juju_revoke_lease
fi
`
