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
Helm chart is recommended:
```
cd charts/nodepoolmanager
helm install nodepoolmanager .
```

#### Verification
```
helm list --filter 'nodepoolmanager' 
kubectl get crd workloadmanagers.k8smanageers.greyridge.com
```

### To Uninstall
**Delete the instances (CRs) from the cluster:**

```sh
kubectl delete -k config/samples/
```

**Delete the APIs(CRDs) from the cluster:**

```sh
make uninstall
```

**UnDeploy the controller from the cluster:**

```sh
make undeploy
```

## Project Distribution

Following are the steps to build the installer and distribute this project to users.

1. Build the installer for the image built and published in the registry:

```sh
make build-installer IMG=<some-registry>/nodepoolmanager:tag
```

NOTE: The makefile target mentioned above generates an 'install.yaml'
file in the dist directory. This file contains all the resources built
with Kustomize, which are necessary to install this project without
its dependencies.

2. Using the installer

Users can just run kubectl apply -f <URL for YAML BUNDLE> to install the project, i.e.:

```sh
kubectl apply -f https://raw.githubusercontent.com/<org>/nodepoolmanager/<tag or branch>/dist/install.yaml
```

## Contributing
// TODO(user): Add detailed information on how you would like others to contribute to this project

**NOTE:** Run `make help` for more information on all potential `make` targets

More information can be found via the [Kubebuilder Documentation](https://book.kubebuilder.io/introduction.html)

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

