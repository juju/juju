package paths

import (
	"os"
	"os/exec"
	"path"
	"path/filepath"

	"github.com/juju/errors"
)

const (
	// TODO (perrito666) this seems to be a package decission we should not
	// rely on it and we should be aware of /usr/lib/juju if its something
	// of ours.

	// mongoBinDir is the path to the juju-bundled mongo executables.
	mongoBinDir = "/usr/lib/juju/bin"

	mongoServer  = "mongod"
	mongoRestore = "mongorestore"
	mongoDump    = "mongodump"
)

// Mongo exposes the mongo-related paths that juju cares about.
type Mongo struct {
	BinDir string
}

// NewMongo creates a new Mongo using the juju-bundled mongo paths and
// returns it.
func NewMongo() Mongo {
	return Mongo{mongoBinDir}
}

// MatchMongo creates a new Mongo based in the same directory as the
// provided mongod path and returns it.
func MatchMongo(mongod string) Mongo {
	return Mongo{path.Dir(mongod)}
}

// Path returns the path to the mongod binary.
func (m Mongo) Path() string {
	return path.Join(m.BinDir, mongoServer)
}

// DumpPath returns the path to the mongodump binary.
func (m Mongo) DumpPath() string {
	return path.Join(m.BinDir, mongoDump)
}

// RestorePath returns the path to the mongorestore binary.
func (m Mongo) RestorePath() string {
	return path.Join(m.BinDir, mongoRestore)
}

var osStat = os.Stat
var execLookPath = exec.LookPath

// Find looks for `executable` on the system and returns it if it
// actually exists. It first looks for the provided path. If that is not
// found then Find checks $PATH for the base name of `executable`.
func Find(executable string) (string, error) {
	if _, err := osStat(executable); err == nil {
		return executable, nil
	}

	name := filepath.Base(executable)
	path, err := execLookPath(name)
	if err != nil {
		return "", errors.Annotatef(err, "could not find %s in $PATH", name)
	}
	return path, nil
}
