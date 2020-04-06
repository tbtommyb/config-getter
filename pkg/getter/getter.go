package getter

import (
	"io/ioutil"
	"net/http"
)

// HTTPGetter fetches the resource over HTTP
type HTTPGetter struct{}

// NewHTTPGetter creates a new HTTPGetter
func NewHTTPGetter() *HTTPGetter {
	return &HTTPGetter{}
}

// Get gets the resource
func (g *HTTPGetter) Get(url string) ([]byte, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)

	return body, err
}
