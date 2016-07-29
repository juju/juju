// The bakery package layers on top of the macaroon package, providing
// a transport and storage-agnostic way of using macaroons to assert
// client capabilities.
//
package bakery

import (
	"crypto/rand"
	"fmt"
	"strings"

	"github.com/juju/loggo"
	"github.com/rogpeppe/fastuuid"
	"gopkg.in/errgo.v1"
	"gopkg.in/macaroon.v1"

	"gopkg.in/macaroon-bakery.v1/bakery/checkers"
)

var logger = loggo.GetLogger("bakery")

var uuidGen = fastuuid.MustNewGenerator()

// Service represents a service which can use macaroons
// to check authorization.
type Service struct {
	location string
	store    storage
	rkStore  RootKeyStorage
	checker  FirstPartyChecker
	encoder  *boxEncoder
	key      *KeyPair
	locator  PublicKeyLocator
}

// NewServiceParams holds the parameters for a NewService call.
type NewServiceParams struct {
	// Location will be set as the location of any macaroons
	// minted by the service.
	Location string

	// Store will be used to store macaroon
	// information locally. If it is nil,
	// an in-memory storage will be used.
	Store Storage

	// RootKeyStore is used to store macaroon root keys. If this
	// is non-nil, it will be used in preference to Store and it
	// will not be possible to specify non-empty macaroon ids and
	// root keys when calling NewMacaroon.
	RootKeyStore RootKeyStorage

	// Key is the public key pair used by the service for
	// third-party caveat encryption.
	// It may be nil, in which case a new key pair
	// will be generated.
	Key *KeyPair

	// Locator provides public keys for third-party services by location when
	// adding a third-party caveat.
	// It may be nil, in which case, no third-party caveats can be created.
	Locator PublicKeyLocator
}

// NewService returns a new service that can mint new
// macaroons and store their associated root keys.
func NewService(p NewServiceParams) (*Service, error) {
	if p.Store == nil {
		p.Store = NewMemStorage()
	}
	svc := &Service{
		location: p.Location,
		locator:  p.Locator,
	}
	if p.RootKeyStore != nil {
		svc.rkStore = p.RootKeyStore
	} else {
		svc.store = storage{p.Store}
	}

	var err error
	if p.Key == nil {
		p.Key, err = GenerateKey()
		if err != nil {
			return nil, err
		}
	}
	if svc.locator == nil {
		svc.locator = PublicKeyLocatorMap(nil)
	}
	svc.key = p.Key
	svc.encoder = newBoxEncoder(p.Key)
	return svc, nil
}

// WithRootKeyStore returns a copy of service where macaroon creation and
// lookup uses the given root key store to look up and create macaroon root
// keys.
//
// When NewMacaroon is called on the returned Service,
// it must always be called with an empty id and rootKey.
func (svc *Service) WithRootKeyStore(store RootKeyStorage) *Service {
	svc1 := *svc
	svc1.rkStore = store
	// Make sure the old store cannot be used.
	svc1.store.store = nil
	return &svc1
}

// Store returns the store used by the service.
// If the service has a RootKeyStorage (there
// was one specified in the parameters or the service
// was created with WithRootKeyStorage), it
// returns nil.
func (svc *Service) Store() Storage {
	if svc.rkStore != nil {
		return nil
	}
	return svc.store.store
}

// Location returns the service's configured macaroon location.
func (svc *Service) Location() string {
	return svc.location
}

// PublicKey returns the service's public key.
func (svc *Service) PublicKey() *PublicKey {
	return &svc.key.Public
}

// Check checks that the given macaroons verify
// correctly using the provided checker to check
// first party caveats. The primary macaroon is in ms[0]; the discharges
// fill the rest of the slice.
//
// If there is a verification error, it returns a VerificationError that
// describes the error (other errors might be returned in other
// circumstances).
func (svc *Service) Check(ms macaroon.Slice, checker FirstPartyChecker) error {
	if len(ms) == 0 {
		return &VerificationError{
			Reason: fmt.Errorf("no macaroons in slice"),
		}
	}
	id := ms[0].Id()
	if svc.rkStore != nil {
		// We're using a RootKeyStore - hack off the
		// uuid at the end of the id.
		if i := strings.LastIndex(id, "-"); i >= 0 {
			id = id[0:i]
		}
	}

	var rootKey []byte
	var err error
	if svc.rkStore != nil {
		rootKey, err = svc.rkStore.Get(id)
	} else {
		rootKey, err = svc.store.Get(id)
	}
	if err != nil {
		if errgo.Cause(err) == ErrNotFound {
			// If the macaroon was not found, it is probably
			// because it's been removed after time-expiry,
			// so return a verification error.
			return &VerificationError{
				Reason: errgo.New("macaroon not found in storage"),
			}
		}
		return errgo.Notef(err, "cannot get macaroon")
	}
	err = ms[0].Verify(rootKey, checker.CheckFirstPartyCaveat, ms[1:])
	if err != nil {
		return &VerificationError{
			Reason: err,
		}
	}
	return nil
}

// CheckAnyM is like CheckAny except that on success it also returns
// the set of macaroons that was successfully checked.
// The "M" suffix is for backward compatibility reasons - in a
// later bakery version, the signature of CheckAny will be
// changed to return the macaroon slice and CheckAnyM will be
// removed.
func (svc *Service) CheckAnyM(mss []macaroon.Slice, assert map[string]string, checker checkers.Checker) (map[string]string, macaroon.Slice, error) {
	if len(mss) == 0 {
		return nil, nil, &VerificationError{
			Reason: errgo.Newf("no macaroons"),
		}
	}
	// TODO perhaps return a slice of attribute maps, one
	// for each successfully validated macaroon slice?
	var err error
	for _, ms := range mss {
		declared := checkers.InferDeclared(ms)
		for key, val := range assert {
			declared[key] = val
		}
		err = svc.Check(ms, checkers.New(declared, checker))
		if err == nil {
			return declared, ms, nil
		}
	}
	// Return an arbitrary error from the macaroons provided.
	// TODO return all errors.
	return nil, nil, errgo.Mask(err, isVerificationError)
}

// CheckAny checks that the given slice of slices contains at least
// one macaroon minted by the given service, using checker to check
// any first party caveats. It returns an error with a
// *bakery.VerificationError cause if the macaroon verification failed.
//
// The assert map holds any required attributes of "declared" attributes,
// overriding any inferences made from the macaroons themselves.
// It has a similar effect to adding a checkers.DeclaredCaveat
// for each key and value, but the error message will be more
// useful.
//
// It adds all the standard caveat checkers to the given checker.
//
// It returns any attributes declared in the successfully validated request.
func (svc *Service) CheckAny(mss []macaroon.Slice, assert map[string]string, checker checkers.Checker) (map[string]string, error) {
	attrs, _, err := svc.CheckAnyM(mss, assert, checker)
	return attrs, err
}

func isVerificationError(err error) bool {
	_, ok := err.(*VerificationError)
	return ok
}

// NewMacaroon mints a new macaroon with the given id and caveats.
// If the id is empty, a random id will be used.
// If rootKey is nil, a random root key will be used.
// The macaroon will be stored in the service's storage.
// TODO swap the first two arguments so that they're
// in the same order as macaroon.New.
func (svc *Service) NewMacaroon(id string, rootKey []byte, caveats []checkers.Caveat) (*macaroon.Macaroon, error) {
	rootKey, id, err := svc.rootKey(rootKey, id)
	if err != nil {
		return nil, errgo.Mask(err)
	}
	m, err := macaroon.New(rootKey, id, svc.location)
	if err != nil {
		return nil, errgo.Notef(err, "cannot bake macaroon")
	}
	for _, cav := range caveats {
		if err := svc.AddCaveat(m, cav); err != nil {
			return nil, errgo.Notef(err, "cannot add caveat")
		}
	}
	if svc.rkStore == nil {
		if err := svc.store.Put(m.Id(), rootKey); err != nil {
			return nil, errgo.Notef(err, "cannot save macaroon to store")
		}
	}
	return m, nil
}

func (svc *Service) rootKey(rootKey []byte, id string) ([]byte, string, error) {
	if svc.rkStore != nil {
		if len(rootKey) > 0 || id != "" {
			return nil, "", errgo.Newf("cannot choose root key or id when using RootKeyStore")
		}
		rootKey, id, err := svc.rkStore.RootKey()
		if err != nil {
			return nil, "", errgo.Mask(err)
		}
		// Add a UUID to the end of the id so that even
		// though we may be re-using the same underlying
		// id and root key, all minted macaroons will have
		// unique ids.
		uuid := uuidGen.Next()
		id = fmt.Sprintf("%s-%x", id, uuid[0:16])
		return rootKey, id, nil
	}
	if rootKey == nil {
		newRootKey, err := randomBytes(24)
		if err != nil {
			return nil, "", fmt.Errorf("cannot generate root key for new macaroon: %v", err)
		}
		rootKey = newRootKey
	}
	if id == "" {
		idBytes, err := randomBytes(24)
		if err != nil {
			return nil, "", fmt.Errorf("cannot generate id for new macaroon: %v", err)
		}
		id = fmt.Sprintf("%x", idBytes)
	}
	return rootKey, id, nil
}

// LocalThirdPartyCaveat returns a third-party caveat that, when added
// to a macaroon with AddCaveat, results in a caveat
// with the location "local", encrypted with the given public key.
// This can be automatically discharged by DischargeAllWithKey.
func LocalThirdPartyCaveat(key *PublicKey) checkers.Caveat {
	return checkers.Caveat{
		Location: "local " + key.String(),
	}
}

// AddCaveat adds a caveat to the given macaroon.
//
// If it's a third-party caveat, it uses the service's caveat-id encoder
// to create the id of the new caveat.
//
// As a special case, if the caveat's Location field has the prefix
// "local " the caveat is added as a client self-discharge caveat
// using the public key base64-encoded in the rest of the location.
// In this case, the Condition field must be empty. The
// resulting third-party caveat will encode the condition "true"
// encrypted with that public key. See LocalThirdPartyCaveat
// for a way of creating such caveats.
func (svc *Service) AddCaveat(m *macaroon.Macaroon, cav checkers.Caveat) error {
	if cav.Location == "" {
		m.AddFirstPartyCaveat(cav.Condition)
		return nil
	}
	var thirdPartyPub *PublicKey
	if strings.HasPrefix(cav.Location, "local ") {
		var key PublicKey
		if err := key.UnmarshalText([]byte(cav.Location[len("local "):])); err != nil {
			return errgo.Notef(err, "cannot unmarshal client's public key in local third-party caveat")
		}
		thirdPartyPub = &key
		cav.Location = "local"
		if cav.Condition != "" {
			return errgo.New("cannot specify caveat condition in local third-party caveat")
		}
		cav.Condition = "true"
	} else {
		var err error
		thirdPartyPub, err = svc.locator.PublicKeyForLocation(cav.Location)
		if err != nil {
			return errgo.Notef(err, "cannot find public key for location %q", cav.Location)
		}
	}
	rootKey, err := randomBytes(24)
	if err != nil {
		return errgo.Notef(err, "cannot generate third party secret")
	}
	id, err := svc.encoder.encodeCaveatId(cav.Condition, rootKey, thirdPartyPub)
	if err != nil {
		return errgo.Notef(err, "cannot create third party caveat id at %q", cav.Location)
	}
	if err := m.AddThirdPartyCaveat(rootKey, id, cav.Location); err != nil {
		return errgo.Notef(err, "cannot add third party caveat")
	}
	return nil
}

// Discharge creates a macaroon that discharges the third party caveat with the
// given id that should have been created earlier using key.Public. The
// condition implicit in the id is checked for validity using checker. If
// it is valid, a new macaroon is returned which discharges the caveat
// along with any caveats returned from the checker.
func Discharge(key *KeyPair, checker ThirdPartyChecker, id string) (*macaroon.Macaroon, []checkers.Caveat, error) {
	decoder := newBoxDecoder(key)

	logger.Infof("server attempting to discharge %q", id)
	rootKey, condition, err := decoder.decodeCaveatId(id)
	if err != nil {
		return nil, nil, errgo.Notef(err, "discharger cannot decode caveat id")
	}
	// Note that we don't check the error - we allow the
	// third party checker to see even caveats that we can't
	// understand.
	cond, arg, _ := checkers.ParseCaveat(condition)
	var caveats []checkers.Caveat
	if cond == checkers.CondNeedDeclared {
		caveats, err = checkNeedDeclared(id, arg, checker)
	} else {
		caveats, err = checker.CheckThirdPartyCaveat(id, condition)
	}
	if err != nil {
		return nil, nil, errgo.Mask(err, errgo.Any)
	}
	// Note that the discharge macaroon does not need to
	// be stored persistently. Indeed, it would be a problem if
	// we did, because then the macaroon could potentially be used
	// for normal authorization with the third party.
	m, err := macaroon.New(rootKey, id, "")
	if err != nil {
		return nil, nil, errgo.Mask(err)
	}
	return m, caveats, nil
}

// Discharge calls Discharge with the service's key and uses the service
// to add any returned caveats to the discharge macaroon.
func (svc *Service) Discharge(checker ThirdPartyChecker, id string) (*macaroon.Macaroon, error) {
	m, caveats, err := Discharge(svc.encoder.key, checker, id)
	if err != nil {
		return nil, errgo.Mask(err, errgo.Any)
	}
	for _, cav := range caveats {
		if err := svc.AddCaveat(m, cav); err != nil {
			return nil, errgo.Notef(err, "cannot add caveat")
		}
	}
	return m, nil
}

func checkNeedDeclared(caveatId, arg string, checker ThirdPartyChecker) ([]checkers.Caveat, error) {
	i := strings.Index(arg, " ")
	if i <= 0 {
		return nil, errgo.Newf("need-declared caveat requires an argument, got %q", arg)
	}
	needDeclared := strings.Split(arg[0:i], ",")
	for _, d := range needDeclared {
		if d == "" {
			return nil, errgo.New("need-declared caveat with empty required attribute")
		}
	}
	if len(needDeclared) == 0 {
		return nil, fmt.Errorf("need-declared caveat with no required attributes")
	}
	caveats, err := checker.CheckThirdPartyCaveat(caveatId, arg[i+1:])
	if err != nil {
		return nil, errgo.Mask(err, errgo.Any)
	}
	declared := make(map[string]bool)
	for _, cav := range caveats {
		if cav.Location != "" {
			continue
		}
		// Note that we ignore the error. We allow the service to
		// generate caveats that we don't understand here.
		cond, arg, _ := checkers.ParseCaveat(cav.Condition)
		if cond != checkers.CondDeclared {
			continue
		}
		parts := strings.SplitN(arg, " ", 2)
		if len(parts) != 2 {
			return nil, errgo.Newf("declared caveat has no value")
		}
		declared[parts[0]] = true
	}
	// Add empty declarations for everything mentioned in need-declared
	// that was not actually declared.
	for _, d := range needDeclared {
		if !declared[d] {
			caveats = append(caveats, checkers.DeclaredCaveat(d, ""))
		}
	}
	return caveats, nil
}

func randomBytes(n int) ([]byte, error) {
	b := make([]byte, n)
	_, err := rand.Read(b)
	if err != nil {
		return nil, fmt.Errorf("cannot generate %d random bytes: %v", n, err)
	}
	return b, nil
}

type VerificationError struct {
	Reason error
}

func (e *VerificationError) Error() string {
	return fmt.Sprintf("verification failed: %v", e.Reason)
}

// TODO(rog) consider possible options for checkers:
// - first and third party checkers could be merged, but
// then there would have to be a runtime check
// that when used to check first-party caveats, the
// checker does not return third-party caveats.

// ThirdPartyChecker holds a function that checks third party caveats
// for validity. If the caveat is valid, it returns a nil error and
// optionally a slice of extra caveats that will be added to the
// discharge macaroon. The caveatId parameter holds the still-encoded id
// of the caveat.
//
// If the caveat kind was not recognised, the checker should return an
// error with a ErrCaveatNotRecognized cause.
type ThirdPartyChecker interface {
	CheckThirdPartyCaveat(caveatId, caveat string) ([]checkers.Caveat, error)
}

type ThirdPartyCheckerFunc func(caveatId, caveat string) ([]checkers.Caveat, error)

func (c ThirdPartyCheckerFunc) CheckThirdPartyCaveat(caveatId, caveat string) ([]checkers.Caveat, error) {
	return c(caveatId, caveat)
}

// FirstPartyChecker holds a function that checks first party caveats
// for validity.
//
// If the caveat kind was not recognised, the checker should return
// ErrCaveatNotRecognized.
type FirstPartyChecker interface {
	CheckFirstPartyCaveat(caveat string) error
}

type FirstPartyCheckerFunc func(caveat string) error

func (c FirstPartyCheckerFunc) CheckFirstPartyCaveat(caveat string) error {
	return c(caveat)
}
