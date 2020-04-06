package controller_test

import (
	"strings"
	"testing"
	"time"

	"github.com/tbtommyb/config-getter/pkg/controller"
	"github.com/tbtommyb/config-getter/pkg/getter"
	api_v1 "k8s.io/api/core/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/tools/record"

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

		handler := &controller.AnnotationHandler{Getter: getter.NewHTTPGetter()}
		clientset := fake.NewSimpleClientset(cm)
		informerFactory := informers.NewSharedInformerFactory(clientset, noResyncPeriodFunc())
		informer := informerFactory.Core().V1().ConfigMaps()
		recorder := record.NewFakeRecorder(20)

		c := controller.New(clientset, handler, informer.Informer(), &TestLogger{}, recorder)
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

		events := collectEvents(recorder.Events)

		if len(events) != 0 {
			t.Errorf("unexpected events %#v\n", events)
		}
	}

}

func TestControllerEvents(t *testing.T) {
	testCase := &ControllerTestCase{
		annotation:      "x-k8s-io/curl-me-that",
		path:            "test=https://notarealwebsite12346.com",
		expectedActions: []string{"list", "watch", "update"},
		expectedKey:     "test",
	}
	expectedEventCount := 6

	cm := makeConfigMap(testCase.annotation, testCase.path)
	handler := &controller.AnnotationHandler{Getter: getter.NewHTTPGetter()}
	clientset := fake.NewSimpleClientset(cm)
	informerFactory := informers.NewSharedInformerFactory(clientset, noResyncPeriodFunc())
	informer := informerFactory.Core().V1().ConfigMaps()
	recorder := record.NewFakeRecorder(20)

	c := controller.New(clientset, handler, informer.Informer(), &TestLogger{}, recorder)
	c.HasSynced = alwaysReady
	stopCh := make(chan struct{})
	defer close(stopCh)

	go c.Run(stopCh)

	<-time.After(1 * time.Second)
	stopCh <- struct{}{}

	events := collectEvents(recorder.Events)

	if len(events) != expectedEventCount {
		t.Errorf("expected %d events, got %d\n", expectedEventCount, len(events))
	}

	for _, event := range events {
		if !strings.Contains(event, "GetFailure") {
			t.Errorf("expected GetFailure error, got %s\n", event)
		}
	}
}

func collectEvents(source <-chan string) []string {
	done := false
	events := make([]string, 0)
	for !done {
		select {
		case event := <-source:
			events = append(events, event)
		default:
			done = true
		}
	}
	return events
}
