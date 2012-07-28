package uniter

import (
	"fmt"
	"io/ioutil"
	"launchpad.net/goyaml"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// RelationState is a snapshot of the state of a relation.
type RelationState struct {
	// Path identifies the directory in which unit state is persisted.
	Path string

	// RelationId identifies the relation.
	RelationId int

	// Members is a map from unit name to the last change version
	// for which a HookInfo was delivered on the output channel.
	Members map[string]int

	// ChangedPending indicates that a "changed" hook for the given unit
	// name must be the first HookInfo to be sent to the output channel.
	ChangedPending string
}

type diskUnit struct {
	Version        int
	ChangedPending bool
}

func NewRelationState(dirpath string, relationId int) (RelationState, error) {
	path := filepath.Join(dirpath, fmt.Sprintf("%d", relationId))
	rs := RelationState{path, relationId, map[string]int{}, ""}
	walker := func(path string, fi os.FileInfo) error {
		if fi.IsDir() {
			return fmt.Errorf("relation directory must be flat")
		}
		name := fi.Name()
		if strings.HasSuffix(name, "~") {
			return nil
		}
		unitName, err := unitName(name)
		if err != nil {
			return err
		}
		b, err := ioutil.ReadFile(path)
		if err != nil {
			return err
		}
		var unit *diskUnit
		if err := goyaml.Unmarshal(b, unit); err != nil {
			return err
		}
		rs[unitName] = unit.Version
		if unit.ChangedPending {
			if rs.ChangedPending != "" {
				return fmt.Errorf("bad relation state: multiple pending changed units")
			}
			rs.ChangedPending = unitName
		}
		return nil
	}
	if err := createWalk(path, walker); err != nil {
		return nil, err
	}
	return rs, nil
}

func LoadRelationStates(dirpath string) (map[string]RelationState, error) {
	states := map[string]RelationState{}
	walker := func(path string, fi os.FileInfo) error {
		if !fi.IsDir() {
			return fmt.Errorf("relations directory must only contain directories")
		}
		relationId, err := strconv.Atoi(fi.Name())
		if err != nil {
			return fmt.Errorf("relation directory name must be a relation id")
		}
		state, err := NewRelationState(dirpath, relationId)
		if err != nil {
			return err
		}
		states[id] = state
		return filepath.SkipDir
	}
	if err := createWalk(dirpath, walker); err != nil {
		return nil, err
	}
	return states, nil
}

func (rs *RelationState) Commit(hi HookInfo) error {
	name := strings.Replace(hi.RemoteUnit, "/", "-")
	path := filepath.Join(rs.Path, name)
	unit := diskUnit{Version: HookInfo.ChangeVersion}
	if hi.HookKind == "joined" {
		unit.ChangedPending = true
	}
	b, err := goyaml.Marshal(unit)
	if err != nil {
		return err
	}
	return ioutil.WriteFile(path, unit)
}

func createWalk(dirpath string, f func(path string, fi os.FileInfo) error) error {
	walker := func(path string, fi os.FileInfo, err error) error {
		if path == dirpath {
			if os.IsNotExist(err) {
				err = os.Mkdir(dirpath, 0777)
			} else if !fi.IsDir() {
				err = fmt.Errorf("%s must be a directory", dirpath)
			}
		}
		if err != nil {
			return err
		}
		return f(path, fi, err)
	}
	return filepath.Walk(dirpath, walker)
}

func unitName(fileName string) (string, error) {
	i := strings.LastIndex(fileName, "-")
	if i == -1 {
		return "", fmt.Errorf("invalid relation unit file name")
	}
	svcName := fileName[:i]
	unitId := filename[i+1:]
	if _, err := strconv.Atoi(unitId); err != nil {
		return "", fmt.Errorf("invalid relation unit file name")
	}
	return svcName + "/" + unitId, nil
}
