package controller

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"

	api_v1 "k8s.io/api/core/v1"
)

// AnnotationHandler represents an entity that processes the annotation
type AnnotationHandler struct {
	Getter ResourceGetter
}

// ResourceGetter gets the specified resource
type ResourceGetter interface {
	Get(string) ([]byte, error)
}

// Process processes the config map's annotation
func (cg *AnnotationHandler) Process(cm *api_v1.ConfigMap) (*api_v1.ConfigMap, error) {
	annotation, present := cm.GetAnnotations()[controllerAnnotation]
	if !present {
		return nil, nil
	}

	key, url, err := parseAnnotation(annotation)
	if err != nil {
		return nil, fmt.Errorf("error parse annotation %s: %w", annotation, err)
	}

	if _, exists := cm.Data[key]; exists {
		return nil, nil
	}

	validatedURL, err := validateURL(url)
	if err != nil {
		return nil, fmt.Errorf("error parse URL %s: %w", url, err)
	}
	body, err := cg.Getter.Get(validatedURL)
	if err != nil {
		return nil, err
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

func validateURL(path string) (string, error) {
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
