package uniter

import (
	"os"
	"path/filepath"
)

// RelationState is a snapshot of the state of a relation.
type RelationState struct {
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
	Version       int
	ChangePending bool
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

func NewRelationState(dirpath string, relationId int) (RelationState, error) {
	rs := RelationState{relationId, map[string]int{}, ""}
	walker := func(path string, fi os.FileInfo) error {
		if fi.IsDir() {
			return fmt.Errorf("relation directory must be flat")
		}
		name := fi.Name()
		if strings.HasSuffix(name, "~") {
			return nil
		}
		b, err := ioutil.ReadFile(path)
		if err != nil {
			return err
		}
		var unit *diskUnit
		if err := goyaml.Unmarshal(b, unit); err != nil {
			return err
		}

	}
	path := filepath.Join(dirpath, fmt.Sprintf("%d", relationId))
	if err := createWalk(path, walker); err != nil {
		return nil, err
	}
	return rs, nil
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
