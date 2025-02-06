/*
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
*/

package controller

import (
	"context"
	coreerrors "errors"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/containerservice/armcontainerservice"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/zap/zapcore"
	k8smanagersv1 "greyridge.com/nodePoolManager/api/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var _ = Describe("NodePoolManager Controller", func() {
	Context("When reconciling a resource", func() {
		const resourceName = "test-resource"

		ctx := context.Background()

		zapLogger := zap.New(zap.UseDevMode(true), zap.Level(zapcore.InfoLevel))
		log.SetLogger(zapLogger)

		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: "default",
		}
		nodepoolmanager := &k8smanagersv1.NodePoolManager{}

		BeforeEach(func() {
			By("creating the default custom resource for the Kind NodePoolManager")
			err := k8sClient.Get(ctx, typeNamespacedName, nodepoolmanager)
			if err != nil && errors.IsNotFound(err) {
				resource := &k8smanagersv1.NodePoolManager{
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceName,
						Namespace: "default",
					},
					Spec: k8smanagersv1.NodePoolManagerSpec{
						SubscriptionID: "3e54eb54-946e-4ff4-a430-d7b190cd45cf",
						ResourceGroup:  "node-upgrader",
						ClusterName:    "lm-cluster",
						RetryOnError:   false,
						TestMode:       false,
						NodePools:      []k8smanagersv1.NodePool{},
					},
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}
		})

		AfterEach(func() {
			resource := &k8smanagersv1.NodePoolManager{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			Expect(err).NotTo(HaveOccurred())

			By("Cleanup the specific resource instance NodePoolManager")
			Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
		})

		It("should successfully reconcile the resource", func() {
			By("Reconciling the created resource")
			controllerReconciler := &NodePoolManagerReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})

			Expect(err).NotTo(HaveOccurred())
		})

		// This test assumes AKS 1.29.0 (or greater)
		It("Create/Update/Delete NP test", func() {
			resource := &k8smanagersv1.NodePoolManager{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			Expect(err).NotTo(HaveOccurred())

			var zero int32 = 0
			var az []*string

			props := armcontainerservice.ManagedClusterAgentPoolProfileProperties{
				OrchestratorVersion: to.Ptr("1.28.5"),
				VMSize:              to.Ptr("Standard_DS2_v2"),
				EnableAutoScaling:   to.Ptr(true),
				MinCount:            &zero,
				MaxCount:            &zero,
				AvailabilityZones:   az,
			}
			np := k8smanagersv1.NodePool{
				Name:  "testnp",
				Props: props,
			}
			resource.Spec.NodePools = append(resource.Spec.NodePools, np)

			controllerReconciler := &NodePoolManagerReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			Expect(k8sClient.Update(ctx, resource)).To(Succeed())

			// Create NP
			_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			// Update NP
			resource.Spec.NodePools[0].Props.OrchestratorVersion = to.Ptr("1.29.0")
			Expect(k8sClient.Update(ctx, resource)).To(Succeed())
			_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			// Delete NP
			resource.Spec.NodePools[0].Action = k8smanagersv1.Delete
			Expect(k8sClient.Update(ctx, resource)).To(Succeed())
			_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

		})

		It("Negative Test: Create NP with invalid name", func() {

			resource := &k8smanagersv1.NodePoolManager{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			Expect(err).NotTo(HaveOccurred())

			var zero int32 = 0
			var az []*string

			props := armcontainerservice.ManagedClusterAgentPoolProfileProperties{
				OrchestratorVersion: to.Ptr("1.28.5"),
				VMSize:              to.Ptr("Standard_DS2_v2"),
				EnableAutoScaling:   to.Ptr(true),
				MinCount:            &zero,
				MaxCount:            &zero,
				AvailabilityZones:   az,
			}

			np := k8smanagersv1.NodePool{
				Name:  "test-np",
				Props: props,
			}
			resource.Spec.NodePools = append(resource.Spec.NodePools, np)

			controllerReconciler := &NodePoolManagerReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			Expect(k8sClient.Update(ctx, resource)).To(Succeed())

			// Create NP
			_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).To(HaveOccurred())
			Expect(err).To(MatchError(coreerrors.New("nodepool[].name must only be lowercase letter and numbers")))
		})

		It("Negative Test: Create NP with invalid version", func() {
			resource := &k8smanagersv1.NodePoolManager{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			Expect(err).NotTo(HaveOccurred())

			var zero int32 = 0
			var az []*string

			props := armcontainerservice.ManagedClusterAgentPoolProfileProperties{
				OrchestratorVersion: to.Ptr("1.28"),
				VMSize:              to.Ptr("Standard_DS2_v2"),
				EnableAutoScaling:   to.Ptr(true),
				MinCount:            &zero,
				MaxCount:            &zero,
				AvailabilityZones:   az,
			}

			np := k8smanagersv1.NodePool{
				Name:  "testnp",
				Props: props,
			}
			resource.Spec.NodePools = append(resource.Spec.NodePools, np)

			controllerReconciler := &NodePoolManagerReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}
			Expect(k8sClient.Update(ctx, resource)).To(Succeed())

			// Create NP
			_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).To(HaveOccurred())
			Expect(err).To(MatchError(coreerrors.New("version numbers must be of the format V.v.n")))
		})

		It("Negative Test: Create NP without properties", func() {
			resource := &k8smanagersv1.NodePoolManager{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			Expect(err).NotTo(HaveOccurred())

			np := k8smanagersv1.NodePool{
				Name: "testnp",
			}
			resource.Spec.NodePools = append(resource.Spec.NodePools, np)

			controllerReconciler := &NodePoolManagerReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}
			Expect(k8sClient.Update(ctx, resource)).To(Succeed())

			// Create NP
			_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).To(HaveOccurred())
			Expect(err).To(MatchError(coreerrors.New("nodepool[].properties.VMSize is mandatory when creating node pools")))
		})
	})
})
