package getter

import (
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
		return nil, err
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)

	return body, err
}
