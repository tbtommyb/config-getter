package controller_test

import (
	"reflect"
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
	return []byte(url), nil
}

func TestProcessWithNonMatchingAnnotation(t *testing.T) {
	handler := &controller.AnnotationGetter{Getter: &TestResourceGetter{}}
	cm := makeConfigMap("test-annotation", "")

	actual, err := handler.Process(cm)

	if actual != nil || err != nil {
		t.Errorf("expected nil result and error")
	}
}

func TestProcessWithInvalidAnnotations(t *testing.T) {
	handler := &controller.AnnotationGetter{Getter: &TestResourceGetter{}}

	testCases := []string{
		"",
		"this-is-not-valid",
		"token",
		"http://google.com",
	}

	for _, test := range testCases {
		cm := makeConfigMap("x-k8s-io/curl-me-that", test)
		_, err := handler.Process(cm)

		if err == nil {
			t.Errorf("expected error")
		}
	}
}

type validPathTestCase struct {
	input    string
	expected map[string]string
}

func TestProcessWithValidPaths(t *testing.T) {
	handler := &controller.AnnotationGetter{Getter: &TestResourceGetter{}}

	testCases := []validPathTestCase{
		{
			input:    "key=www.google.com",
			expected: map[string]string{"key": "https://www.google.com"},
		},
		{
			input:    "key=http://www.google.com",
			expected: map[string]string{"key": "http://www.google.com"},
		},
		{
			input:    "key=https://www.google.com",
			expected: map[string]string{"key": "https://www.google.com"},
		},
	}

	for _, testCase := range testCases {
		cm := makeConfigMap("x-k8s-io/curl-me-that", testCase.input)
		actual, err := handler.Process(cm)

		if err != nil {
			t.Errorf("unexpected error %v\n", err)
		}
		if !reflect.DeepEqual(testCase.expected, actual.Data) {
			t.Errorf("expected %#v, got %#v\n", testCase.expected, actual.Data)
		}
	}
}
