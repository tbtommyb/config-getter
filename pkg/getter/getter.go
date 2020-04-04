package getter

import (
	"fmt"
	"io/ioutil"
	"net/http"
)

type HTTPGetter struct{}

func NewHTTPGetter() *HTTPGetter {
	return &HTTPGetter{}
}

func (g *HTTPGetter) Get(url string) ([]byte, error) {
	resp, err := http.Get(url)
	if err != nil {
		// TODO: think about this. Store in CM somewhere
		return nil, fmt.Errorf("failed to fetch %s: %w", url, err)
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)

	return body, err
}
