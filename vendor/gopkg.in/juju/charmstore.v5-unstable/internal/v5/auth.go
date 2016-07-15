// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package v5 // import "gopkg.in/juju/charmstore.v5-unstable/internal/v5"

import (
	"encoding/base64"
	"net/http"
	"sort"
	"strings"
	"time"

	"gopkg.in/errgo.v1"
	"gopkg.in/juju/charmrepo.v2-unstable/csclient/params"
	"gopkg.in/macaroon-bakery.v1/bakery"
	"gopkg.in/macaroon-bakery.v1/bakery/checkers"
	"gopkg.in/macaroon-bakery.v1/httpbakery"
	"gopkg.in/macaroon.v1"

	"gopkg.in/juju/charmstore.v5-unstable/internal/charmstore"
	"gopkg.in/juju/charmstore.v5-unstable/internal/mongodoc"
	"gopkg.in/juju/charmstore.v5-unstable/internal/router"
)

const UsernameAttr = "username"

// authorization conatains authorization information extracted from an HTTP request.
// The zero value for a authorization contains no privileges.
type authorization struct {
	Admin    bool
	Username string
}

const (
	PromulgatorsGroup = "charmers"

	defaultMacaroonExpiry   = 24 * time.Hour
	shortTermMacaroonExpiry = time.Minute
)

// These operations represent categories of operation by the user on
// the charm store. All the operations should be
// mutually distinct.
const (
	// OpReadWithNoTerms is the operation of reading an entity
	// where either the entity has no associated terms and conditions
	// or we don't need to agree to them to access the entity.
	OpReadWithNoTerms = "read-no-terms"

	// OpReadWithTerms is the operation of reading an entity
	// where terms must be agreed to before contents
	// are provided.
	OpReadWithTerms = "read-with-terms"

	// OpWrite indicates an operation that changes something in the charmstore.
	OpWrite = "write"
)

// authnCheckableOps holds the set of operations that
// can be authorized with just a username.
var authnCheckableOps = []string{
	OpReadWithNoTerms,
	OpWrite,
}

// timeNow is defined as a variable so that it can be overridden in tests.
var timeNow = time.Now

// AuthorizeEntity is a convenience method that calls authorize to check
// that the given request can access the entity with the given id to
// perform all of the given operations. The operation will be chosen based
// on the request method (OpReadNonArchive for non-mutating HTTP
// methods; OpWrite for others).
//
// If the request fails because authorization is denied, a macaroon is
// minted that will provide access when discharged, and returned as a
// discharge-required error.
//
// This method implements router.Context.AuthorizeEntity.
func (h *ReqHandler) AuthorizeEntity(id *router.ResolvedURL, req *http.Request) error {
	var op string
	switch req.Method {
	case "GET", "HEAD", "OPTIONS":
		op = OpReadWithNoTerms
	default:
		op = OpWrite
	}
	return h.AuthorizeEntityForOp(id, req, op)
}

// Authenticate is a convenience method that calls authorize to check
// that that the given request is authenticated for some user.
func (h *ReqHandler) Authenticate(req *http.Request) (authorization, error) {
	return h.authorize(authorizeParams{
		req: req,
		acls: []mongodoc.ACL{{
			Read: []string{params.Everyone},
		}},
		ops:           []string{OpReadWithNoTerms},
		authnRequired: true,
	})
}

// AuthorizeEntityForOp is a convenience method that calls authorize to check
// that that the given request is authorized to perform the given operation
// on the entity with the given id.
func (h *ReqHandler) AuthorizeEntityForOp(id *router.ResolvedURL, req *http.Request, op string) error {
	_, err := h.authorize(authorizeParams{
		req:       req,
		ops:       []string{op},
		entityIds: []*router.ResolvedURL{id},
	})
	if err != nil {
		return errgo.Mask(err, errgo.Any)
	}
	return nil
}

// authenticateAdmin checks that the given request has admin credentials.
func (h *ReqHandler) authenticateAdmin(req *http.Request) error {
	if _, err := h.authorize(authorizeParams{
		req: req,
		ops: []string{OpReadWithNoTerms},
		// Provide an empty ACL, which means that only a request
		// with admin credentials can be authorized.
		acls: []mongodoc.ACL{{}},
	}); err != nil {
		return errgo.Mask(err, errgo.Any)
	}
	return nil
}

// authorizeParams holds parameters for an Authorize request.
type authorizeParams struct {
	// req holds the client HTTP request.
	req *http.Request

	// ops holds the operations to be performed.
	// Authorize checks that all these operations are
	// allowed.
	ops []string

	// acls holds optional additional ACLs to be checked. The request
	// will only be authorized if the authenticated user
	// is a member of the appropriate member of these ACLs
	// and of the ACLs of all the entity ids (but see also
	// ignoreEntityACLs).
	acls []mongodoc.ACL

	// entityIds holds the set of entity ids being accessed.
	entityIds []*router.ResolvedURL

	// ignoreEntityACLs holds whether ACLs on the above
	// entityIds should be ignored. In this case, acls should
	// hold the actual ACLs to check.
	ignoreEntityACLs bool

	// authnRequired holds whether the request should be
	// authenticated even if the ACLs are open to everyone.
	// This automatically applies to non-read requests.
	authnRequired bool
}

// authorize checks that the current user is authorized to perform
// the request specified in the given parameters. If an authenticated user
// is required, authorize tries to retrieve the current user in the
// following ways:
//
// - by checking that the request header's HTTP basic auth credentials match
//   the auth credentials stored in the API handler;
//
// - by checking that there is a valid macaroon in the request's cookies.
// A params.ErrUnauthorized error is returned if superuser credentials fail;
// otherwise a macaroon is minted and a httpbakery discharge-required
// error is returned holding the macaroon.
//
// This method also sets h.auth to the returned authorization info.
func (h *ReqHandler) authorize(p authorizeParams) (authorization, error) {
	if len(p.ops) == 0 {
		return authorization{}, errgo.Newf("no operation to be authorized")
	}
	// TODO this is logging statement is actually quite costly
	// when we're dealing with requests that need to authorize
	// many entities (e.g. charm-related). Consider removing
	// it or making it dependent on context.
	logger.Infof(
		"authorize, ops %q, acls %q, path: %q, method: %q, entities: %v",
		p.ops,
		p.acls,
		p.req.URL.Path,
		p.req.Method,
		p.entityIds,
	)

	set := newACLSet(len(p.entityIds) + 1)
	if !p.ignoreEntityACLs {
		if err := h.addEntitiesACLs(set, p.entityIds); err != nil {
			return authorization{}, errgo.Mask(err)
		}
	}
	for _, acl := range p.acls {
		set.add(acl)
	}
	if len(set.acls) == 0 {
		return authorization{}, errgo.Newf("no ACLs or entities specified in authorization request")
	}

	newOps, requiredTerms, newAuthnRequired, err := h.verifyOps(p.ops, p.entityIds)
	if err != nil {
		return authorization{}, errgo.Mask(err)
	}
	p.ops = newOps
	p.authnRequired = p.authnRequired || newAuthnRequired

	if len(requiredTerms) > 0 && h.Handler.config.TermsLocation == "" {
		return authorization{}, errgo.WithCausef(nil, params.ErrUnauthorized, "charmstore not configured to serve charms with terms and conditions")
	}

	if !p.authnRequired && set.readPublic {
		// No need to authenticate if the ACL is open to everyone and we're
		// just trying to read something that won't require terms to be agreed to.
		// TODO return Username: "everyone" here?
		return authorization{}, nil
	}
	auth, verr := h.checkRequest(p)
	if verr == nil {
		// The request is OK. Now check that the user associated with
		// the verified macaroons is part of the ACL.
		if err := set.check(auth, p.ops, h.Handler.permChecker.Allow); err != nil {
			return authorization{}, errgo.WithCausef(err, params.ErrUnauthorized, "")
		}
		h.auth = auth
		return auth, nil
	}
	if _, ok := errgo.Cause(verr).(*bakery.VerificationError); !ok {
		return authorization{}, errgo.Mask(verr, errgo.Is(params.ErrUnauthorized), isDischargeRequiredError)
	}

	// Macaroon verification failed: mint a new macaroon.

	shortTerm := len(requiredTerms) > 0
	grantOps := p.ops
	if !shortTerm {
		// If we're issuing a long-term macaroon, allow any operations
		// that can be authorized with authentication only.
		grantOps = authnCheckableOps
	}
	m, err := h.newMacaroon(grantOps, p.entityIds, requiredTerms, shortTerm)
	if err != nil {
		return authorization{}, errgo.Notef(err, "cannot mint macaroon")
	}
	return authorization{}, h.newDischargeRequiredError(m, verr, p.req, shortTerm)
}

// verifyOps verifies that all the given operations on the given entity ids
// are valid and are appropriate. It returns the actually applicable
// operations, changing OpReadWithTerms to OpReadWithNoTerms
// if none of the entities require terms to be agreed.
//
// It also reports whether authentication should always be done.
func (h *ReqHandler) verifyOps(ops []string, entityIds []*router.ResolvedURL) (actualOps []string, requiredTerms []string, authnRequired bool, err error) {
	opsMap := make(map[string]bool)
	opsChanged := false
	for _, op := range ops {
		switch op {
		case OpReadWithTerms:
			var err error
			// Terms agreement might be required. Find out all the required terms
			// so we can add them to the macaroon.
			requiredTerms, err = h.entitiesRequiredTerms(entityIds)
			if err != nil {
				return nil, nil, false, errgo.Notef(err, "cannot acquire required terms for entities")
			}
			if len(requiredTerms) == 0 {
				// There are no terms, so OpReadWithTerms is unnecessary.
				opsMap[OpReadWithNoTerms] = true
				opsChanged = true
				break
			}
			authnRequired = true
			opsMap[op] = true
		case OpWrite:
			authnRequired = true
			opsMap[op] = true
		case OpReadWithNoTerms:
			opsMap[op] = true
		default:
			return nil, nil, false, errgo.Newf("unknown operation %q", op)
		}
	}
	if !opsChanged {
		return ops, requiredTerms, authnRequired, nil
	}
	actualOps = make([]string, 0, len(opsMap))
	for op := range opsMap {
		actualOps = append(actualOps, op)
	}
	return actualOps, requiredTerms, authnRequired, nil
}

// newDischargeRequiredError returns a discharge-required error that contains the
// given macaroon, created because the given request failed with the given authorization
// error. The shortTerm parameter specifies whether the macaroon has a short
// expiry duration.
func (h *ReqHandler) newDischargeRequiredError(m *macaroon.Macaroon, verr error, req *http.Request, shortTerm bool) error {
	// Request that this macaroon be supplied for all requests
	// to the whole handler. We use a relative path because
	// the charm store is conventionally under an external
	// root path, with other services also under the same
	// externally visible host name, and we don't want our
	// cookies to be presnted to those services.
	cookiePath := "/"
	if p, err := router.RelativeURLPath(req.RequestURI, "/"); err != nil {
		// Should never happen, as RequestURI should always be absolute.
		logger.Infof("cannot make relative URL from %v", req.RequestURI)
	} else {
		cookiePath = p
	}
	dischargeErr := httpbakery.NewDischargeRequiredErrorForRequest(m, cookiePath, verr, req)
	cookieName := "authn"
	if shortTerm {
		// It's a short term authorization macaroon so use a different
		// cookie name so  that we don't overrite the longer term authentication
		// macaroon.
		cookieName = "authz"
	}
	dischargeErr.(*httpbakery.Error).Info.CookieNameSuffix = cookieName
	return dischargeErr
}

func isDischargeRequiredError(err error) bool {
	if err, ok := errgo.Cause(err).(*httpbakery.Error); ok && err.Code == httpbakery.ErrDischargeRequired {
		return true
	}
	return false
}

// addEntitiesACLs adds ACLs for all the given entities to the given ACL set.
func (h *ReqHandler) addEntitiesACLs(set *aclSet, ids []*router.ResolvedURL) error {
	for _, id := range ids {
		acl, err := h.entityACLs(id)
		if err != nil {
			return errgo.Mask(err, errgo.Is(params.ErrNotFound))
		}
		set.add(acl)
	}
	return nil
}

var errActiveTimeExpired = errgo.New("active time expired")

// checkRequest checks whether the given HTTP request is authorized
// with respect to the given authorization parameters.
// If no suitable credentials are found, or an error occurs, then a zero
// valued authorization is returned. It also checks any first party
// caveats. It does not check ACLs.
func (h *ReqHandler) checkRequest(p authorizeParams) (authorization, error) {
	user, passwd, err := parseCredentials(p.req)
	if err == nil {
		if user != h.Handler.config.AuthUsername || passwd != h.Handler.config.AuthPassword {
			return authorization{}, errgo.WithCausef(nil, params.ErrUnauthorized, "invalid user name or password")
		}
		return authorization{Admin: true}, nil
	}
	bk := h.Store.Bakery
	if errgo.Cause(err) != errNoCreds || bk == nil || h.Handler.config.IdentityLocation == "" {
		return authorization{}, errgo.WithCausef(err, params.ErrUnauthorized, "authentication failed")
	}
	// active holds whether we're checking with active status.
	// It's used by the active-time-before caveat checker.
	active := true
	reqCheckers := checkers.New(
		isEntityChecker{p.entityIds},
		checkers.OperationsChecker(p.ops),
		checkers.CheckerFunc{
			Condition_: condActiveTimeBefore,
			Check_: func(_, args string) error {
				t, err := time.Parse(time.RFC3339Nano, args)
				if err != nil {
					return errgo.Mask(err)
				}
				if !active || timeNow().Before(t) {
					return nil
				}
				return errActiveTimeExpired
			},
		},
	)

	attrMap, _, err := httpbakery.CheckRequestM(bk, p.req, nil, reqCheckers)
	if err == nil {
		// TODO check username is non-empty?
		return authorization{
			Admin:    false,
			Username: attrMap[UsernameAttr],
		}, nil
	}
	verr, ok := errgo.Cause(err).(*bakery.VerificationError)
	if !ok || errgo.Cause(verr.Reason) != errActiveTimeExpired {
		return authorization{}, errgo.Mask(err, errgo.Any)
	}
	// Set active to false and see if the macaroon can be used to self-renew.
	active = false
	_, ms, err := httpbakery.CheckRequestM(bk, p.req, nil, reqCheckers)
	if err != nil {
		return authorization{}, errgo.Mask(err, errgo.Any)
	}
	// The active time period of the macaroon has expired, but it's
	// otherwise still valid. Mint another macaroon with a later expiration
	// date but all other first party caveats the same.
	newm, err := h.Store.Bakery.NewMacaroon("", nil, nil)
	if err != nil {
		return authorization{}, errgo.Notef(err, "cannot make renewed macaroon")
	}
	activeExpireTime := timeNow().Add(DelegatableMacaroonExpiry)
	err = renewMacaroon(newm, ms, activeExpireTime)
	if err != nil {
		return authorization{}, errgo.Notef(err, "cannot renew macaroon")
	}
	return authorization{}, h.newDischargeRequiredError(newm, errgo.New("active lifetime expired; renew macaroon"), p.req, false)
}

// entityACLs calculates the ACLs for the specified entity. If the entity
// has been published to the stable channel then the StableChannel ACLs will be
// used; if the entity has been published to development, but not stable
// then the DevelopmentChannel ACLs will be used; otherwise
// the unpublished ACLs are used.
func (h *ReqHandler) entityACLs(id *router.ResolvedURL) (mongodoc.ACL, error) {
	ch, err := h.entityChannel(id)
	if err != nil {
		return mongodoc.ACL{}, errgo.Mask(err, errgo.Is(params.ErrNotFound))
	}
	baseEntity, err := h.Cache.BaseEntity(&id.URL, charmstore.FieldSelector("channelacls"))
	if err != nil {
		return mongodoc.ACL{}, errgo.Notef(err, "cannot retrieve base entity %q for authorization", id)
	}
	return baseEntity.ChannelACLs[ch], nil
}

// entitiesRequiredTerms returns the set of terms that the user must have
// agreed to in order to access the entities with the given ids.
func (h *ReqHandler) entitiesRequiredTerms(ids []*router.ResolvedURL) ([]string, error) {
	found := make(map[string]bool)
	var terms []string
	for _, id := range ids {
		if id.URL.Series == "bundle" {
			// Bundles cannot have terms.
			continue
		}
		entity, err := h.Cache.Entity(&id.URL, charmstore.FieldSelector("charmmeta"))
		if err != nil {
			return nil, errgo.Mask(err, errgo.Is(params.ErrNotFound))
		}
		for _, term := range entity.CharmMeta.Terms {
			if found[term] {
				continue
			}
			terms = append(terms, term)
			found[term] = true
		}
	}
	sort.Strings(terms)
	return terms, nil
}

// renewMacaroon renews the macaroons in the given slice by copying all
// their first-party caveats onto newm, except for active-time-before,
// which gets extended to the given new expiry time.
func renewMacaroon(newm *macaroon.Macaroon, ms macaroon.Slice, newExpiry time.Time) error {
	for _, m := range ms {
		for _, c := range m.Caveats() {
			if c.Location != "" {
				// Ignore third party caveat.
				continue
			}
			if strings.HasPrefix(c.Id, condActiveTimeBefore+" ") {
				// Ignore any old active-time-before caveats.
				continue
			}
			if err := newm.AddFirstPartyCaveat(c.Id); err != nil {
				// Can't happen in fact, as the only failure
				// mode is when the id is too big but we know
				// it's small enough because it came from a macaroon.
				return errgo.Mask(err)
			}
		}
	}
	if err := newm.AddFirstPartyCaveat(activeTimeBeforeCaveat(newExpiry).Condition); err != nil {
		return errgo.Mask(err)
	}
	return nil
}

func isEntityCaveat(ids []*router.ResolvedURL) checkers.Caveat {
	idStrs := make([]string, len(ids))
	for i, id := range ids {
		idStrs[i] = id.String()
	}
	return checkers.Caveat{
		Condition: "is-entity " + strings.Join(idStrs, " "),
	}
}

// isEntityChecker implements the is-entity caveat checker.
type isEntityChecker struct {
	ids []*router.ResolvedURL
}

func (c isEntityChecker) Condition() string {
	return "is-entity"
}

func (c isEntityChecker) Check(_, args string) error {
	if len(c.ids) == 0 {
		return errgo.Newf("operation does not involve any entities")
	}
	allowedIds := make(map[string]bool)
	for _, idStr := range strings.Fields(args) {
		allowedIds[idStr] = true
	}
	for _, id := range c.ids {
		if allowedIds[id.URL.String()] {
			continue
		}
		purl := id.PromulgatedURL()
		if purl != nil && allowedIds[purl.String()] {
			continue
		}
		return errgo.Newf("operation on entity %v not allowed", id)
	}
	return nil
}

// entityChannel returns the default channel that applies to
// the entity with the given id. If the request has explictly
// mentioned a channel, that channel is used; otherwise
// a channel will be selected from the channels that the
// entity has been published to: in order of preference,
// stable, development and unpublished.
func (h *ReqHandler) entityChannel(id *router.ResolvedURL) (params.Channel, error) {
	if h.Store.Channel != params.NoChannel {
		return h.Store.Channel, nil
	}
	entity, err := h.Cache.Entity(&id.URL, charmstore.FieldSelector("development", "stable"))
	if err != nil {
		if errgo.Cause(err) == params.ErrNotFound {
			return params.NoChannel, errgo.WithCausef(nil, params.ErrNotFound, "entity %q not found", id)
		}
		return params.NoChannel, errgo.Notef(err, "cannot retrieve entity %q for authorization", id)
	}
	var ch params.Channel
	switch {
	case entity.Stable:
		ch = params.StableChannel
	case entity.Development:
		ch = params.DevelopmentChannel
	default:
		ch = params.UnpublishedChannel
	}
	return ch, nil
}

// newMacaroon returns a new macaroon that allows only the given operations
// and can only be satisified when the user has authenticated with the
// identity manager and agreed to all the required terms.
//
// If shortTerm is true, the macaroon will be issued with a short lifespan.
func (h *ReqHandler) newMacaroon(
	allowedOps []string,
	ids []*router.ResolvedURL,
	requiredTerms []string,
	shortTerm bool,
) (*macaroon.Macaroon, error) {
	expiry := defaultMacaroonExpiry
	if shortTerm {
		expiry = shortTermMacaroonExpiry
	}

	caveats := make([]checkers.Caveat, 0, 5)
	caveats = append(caveats,
		checkers.NeedDeclaredCaveat(
			checkers.Caveat{
				Location:  h.Handler.config.IdentityLocation,
				Condition: "is-authenticated-user",
			},
			UsernameAttr,
		),
		checkers.AllowCaveat(allowedOps...),
		checkers.TimeBeforeCaveat(timeNow().Add(expiry)),
	)
	if len(requiredTerms) > 0 {
		// Terms are required, which means that we must restrict
		// the macaroon so it can only be used if those terms have
		// been agreed to and with the provided entity ids.
		//
		// In this case, we also use a short term expiry time,
		// because the macaroon is only of limited use.
		caveats = append(caveats,
			isEntityCaveat(ids),
			checkers.Caveat{
				Location:  h.Handler.config.TermsLocation,
				Condition: "has-agreed " + strings.Join(requiredTerms, " "),
			},
		)
	}
	return h.Store.Bakery.NewMacaroon("", nil, caveats)
}

var errNoCreds = errgo.New("missing HTTP auth header")

// parseCredentials parses the given request and returns the HTTP basic auth
// credentials included in its header.
func parseCredentials(req *http.Request) (username, password string, err error) {
	auth := req.Header.Get("Authorization")
	if auth == "" {
		return "", "", errNoCreds
	}
	parts := strings.Fields(auth)
	if len(parts) != 2 || parts[0] != "Basic" {
		return "", "", errgo.New("invalid HTTP auth header")
	}
	// Challenge is a base64-encoded "tag:pass" string.
	// See RFC 2617, Section 2.
	challenge, err := base64.StdEncoding.DecodeString(parts[1])
	if err != nil {
		return "", "", errgo.New("invalid HTTP auth encoding")
	}
	tokens := strings.SplitN(string(challenge), ":", 2)
	if len(tokens) != 2 {
		return "", "", errgo.New("invalid HTTP auth contents")
	}
	return tokens[0], tokens[1], nil
}

const condActiveTimeBefore = "active-time-before"

// activeTimeBeforeCaveat returns a caveat that will be satisfied
// before the given time; after the given time, it will only
// be satisfied if the macaroon is being renewed.
func activeTimeBeforeCaveat(t time.Time) checkers.Caveat {
	return checkers.Caveat{
		Condition: condActiveTimeBefore + " " + t.UTC().Format(time.RFC3339Nano),
	}
}

// aclSet represents a set of ACLs. A user is considered to be
// a part of the set if the user is a member of each of the
// set.acls elements.
type aclSet struct {
	readPublic  bool
	writePublic bool
	acls        []mongodoc.ACL
}

func newACLSet(cap int) *aclSet {
	return &aclSet{
		acls:        make([]mongodoc.ACL, 0, cap),
		readPublic:  true,
		writePublic: true,
	}
}

func (s *aclSet) add(acl mongodoc.ACL) {
	s.acls = append(s.acls, acl)
	s.readPublic = s.readPublic && isPublicACL(acl.Read)
	s.writePublic = s.writePublic && isPublicACL(acl.Write)
}

// check checks that the request with the given authorization
// is allowed to perform all the given operations with respect
// to all the ACLs in the set. It uses the allow function to check
// individual ACL membership.
func (s *aclSet) check(auth authorization, ops []string, allow func(user string, acl []string) (bool, error)) error {
	if auth.Admin {
		return nil
	}
	if auth.Username == "" {
		return errgo.New("no username declared")
	}
	if len(s.acls) == 0 {
		return errgo.New("no ACLs found to check")
	}
	logger.Infof("check username %q; ops %q; acls: %#v", auth.Username, ops, s.acls)
	for _, acl := range s.acls {
		for _, op := range ops {
			// TODO could be slightly more efficient here
			// because we'll do unnecessary calls to allow when
			// several of the operations use the same set
			// of ACLs.
			ok, err := allow(auth.Username, aclForOp(acl, op))
			if err != nil {
				return errgo.Mask(err)
			}
			if !ok {
				return errgo.Newf("access denied for user %q", auth.Username)
			}
			logger.Infof("%q allowed access through op %q, acl %q", auth.Username, op, aclForOp(acl, op))
		}
	}
	return nil
}

func aclForOp(acls mongodoc.ACL, op string) []string {
	switch op {
	case OpReadWithTerms, OpReadWithNoTerms:
		return acls.Read
	case OpWrite:
		return acls.Write
	}
	// Fail safe if we don't understand the operation.
	return nil
}

func isPublicACL(acl []string) bool {
	for _, u := range acl {
		if u == params.Everyone {
			return true
		}
	}
	return false
}

type noGroupCache struct{}

func (noGroupCache) Groups(username string) ([]string, error) {
	return nil, nil
}

type noGroupsPermChecker struct{}

func (noGroupsPermChecker) Allow(username string, acl []string) (bool, error) {
	for _, name := range acl {
		if name == username || name == params.Everyone {
			return true, nil
		}
	}
	return false, nil
}
