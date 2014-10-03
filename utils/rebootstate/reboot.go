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
	_, err := os.Stat(RebootStateFile)
	if err != nil && os.IsNotExist(err) {
		return nil
	} else {
		return err
	}
	err = os.Remove(RebootStateFile)
	return err
}

func IsPresent() (bool, error) {
	_, err := os.Stat(RebootStateFile)
	if err != nil && os.IsNotExist(err) {
		return false, nil
	} else {
		return false, err
	}
	return true, nil
}
