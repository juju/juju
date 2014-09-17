package rebootstate

import (
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/juju/errors"

	"github.com/juju/juju/agent"
)

var RebootStateFile = filepath.Join(agent.DefaultDataDir, "reboot-state.txt")

func New() error {
	if IsPresent() {
		return errors.Errorf("state file %s already exists", RebootStateFile)
	}
	err := ioutil.WriteFile(RebootStateFile, []byte(""), 400)
	if err != nil {
		return err
	}
	return nil
}

func Remove() error {
	if _, err := os.Stat(RebootStateFile); err == nil {
		err = os.Remove(RebootStateFile)
		return err
	}
	return nil
}

func IsPresent() bool {
	if _, err := os.Stat(RebootStateFile); err != nil {
		return false
	}
	return true
}
