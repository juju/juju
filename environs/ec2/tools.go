package ec2

import (
	"net/http"
	"fmt"
)

// fetchMetadata fetches a single atom of data from the ec2 instance metadata service.
// http://docs.amazonwebservices.com/AWSEC2/latest/UserGuide/AESDG-chapter-instancedata.html
func fetchMetadata(name string) (string, error) {
	resp, err := http.Get(fmt.Sprintf("http://169.254.169.254/latest/meta-data/%s", name))
	if err != nil { return "", err }
	defer resp.Body.Close()
	var v string
	n, err := fmt.Fscanln(resp.Body, &v)
	if n != 1 || err != nil {
		return "", err 	
	}
	return v, nil
}
