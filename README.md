# kubectl-fields

`kubectl-fields` is a `kubectl` plugin that annotates Kubernetes manifests with the last manager of each field.

This helps you understand which controller is managing which part of your Kubernetes objects.


## Usage

Pipe the output of `kubectl get -o yaml --show-managed-fields` to `kubectl-fields`:

```sh
kubectl get deploy/my-app -o yaml --show-managed-fields | kubectl fields
```

### Example

Here's an example of the output for a Deployment object:

```yaml
apiVersion: apps/v1 # kube-controller-manager (2y ago)
kind: Deployment # kube-controller-manager (2y ago)
metadata: # kube-apiserver (2y ago)
  name: my-app # kube-apiserver (2y ago)
  namespace: default # kube-apiserver (2y ago)
spec: # kube-controller-manager (2y ago)
  replicas: 1 # kube-apiserver (2y ago)
  selector: # kube-controller-manager (2y ago)
    matchLabels: # kube-controller-manager (2y ago)
      app: my-app # kube-controller-manager (2y ago)
  template: # kube-controller-manager (2y ago)
    metadata: # kube-controller-manager (2y ago)
      labels: # kube-controller-manager (2y ago)
        app: my-app # kube-controller-manager (2y ago)
    spec: # kube-controller-manager (2y ago)
      containers: # kube-controller-manager (2y ago)
      - name: my-app # kube-controller-manager (2y ago)
        image: nginx # kube-apiserver (2y ago)
```

As you can see, `kubectl-fields` annotates each field with the controller that last modified it, along with a relative timestamp of when the change was made. This provides valuable insight into how your Kubernetes objects are being managed.
