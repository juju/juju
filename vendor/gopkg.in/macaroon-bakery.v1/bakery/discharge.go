package bakery

import (
	"fmt"

	"gopkg.in/errgo.v1"
	"gopkg.in/macaroon.v1"

	"gopkg.in/macaroon-bakery.v1/bakery/checkers"
)

// DischargeAll gathers discharge macaroons for all the third party
// caveats in m (and any subsequent caveats required by those) using
// getDischarge to acquire each discharge macaroon. It returns a slice
// with m as the first element, followed by all the discharge macaroons.
// All the discharge macaroons will be bound to the primary macaroon.
func DischargeAll(
	m *macaroon.Macaroon,
	getDischarge func(firstPartyLocation string, cav macaroon.Caveat) (*macaroon.Macaroon, error),
) (macaroon.Slice, error) {
	return DischargeAllWithKey(m, getDischarge, nil)
}

// DischargeAllWithKey is like DischargeAll except that the localKey
// parameter may optionally hold the key of the client, in which case it
// will be used to discharge any third party caveats with the special
// location "local". In this case, the caveat itself must be "true". This
// can be used be a server to ask a client to prove ownership of the
// private key.
//
// When localKey is nil, DischargeAllWithKey is exactly the same as
// DischargeAll.
func DischargeAllWithKey(
	m *macaroon.Macaroon,
	getDischarge func(firstPartyLocation string, cav macaroon.Caveat) (*macaroon.Macaroon, error),
	localKey *KeyPair,
) (macaroon.Slice, error) {
	sig := m.Signature()
	discharges := macaroon.Slice{m}
	var need []macaroon.Caveat
	addCaveats := func(m *macaroon.Macaroon) {
		for _, cav := range m.Caveats() {
			if cav.Location == "" {
				continue
			}
			need = append(need, cav)
		}
	}
	addCaveats(m)
	firstPartyLocation := m.Location()
	for len(need) > 0 {
		cav := need[0]
		need = need[1:]
		var dm *macaroon.Macaroon
		var err error
		if localKey != nil && cav.Location == "local" {
			dm, _, err = Discharge(localKey, localDischargeChecker, cav.Id)
		} else {
			dm, err = getDischarge(firstPartyLocation, cav)
		}
		if err != nil {
			return nil, errgo.NoteMask(err, fmt.Sprintf("cannot get discharge from %q", cav.Location), errgo.Any)
		}
		dm.Bind(sig)
		discharges = append(discharges, dm)
		addCaveats(dm)
	}
	return discharges, nil
}

var localDischargeChecker = ThirdPartyCheckerFunc(func(caveatId, caveat string) ([]checkers.Caveat, error) {
	if caveat != "true" {
		return nil, checkers.ErrCaveatNotRecognized
	}
	return nil, nil
})
