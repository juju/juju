package idservice

import (
	"encoding/json"
	"fmt"

	"gopkg.in/macaroon-bakery.v1/bakery/example/meeting"
)

type thirdPartyCaveatInfo struct {
	CaveatId string
	Caveat   string
}

type loginInfo struct {
	User string
	// TODO(rog) add error here for when the login fails.
}

// place layers our desired types onto the general meeting.Place,
type place struct {
	place *meeting.Place
}

func (p *place) NewRendezvous(info *thirdPartyCaveatInfo) (string, error) {
	reqData, err := json.Marshal(info)
	if err != nil {
		return "", fmt.Errorf("cannot marshal reqData: %v", err)
	}
	return p.place.NewRendezvous(reqData)
}

func (p *place) Done(waitId string, info *loginInfo) error {
	data, err := json.Marshal(info)
	if err != nil {
		return fmt.Errorf("cannot marshal loginData: %v", err)
	}
	return p.place.Done(waitId, data)
}

func (p *place) Wait(waitId string) (*thirdPartyCaveatInfo, *loginInfo, error) {
	reqData, loginData, err := p.place.Wait(waitId)
	if err != nil {
		return nil, nil, fmt.Errorf("cannot wait: %v", err)
	}
	var caveat thirdPartyCaveatInfo
	if err := json.Unmarshal(reqData, &caveat); err != nil {
		return nil, nil, fmt.Errorf("cannot unmarshal reqData: %v", err)
	}
	var login loginInfo
	if err := json.Unmarshal(loginData, &login); err != nil {
		return nil, nil, fmt.Errorf("cannot unmarshal loginData: %v", err)
	}
	return &caveat, &login, nil
}
