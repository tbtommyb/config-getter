package main

import (
	"os"
	"os/signal"
	"syscall"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	controller "github.com/tbtommyb/config-getter/pkg/controller"
)

func main() {
	config, err := rest.InClusterConfig()
	if err != nil {
		panic(err.Error())
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		panic(err.Error())
	}

	cont := controller.New(clientset)
	stopCh := make(chan struct{})
	defer close(stopCh)

	go cont.Run(stopCh)

	sigterm := make(chan os.Signal, 1)
	signal.Notify(sigterm, syscall.SIGTERM)
	signal.Notify(sigterm, syscall.SIGINT)
	<-sigterm
}
