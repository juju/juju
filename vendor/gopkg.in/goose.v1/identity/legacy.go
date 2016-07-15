package identity

import (
	"fmt"
	"io/ioutil"
	"net/http"

	goosehttp "gopkg.in/goose.v1/http"
)

type Legacy struct {
	client *goosehttp.Client
}

func (l *Legacy) Auth(creds *Credentials) (*AuthDetails, error) {
	if l.client == nil {
		l.client = goosehttp.New()
	}
	request, err := http.NewRequest("GET", creds.URL, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("X-Auth-User", creds.User)
	request.Header.Set("X-Auth-Key", creds.Secrets)
	response, err := l.client.Do(request)
	defer response.Body.Close()
	if err != nil {
		return nil, err
	}
	if response.StatusCode != http.StatusNoContent {
		content, _ := ioutil.ReadAll(response.Body)
		return nil, fmt.Errorf("Failed to Authenticate (code %d %s): %s",
			response.StatusCode, response.Status, content)
	}
	details := &AuthDetails{}
	details.Token = response.Header.Get("X-Auth-Token")
	if details.Token == "" {
		return nil, fmt.Errorf("Did not get valid Token from auth request")
	}
	details.RegionServiceURLs = make(map[string]ServiceURLs)
	serviceURLs := make(ServiceURLs)
	// Legacy authentication doesn't require a region so use "".
	details.RegionServiceURLs[""] = serviceURLs
	nova_url := response.Header.Get("X-Server-Management-Url")
	if nova_url == "" {
		return nil, fmt.Errorf("Did not get valid nova management URL from auth request")
	}
	serviceURLs["compute"] = nova_url

	swift_url := response.Header.Get("X-Storage-Url")
	if swift_url == "" {
		return nil, fmt.Errorf("Did not get valid swift management URL from auth request")
	}
	serviceURLs["object-store"] = swift_url

	return details, nil
}
