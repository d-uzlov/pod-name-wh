
# Pod name webhook

Mutating webhook for k8s that modifies pod names to use node name as name suffix.

The intended use for this is to allow for stable names for daemonsets.
But it should work for any pods are bound to a certain node before scheduling.

Comparison of 2 DaemonSets, with and without relevant label:

```bash
$ kubectl -n test-pod-webhook get pod -o custom-columns=NAME:.metadata.name,NODE:.spec.nodeName
NAME                             NODE
my-deployment-68d8644f97-tvn5z   node2.domain
my-daemonset-node1.domain        node1.domain
my-daemonset-node2.domain        node2.domain
my-daemonset-node3.domain        node3.domain

$ kubectl -n test-vanilla get pod -o custom-columns=NAME:.metadata.name,NODE:.spec.nodeName
NAME                             NODE
my-deployment-6b8d4d8fcb-x2kxc   node2.domain
my-daemonset-6c5w6               node1.domain
my-daemonset-dsz8x               node2.domain
my-daemonset-hcnrq               node3.domain
```

The obvious consequence of using this webhook is that DaemonSet names are no longer random/unique.

This is very convenient, because for DaemonSet node name is very important,
and now human reader can infer it from pod name, without looking at extended properties of pod or metric.
This avoids cluttering monitoring system with many pods, whenever a DaemonSet needs to be configured or restarted.

This may be an issue in case you rely on pod names being unique.

However, there are other cases where pod names are stable: static pods and StatefulSets.
Also, some controllers sometimes use stable names. For example, CNPG's Cluster controller is similar to StatefulSet.
Open Kruise allows you to hot-replace containers using different images and several other parameters.
And even when using standard Deployment/DaemonSet containers in a pod can be recreated, after previous container finished with an error.
So your monitoring/alerting system already needs to be able to deal with pods with stable names,
so giving stable names to DaemonSet pods should not create any _new_ issues.

# Installation

This app needs a certificate setup for a webhook.
This example uses `cert-manager` to avoid creating certificate manually.
Make sure that cert-manager is installed: https://cert-manager.io/docs/installation/

Deploy the app in your cluster:

```bash

kubectl create ns pod-name-wh
kubectl label ns pod-name-wh pod-security.kubernetes.io/enforce=baseline

kubectl apply -f https://github.com/d-uzlov/pod-name-wh/raw/refs/heads/main/deployment/cert.yaml
kubectl apply -f https://github.com/d-uzlov/pod-name-wh/raw/refs/heads/main/deployment/deployment.yaml
kubectl apply -f https://github.com/d-uzlov/pod-name-wh/raw/refs/heads/main/deployment/webhook.yaml

# check that everything is working
kubectl -n pod-name-wh get pod -o wide

```

# Modifying name on a DaemonSet

Add a label to the pod template:

```yaml
metadata:
  labels:
    pod-name.meoe.io/mutate: enable
```

Note that this will only work for pods that either have `NodeName` specified,
or have `NodeAffinity` matching a single node.
If you add this label to a simple manually created pod, or to deployment (that does not specify affinity)
then pod creation will fail.

# Configuration

You can customize which part of the node name will be used as pod suffix.
Set `NODE_REGEX` env to appropriate value.
The first matching capture group will be used as pod suffix.

For example:

- `^(.*)$` (default): use whole node name
- `^(.*)\.domain$`: use domain prefix. If node name does not match the suffix, fail
- `^(node\.fqdn)$|^(.*)\.domain$`: if node name matches `node.fqdn`, use it, else use domain prefix

# Building

```bash

# local testing
CGO_ENABLED=0 go build .
./daemonset-name-webhook

# build image for deployment
docker build .

image_name=daemonset-name-wh:v0.1.5

docker_username=
docker build --push . -t docker.io/$docker_username/$image_name

github_username=
docker build --push . -t ghcr.io/$github_username/$image_name

```
