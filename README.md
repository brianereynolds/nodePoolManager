# nodepoolmanager
Node Pool Manager is used to create, update and delete node pools on an AKS cluster.

## Description
Some applications cannot take full advantage of the Kubernetes high-availability concepts. This CRD has been designed to manage the node pools that run application workloads.

Based on the YAML you provide, the controller will bring the node pools to the state reflected in the configuration.

## Getting Started

### Prerequisites
- Access to a Azure Kubernetes Service 1.26+
- A Managed Identity with AKS Contributor Role for the cluster

### Deployment
Helm chart :
```
kubectl create ns operations
helm repo add k8smanagers https://k8smanagers.blob.core.windows.net/helm/
helm install nodepoolmanager k8smanagers/nodepoolmanager -n operations
```

#### Verification
```
helm list --filter 'nodepoolmanager' -n operations
kubectl get crd nodepoolmanagers.k8smanagers.greyridge.com
```

### Uninstall
Uninstall the helm chart
```
helm uninstall -n operations nodepoolmanager
```

## License

Copyright 2024.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.