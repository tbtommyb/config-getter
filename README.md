# config-getter

Get from ConfigMaps.

## Usage

Create a ConfigMap with the annotation `x-k8s-io/curl-me-that: key=value` e.g. `x-k8s-io/curl-me-that: joke=curl-a-joke.herokuapp.com`. config-getter will fetch the requested resource and add it to the ConfigMap's data section.

It will not re-fetch a resource if the key does not change.

The controller is currently intended to be run outside the cluster with: `go run cmd/config-getter/main.go`.

Test with: `go test -v ./...`.

## Open questions

1. How would you deploy your controller to a Kubernetes cluster

The controller can run in the user plane like any containerised application. I would Dockerise the controller and deploy it to a cluster as part of a Deployment.

2. Kubernetes is level-based as opposed to edge-triggered. What benefits does it bring in the context of your controller?

The controller does not need to worry about missing an event leading it into an invalid state. Resyncs ensure that the controller's understanding of desired state is correct (in case an event was missed). The ConfigMaps' resource version is checked to avoid unnecessary updates.

7. The content returned when curling URLs may be always different. How is it going to affect your controllers?

If we imagine that Kubernetes is constantly trying to take a step closer to the desired state then it is problematic if that desired state is changeable. I dealt with this by not performing an update if the ConfigMap's data already holds the specified key. The desired state is therefore assumed to be "the content returned by the specified path at the time the key was specified".

Another solution beyond the scope of the specification would be to add a timer to the annotation. This would change the definition of the desired state to be "the content returned by the specified path within the last x seconds". This would be useful if the intention of the controller was to introduce the up-to-date state of the resource into the cluster.

