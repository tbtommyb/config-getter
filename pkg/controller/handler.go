package controller

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"

	api_v1 "k8s.io/api/core/v1"
)

type AnnotationGetter struct {
	Getter ResourceGetter
}

type ResourceGetter interface {
	Get(string) ([]byte, error)
}

func (cg *AnnotationGetter) Process(cm *api_v1.ConfigMap) (*api_v1.ConfigMap, error) {
	annotation, present := cm.GetAnnotations()["x-k8s-io/curl-me-that"]
	if !present {
		return nil, nil
	}

	key, url, err := parseAnnotation(annotation)
	if err != nil {
		return nil, fmt.Errorf("could not parse annotation %s: %w", annotation, err)
	}

	validatedURL, err := validateUrl(url)
	if err != nil {
		return nil, fmt.Errorf("could not parse URL %s: %w", url, err)
	}
	body, err := cg.Getter.Get(validatedURL)
	if err != nil {
		// TODO: replace with better error-handling
		return nil, fmt.Errorf("could not fetch %s", validatedURL)
	}

	updated := cm.DeepCopy()
	data := updated.Data
	if data == nil {
		updated.Data = make(map[string]string)
	}
	updated.Data[key] = string(body)

	return updated, nil
}

func parseAnnotation(annotation string) (string, string, error) {
	match, err := regexp.MatchString(`(\w+)=(\w+)`, annotation)
	if !match {
		return "", "", fmt.Errorf("annotation %s does not match format key=https://path.com", annotation)
	}
	if err != nil {
		return "", "", err
	}
	x := strings.Split(annotation, "=")
	return x[0], x[1], nil
}

func validateUrl(path string) (string, error) {
	if !(strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://")) {
		path = "https://" + path
	}

	u, err := url.Parse(path)
	if err != nil {
		return "", err
	}

	if u.Host == "" {
		return "", fmt.Errorf("no host in %v", u)
	}

	return u.String(), nil
}
