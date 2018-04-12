package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

func main() {
	if err := run(os.Args); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) != 2 {
		return fmt.Errorf("expected exactly one argument, the action name")
	}
	switch args[1] {
	case "fill-unit":
		return fillUnit()
	case "fill-machine":
		return fillMachine()
	case "unit-size":
		return unitLogSizes()
	case "machine-size":
		return machineLogSizes()
	default:
		return fmt.Errorf("unknown action: %q", args[1])
	}
}

func fillUnit() error {
	return fillLog(os.Stdout)
}

func unitLogSizes() error {
	return writeSizes("/var/log/juju/unit-fill-logs*.log")
}

func machineLogSizes() error {
	return writeSizes("/var/log/juju/machine-*.log")
}

func fillMachine() (err error) {
	machine, err := getMachine()
	if err != nil {
		return err
	}
	svcname := fmt.Sprintf("jujud-machine-%d", machine)
	out, err := exec.Command("service", svcname, "stop").CombinedOutput()
	if err != nil {
		return fmt.Errorf("error stopping machine agent %q: %s", svcname, out)
	}
	defer func() {
		out, err2 := exec.Command("service", svcname, "start").CombinedOutput()
		if err2 == nil {
			return
		}
		if err == nil {
			// function error is currently nil, so overwrite with this one.
			err = fmt.Errorf("error starting machine agent %q: %s", svcname, out)
			return
		}
		// function error is non-nil, so can't overwrite, just print.
		fmt.Printf("error starting machine agent %q: %s", svcname, out)
	}()
	logname := fmt.Sprintf("/var/log/juju/machine-%d.log", machine)
	f, err := os.OpenFile(logname, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("failed to open machine log file: %v", err)
	}
	defer f.Close()
	return fillLog(f)
}

func fillLog(w io.Writer) error {
	megs, err := getMegs()
	if err != nil {
		return err
	}
	bytes := megs * 1024 * 1024
	total := 0

	for total < bytes {
		// technically, the log file will be bigger than asked for, since it
		// prepends a bunch of stuff to each log call, but this guarantees we've
		// put *at least* this much data in the log, which should guarantee a
		// rotation.
		n, err := fmt.Fprintln(w, lorem)
		if err != nil {
			return fmt.Errorf("error writing to log: %s", err)
		}
		total += n
	}
	return nil
}

func writeSizes(glob string) error {
	paths, err := filepath.Glob(glob)
	if err != nil {
		return fmt.Errorf("error getting logs for %q: %s", glob, err)
	}

	// go through the list in reverse, since the primary log file is always last,
	// but it's a lot more convenient for parsing if it's first in the output.
	for i, j := len(paths)-1, 0; i >= 0; i-- {
		path := paths[i]
		info, err := os.Stat(path)
		if err != nil {
			return fmt.Errorf("error stating log %q: %s", path, err)
		}
		name := fmt.Sprintf("result-map.log%d.name=%s", j, path)
		size := fmt.Sprintf("result-map.log%d.size=%d", j, info.Size()/1024/1024)
		out, err := exec.Command("action-set", name, size).CombinedOutput()
		if err != nil {
			return fmt.Errorf("error calling action-set: %s", out)
		}
		j++
	}

	return nil
}

func getMegs() (int, error) {
	return getInt("megs")
}

func getMachine() (int, error) {
	return getInt("machine")
}

func getInt(name string) (int, error) {
	out, err := exec.Command("action-get", name).CombinedOutput()
	if err != nil {
		fmt.Fprintln(os.Stderr, out)
		return 0, fmt.Errorf("error calling action-get: %s", err)
	}
	// for some reason the output always comes with a /n at the end, so just
	// trim it.
	return strconv.Atoi(strings.TrimSpace(string(out)))
}

const lorem = `Lorem ipsum dolor sit amet, consectetur adipiscing elit, sed do eiusmod tempor incididunt ut labore et dolore magna aliqua. Ut enim ad minim veniam, quis nostrud exercitation ullamco laboris nisi ut aliquip ex ea commodo consequat. Duis aute irure dolor in reprehenderit in voluptate velit esse cillum dolore eu fugiat nulla pariatur. Excepteur sint occaecat cupidatat non proident, sunt in culpa qui officia deserunt mollit anim id est laborum. Lorem ipsum dolor sit amet, consectetur adipiscing elit, sed do eiusmod tempor incididunt ut labore et dolore magna aliqua. Ut enim ad minim veniam, quis nostrud exercitation ullamco laboris nisi ut aliquip ex ea commodo consequat. Duis aute irure dolor in reprehenderit in voluptate velit esse cillum dolore eu fugiat nulla pariatur. Excepteur sint occaecat cupidatat non proident, sunt in culpa qui officia deserunt mollit anim id est laborum. Lorem ipsum dolor sit amet, consectetur adipiscing elit, sed do eiusmod tempor incididunt ut labore et dolore magna aliqua. Ut enim ad minim veniam, quis nostrud exercitation ullamco laboris nisi ut aliquip ex ea commodo consequat. Duis aute irure dolor in reprehenderit in voluptate velit esse cillum dolore eu fugiat nulla pariatur. Excepteur sint occaecat cupidatat non proident, sunt in culpa qui officia deserunt mollit anim id est laborum. Lorem ipsum dolor sit amet, consectetur adipiscing elit, sed do eiusmod tempor incididunt ut labore et dolore magna aliqua. Ut enim ad minim veniam, quis nostrud exercitation ullamco laboris nisi ut aliquip ex ea commodo consequat. Duis aute irure dolor in reprehenderit in voluptate velit esse cillum dolore eu fugiat nulla pariatur. Excepteur sint occaecat cupidatat non proident, sunt in culpa qui officia deserunt mollit anim id est laborum.`
