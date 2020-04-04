package controller

import (
	"fmt"
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
	requestedURL, present := cm.GetAnnotations()["x-k8s-io/curl-me-that"]
	if !present {
		return nil, nil
	}

	key, url, err := parseAnnotation(requestedURL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse %s: %w", requestedURL, err)
	}

	body, err := cg.Getter.Get(url)

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
