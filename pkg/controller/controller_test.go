package controller_test

import (
	"testing"
	"time"

	"github.com/tbtommyb/config-getter/pkg/controller"
	"github.com/tbtommyb/config-getter/pkg/getter"
	api_v1 "k8s.io/api/core/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes/fake"

	// "k8s.io/client-go/tools/cache"

	core "k8s.io/client-go/testing"
)

var (
	alwaysReady        = func() bool { return true }
	noResyncPeriodFunc = func() time.Duration { return 0 }
)

type ControllerTestCase struct {
	annotation      string
	path            string
	expectedActions []string
	expectedKey     string
}

type TestLogger struct{}

func (tl *TestLogger) Info(objects ...interface{})               {}
func (tl *TestLogger) Infof(msg string, objects ...interface{})  {}
func (tl *TestLogger) Errorf(msg string, objects ...interface{}) {}

func TestController(t *testing.T) {
	testCases := []ControllerTestCase{
		{
			annotation:      "x-k8s-io/curl-me-that",
			path:            "test=curl-a-joke.herokuapp.com",
			expectedActions: []string{"list", "watch", "update"},
			expectedKey:     "test",
		},
		{
			annotation:      "x-k8s-io/curl-me-that",
			path:            "test_key=http://curl-a-joke.herokuapp.com",
			expectedActions: []string{"list", "watch", "update"},
			expectedKey:     "test_key",
		},
		{
			annotation:      "x-k8s-io/incorrect-annotation",
			path:            "test_key=http://curl-a-joke.herokuapp.com",
			expectedActions: []string{"list", "watch"},
			expectedKey:     "",
		},
	}

	for _, testCase := range testCases {
		cm := makeConfigMap(testCase.annotation, testCase.path)

		handler := &controller.AnnotationGetter{Getter: getter.NewHTTPGetter()}
		clientset := fake.NewSimpleClientset(cm)
		informerFactory := informers.NewSharedInformerFactory(clientset, noResyncPeriodFunc())
		informer := informerFactory.Core().V1().ConfigMaps()

		c := controller.New(clientset, handler, informer.Informer(), &TestLogger{})
		c.HasSynced = alwaysReady
		stopCh := make(chan struct{})
		defer close(stopCh)

		go c.Run(stopCh)

		<-time.After(1 * time.Second)
		stopCh <- struct{}{}

		actions := clientset.Actions()

		if len(actions) != len(testCase.expectedActions) {
			t.Errorf("unexpected actions %#v", actions)
		}
		for i, action := range clientset.Actions() {
			if !action.Matches(testCase.expectedActions[i], "configmaps") {
				t.Errorf("expected action %s, got %s", testCase.expectedActions[i], action)
			}
		}

		if testCase.expectedKey == "" {
			return
		}
		actionConfigMap := actions[2].(core.UpdateAction).GetObject().(*api_v1.ConfigMap)
		if _, present := actionConfigMap.Data[testCase.expectedKey]; !present {
			t.Errorf("expected %s to be in data", testCase.expectedKey)
		}
	}

}
