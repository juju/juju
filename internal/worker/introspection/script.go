// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package introspection

import (
	"bytes"
	"context"
	"os"
	"path"
	"runtime"

	"github.com/juju/errors"
)

var (
	// ProfileDir is the directory where the profile script is written.
	ProfileDir         = "/etc/profile.d"
	shellFuncsFilename = "juju-introspection.sh"
)

// FileReaderWriter for mocking file reads and writes.
type FileReaderWriter interface {
	ReadFile(filename string) ([]byte, error)
	WriteFile(filename string, data []byte, perm os.FileMode) error
}

type osFileReaderWriter struct{}

func (osFileReaderWriter) ReadFile(filename string) ([]byte, error) {
	return os.ReadFile(filename)
}

func (osFileReaderWriter) WriteFile(filename string, data []byte, perm os.FileMode) error {
	return os.WriteFile(filename, data, perm)
}

// WriteProfileFunctions writes the shellFuncs below to a file in the
// /etc/profile.d directory so all bash terminals can easily access the
// introspection worker.
// Deprecated: use UpdateProfileFunction with a FileReaderWriter.
func WriteProfileFunctions(profileDir string) error {
	return UpdateProfileFunctions(osFileReaderWriter{}, profileDir)
}

// UpdateProfileFunctions updates the shellFuncs written to disk if they have changed to
// the directory passed in. It allows all bash terminals to easily access the
// introspection worker.
func UpdateProfileFunctions(io FileReaderWriter, profileDir string) error {
	if runtime.GOOS != "linux" {
		logger.Debugf(context.Background(), "skipping profile funcs install")
		return nil
	}

	filename := profileFilename(profileDir)
	shellFuncsBytes := []byte(shellFuncs)
	if currentBytes, err := io.ReadFile(filename); err == nil {
		if bytes.Equal(currentBytes, shellFuncsBytes) {
			// This is here to avoid trying to write this when the file is readonly and already
			// the correct content.
			return nil
		}
	} else if !os.IsNotExist(err) {
		return errors.Annotate(err, "reading old introspection bash funcs")
	}
	if err := io.WriteFile(filename, shellFuncsBytes, 0644); err != nil {
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
  if [ -x "$(which sudo)" ]; then
    sudo juju-introspect --agent=$agent $@
  else
    juju-introspect --agent=$agent $@
  fi
}

juju_machine_agent_name () {
  local machine=$(find /var/lib/juju/agents -maxdepth 1 -type d -name 'machine-*' -printf %f)
  echo $machine
}

juju_controller_agent_name () {
  local controller=$(find /var/lib/juju/agents -maxdepth 1 -type d -name 'controller-*' -printf %f)
  echo $controller
}

juju_application_agent_name () {
  local application=$(find /var/lib/juju/agents -maxdepth 1 -type d -name 'application-*' -printf %f)
  echo $application
}

juju_unit_agent_name () {
  local unit=$(find /var/lib/juju/agents -maxdepth 1 -type d -name 'unit-*' -printf %f)
  echo $unit
}

juju_agent () {
  local agent=$(juju_machine_agent_name)
  if [ -z "$agent" ]; then
    agent=$(juju_controller_agent_name)
  fi
  if [ -z "$agent" ]; then
    agent=$(juju_application_agent_name)
  fi
  if [ -z "$agent" ]; then
    agent=$(juju_unit_agent_name)
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

juju_metrics () {
  juju_agent metrics
}

juju_statetracker_report () {
  juju_agent debug/pprof/juju/state/tracker?debug=1
}

juju_machine_lock () {
  for agent in $(ls /var/lib/juju/agents); do
    juju_agent machinelock --agent=$agent 2> /dev/null
  done
}

juju_unit_status () {
  juju_agent units?action=status
}

juju_db_repl () {
  local type=$1
  local flag
  if [ -z "$type" ]; then
    flag="--machine-id"
  elif [ "$type" = "caas" ]; then
    flag="--controller-id"
  elif [ "$type" = "iaas" ]; then
    flag="--machine-id"
  fi
  local id=$2
  if [ -z "$id" ]; then
    id="0"
  fi

  flag="$flag=$id"

  local agent=$(juju_machine_agent_name)
  if [ -x "$(which sudo)" ]; then
    sudo /var/lib/juju/tools/$agent/jujud db-repl $flag
  else
    /var/lib/juju/tools/$agent/jujud db-repl $flag
  fi
}

juju_object_store_contents () {
  res=$(juju_object_store_contents_ "/var/lib/juju/objectstore")
  res="Model    Name\n$res"
  echo -e $res | column -t
}

juju_object_store_contents_ () {
  target=${1:-.}
  for i in "$target"; do
    if [ -d "$i" ]; then
      for sub in "$i"/*; do
        s=$(juju_object_store_contents_ "$sub")
        if [ -n "$s" ]; then
           echo $s
        fi
      done
    elif [ -f "$i" ]; then
      m=$(dirname "$i" | xargs basename)
      f=$(basename "$i")
      echo "$m  $f\n"
    fi
  done
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
  export -f juju_machine_lock
  export -f juju_unit_status
  export -f juju_db_repl
fi
`
