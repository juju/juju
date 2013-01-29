package state
import (
	"fmt"
	"labix.org/v2/mgo"
	"labix.org/v2/mgo/txn"
	"launchpad.net/juju-core/trivial"
	"regexp"
	"strings"
)

var validUser = regexp.MustCompile("^[a-zA-Z][a-zA-Z0-9]*$")

// AddUser adds a user to the state.
func (st *State) AddUser(name, password string) error {
	if !validUser.MatchString(name) {
		return fmt.Errorf("invalid user name %q", name)
	}
	udoc := userDoc{
		Name: name,
		PasswordHash: trivial.PasswordHash(password),
	}
	ops := []txn.Op{{
		C: st.users.Name,
		Id: name,
		Assert: txn.DocMissing,
		Insert: udoc,
	}}
	err := st.runner.Run(ops, "", nil)
	if err == txn.ErrAborted {
		err = fmt.Errorf("user already exists")
	}
	return err
}

// getUser fetches information about the user with the
// given name into the provided userDoc.
func (st *State) getUser(name string, udoc *userDoc) error {
	err := st.users.Find(D{{"_id", name}}).One(udoc)
	if err == mgo.ErrNotFound {
		err = notFoundf("user %s", name)
	}
	return err
}


// User returns the state user for the given name,
func (st *State) User(name string) (*User, error) {
	u := &User{st: st}
	if err := st.getUser(name, &u.doc); err != nil {
		return nil, err
	}
	return u, nil
}

// Entity represents an entity that has
// a password that can be authenticated against.
type AuthEntity interface {
	SetPassword(pass string) error
	PasswordValid(pass string) bool
}

// AuthEntity returns the entity for the given name.
func (st *State) AuthEntity(entityName string) (AuthEntity, error) {
	i := strings.Index(entityName, "-")
	if i <= 0 || i >= len(entityName)-1 {
		return nil, fmt.Errorf("invalid entity name %q", entityName)
	}
	prefix, id := entityName[0:i], entityName[i+1:]
	switch prefix {
	case "machine":
		return st.Machine(id)
	case "unit":
		return st.Unit(id)
	case "user":
		return st.User(id)
	}
	return nil, fmt.Errorf("invalid entity name %q", entityName)
}

// User represents a juju client user.
type User struct {
	st *State
	doc userDoc
}

type userDoc struct {
	Name string		`bson:"_id_"`
	PasswordHash string
}

// Name returns the user name,
func (u *User) Name() string {
	return u.doc.Name
}

// EntityName returns the entity name for
// the user ("user-$username")
func (u *User) EntityName() string {
	return "user-"+u.doc.Name
}

// SetPassword sets the password associated with the user.
func (u *User) SetPassword(password string) error {
	hp := trivial.PasswordHash(password)
	ops := []txn.Op{{
		C: u.st.users.Name,
		Id: u.Name(),
		Update: D{{"$set", D{{"passwordhash", hp}}}},
	}}
	if err := u.st.runner.Run(ops, "", nil); err != nil {
		return fmt.Errorf("cannot set password of user %q: %v", u.Name(), err)
	}
	u.doc.PasswordHash = hp
	return nil
}

// PasswordValid returns whether the given password
// is valid for the user.
func (u *User) PasswordValid(password string) bool {
	return trivial.PasswordHash(password) == u.doc.PasswordHash
}

// Refresh refreshes information about the user
// from the state.
func (u *User) Refresh() error {
	var udoc userDoc
	if err := u.st.getUser(u.Name(), &udoc); err != nil {
		return err
	}
	u.doc = udoc
	return nil
}