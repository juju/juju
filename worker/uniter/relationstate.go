package uniter

import (
	"fmt"
	"io/ioutil"
	"launchpad.net/goyaml"
	"launchpad.net/juju-core/schema"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// AllRelationStates loads and returns every RelationState persisted directly
// inside the directory at the supplied path. It is an error for the directory
// to contain anything other than valid persisted RelationStates, identified by
// relation id. If the directory does not exist, it will be created.
func AllRelationStates(dirpath string) (map[int]*RelationState, error) {
	states := map[int]*RelationState{}
	walker := func(path string, fi os.FileInfo) error {
		relationId, err := strconv.Atoi(fi.Name())
		if err != nil {
			return fmt.Errorf("%q is not a valid relation id", fi.Name())
		}
		if !fi.IsDir() {
			return fmt.Errorf("relation %d is not a directory", relationId)
		}
		state, err := NewRelationState(dirpath, relationId)
		if err != nil {
			return err
		}
		states[relationId] = state
		return filepath.SkipDir
	}
	if err := createWalk(dirpath, walker); err != nil {
		return nil, fmt.Errorf("cannot load relations state from %s: %v", dirpath, err)
	}
	return states, nil
}

// RelationState is a filesystem-backed representation of the state of a
// relation. Concurrent modifications to the underlying state directory
// may cause RelationState instances to exhibit undefined behaviour.
type RelationState struct {
	// Path identifies the directory holding persistent state.
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

// checker validates the format of relation unit state files.
var checker = schema.StrictFieldMap(
	schema.Fields{"change-version": schema.Int(), "changed-pending": schema.Bool()},
	schema.Defaults{"changed-pending": false},
)

// NewRelationState loads a RelationState from the subdirectory of dirpath named
// for the supplied RelationId. The directory must not contain anything other than
// valid relation unit files, as written by Commit(), and files with names ending
// with "~", which will be ignored. If the directory does not exist, it will be
// created.
func NewRelationState(dirpath string, relationId int) (*RelationState, error) {
	path := filepath.Join(dirpath, fmt.Sprintf("%d", relationId))
	rs := &RelationState{path, relationId, map[string]int{}, ""}
	walker := func(path string, fi os.FileInfo) error {
		if fi.IsDir() {
			return fmt.Errorf("directory must be flat")
		}
		name := fi.Name()
		if strings.HasSuffix(name, "~") {
			return nil
		}
		unitname, err := unitname(name)
		if err != nil {
			return err
		}
		data, err := ioutil.ReadFile(path)
		if err != nil {
			return err
		}
		m := map[string]interface{}{}
		if err := goyaml.Unmarshal(data, m); err != nil {
			return err
		} else {
			m1, err := checker.Coerce(m, nil)
			if err != nil {
				return fmt.Errorf("invalid unit file %q: %v", name, err)
			}
			m = m1.(map[string]interface{})
		}
		rs.Members[unitname] = int(m["change-version"].(int64))
		if m["changed-pending"].(bool) {
			if rs.ChangedPending != "" {
				return fmt.Errorf("%q and %q both have pending changed hooks", rs.ChangedPending, unitname)
			}
			rs.ChangedPending = unitname
		}
		return nil
	}
	if err := createWalk(path, walker); err != nil {
		return nil, fmt.Errorf("cannot load relation state from %s: %v", path, err)
	}
	return rs, nil
}

// Validate returns an error if hi does not represent a valid change to the
// RelationState. It should be called before running any relation hook, to
// ensure that the system's guarantees about hook execution order hold true.
func (rs *RelationState) Validate(hi HookInfo) (err error) {
	defer func() {
		if err != nil {
			err = fmt.Errorf("invalid %q for %q: %v", hi.HookKind, hi.RemoteUnit, err)
		}
	}()
	if hi.RelationId != rs.RelationId {
		return fmt.Errorf("expected relation %d, got relation %d", rs.RelationId, hi.RelationId)
	}
	unit, hook := hi.RemoteUnit, hi.HookKind
	if rs.ChangedPending != "" {
		if unit != rs.ChangedPending || hook != "changed" {
			return fmt.Errorf(`expected "changed" for %q`, rs.ChangedPending)
		}
	} else if _, joined := rs.Members[unit]; joined && hook == "joined" {
		return fmt.Errorf(`unit already joined`)
	} else if !joined && hook != "joined" {
		return fmt.Errorf("unit not joined")
	}
	return nil
}

// Commit atomically writes to disk the relation state change embodied by
// the supplied HookInfo. It should be called after the supplied HookInfo
// has been successfully run. If the change is not valid, or cannot be
// written, an error is returned and the change is neither persisted on
// disk nor changed in memory.
func (rs *RelationState) Commit(hi HookInfo) (err error) {
	if err = rs.Validate(hi); err != nil {
		return err
	}
	defer func() {
		if err != nil {
			err = fmt.Errorf("cannot commit %q for %q: %v", hi.HookKind, hi.RemoteUnit, err)
		}
	}()
	name := strings.Replace(hi.RemoteUnit, "/", "-", -1)
	path := filepath.Join(rs.Path, name)
	if hi.HookKind == "departed" {
		if err = os.Remove(path); err != nil {
			return err
		}
		delete(rs.Members, hi.RemoteUnit)
		return nil
	}
	unit := struct {
		ChangeVersion  int  `yaml:"change-version"`
		ChangedPending bool `yaml:"changed-pending,omitempty"`
	}{
		hi.ChangeVersion,
		hi.HookKind == "joined",
	}
	data, err := goyaml.Marshal(unit)
	if err != nil {
		return err
	}
	if err = ioutil.WriteFile(path+"~", data, 0777); err != nil {
		return err
	}
	if err = os.Rename(path+"~", path); err != nil {
		return err
	}
	rs.Members[hi.RemoteUnit] = hi.ChangeVersion
	if hi.HookKind == "joined" {
		rs.ChangedPending = hi.RemoteUnit
	} else {
		rs.ChangedPending = ""
	}
	return nil
}

// createWalk walks the supplied directory tree, creating it if it does not
// exist, and calls the supplied function for its children, in the manner of
// os.Walk. If the path points to anything other than a directory, or if the
// directory is missing and cannot be created, an error is returned.
func createWalk(dirpath string, f func(path string, fi os.FileInfo) error) error {
	walker := func(path string, fi os.FileInfo, err error) error {
		if path == dirpath {
			if os.IsNotExist(err) {
				return os.Mkdir(dirpath, 0777)
			} else if !fi.IsDir() {
				return fmt.Errorf("%s must be a directory", dirpath)
			}
			return nil
		} else if err != nil {
			return err
		}
		return f(path, fi)
	}
	return filepath.Walk(dirpath, walker)
}

// unitname converts a relation unit filename to a unit name.
func unitname(filename string) (string, error) {
	i := strings.LastIndex(filename, "-")
	if i == -1 {
		return "", fmt.Errorf("invalid unit filename %q", filename)
	}
	svcName := filename[:i]
	unitId := filename[i+1:]
	if _, err := strconv.Atoi(unitId); err != nil {
		return "", fmt.Errorf("invalid unit filename %q", filename)
	}
	return svcName + "/" + unitId, nil
}
