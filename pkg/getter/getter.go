package getter

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
)

type HTTPGetter struct{}

func NewHTTPGetter() *HTTPGetter {
	return &HTTPGetter{}
}

func (g *HTTPGetter) Get(url string) ([]byte, error) {
	validatedURL, err := validateUrl(url)
	if err != nil {
		return nil, fmt.Errorf("could not parse %s: %w", url, err)
	}

	resp, err := http.Get(validatedURL)
	if err != nil {
		// TODO: think about this. Store in CM somewhere
		return nil, fmt.Errorf("failed to fetch %s: %w", url, err)
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)

	return body, err
}

func validateUrl(path string) (string, error) {
	u, err := url.Parse(path)
	if err != nil {
		return "", err
	}
	if u.Path == "" {
		return "", fmt.Errorf("no path in %s", path)
	}
	if u.Scheme == "" {
		u.Scheme = "http"
	}

	return u.String(), nil
}
