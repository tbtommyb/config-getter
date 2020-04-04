package controller_test

import (
	"testing"

	"github.com/tbtommyb/config-getter/pkg/controller"
	api_v1 "k8s.io/api/core/v1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func makeConfigMap(key, value string) *api_v1.ConfigMap {
	m := make(map[string]string)
	m[key] = value
	return &api_v1.ConfigMap{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:        "test",
			Namespace:   "default",
			Annotations: m,
		},
	}
}

type TestResourceGetter struct{}

func (trg *TestResourceGetter) Get(url string) ([]byte, error) {
	return []byte("HTTP resource body"), nil
}

func TestProcessWithNonMatchingAnnotation(t *testing.T) {
	handler := &controller.AnnotationGetter{Getter: &TestResourceGetter{}}
	cm := makeConfigMap("test-annotation", "")

	actual, err := handler.Process(cm)

	if actual != nil || err != nil {
		t.Errorf("expected nil result and error")
	}
}
