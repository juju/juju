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

var checker = schema.StrictFieldMap(
	schema.Fields{"change-version": schema.Int(), "changed-pending": schema.Bool()},
	schema.Defaults{"changed-pending": false},
)

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

func LoadRelationStates(dirpath string) (map[int]*RelationState, error) {
	states := map[int]*RelationState{}
	walker := func(path string, fi os.FileInfo) error {
		if !fi.IsDir() {
			return fmt.Errorf("relation %q is not a directory")
		}
		relationId, err := strconv.Atoi(fi.Name())
		if err != nil {
			return fmt.Errorf("%q is not a valid relation id", fi.Name())
		}
		state, err := NewRelationState(dirpath, relationId)
		if err != nil {
			return err
		}
		states[relationId] = state
		return filepath.SkipDir
	}
	if err := createWalk(dirpath, walker); err != nil {
		return nil, err
	}
	return states, nil
}

func (rs *RelationState) Commit(hi HookInfo) error {
	if hi.RelationId != rs.RelationId {
		panic("tried to persist hook state to inappropriate relation!")
	}
	name := strings.Replace(hi.RemoteUnit, "/", "-", -1)
	path := filepath.Join(rs.Path, name)
	unit := map[string]interface{}{"change-version": hi.ChangeVersion}
	if hi.HookKind == "joined" {
		unit["changed-pending"] = true
	}
	data, err := goyaml.Marshal(unit)
	if err != nil {
		return err
	}
	if err = ioutil.WriteFile(path+"~", data, 0777); err != nil {
		return err
	}
	return os.Rename(path+"~", path)
}

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

func unitname(fileName string) (string, error) {
	i := strings.LastIndex(fileName, "-")
	if i == -1 {
		return "", fmt.Errorf("invalid unit filename %q", fileName)
	}
	svcName := fileName[:i]
	unitId := fileName[i+1:]
	if _, err := strconv.Atoi(unitId); err != nil {
		return "", fmt.Errorf("invalid unit filename %q", fileName)
	}
	return svcName + "/" + unitId, nil
}
