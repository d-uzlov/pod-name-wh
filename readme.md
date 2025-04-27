
# Pod name webhook

Mutating webhook for k8s that modifies pod names to use node name as name suffix.

The intended use for this is to allow for stable names for daemonsets.
But it should work for any pods are bound to a certain node before scheduling.

# Usage example

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

You can customize which part of the node name will be used as pod suffix.
Set `NODE_REGEX` env to appropriate value.
The first matching capture group will be used as pod suffix.

For example:

- `^(.*)$` (default): use whole node name
- `^(.*)\.domain$`: use domain prefix
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
