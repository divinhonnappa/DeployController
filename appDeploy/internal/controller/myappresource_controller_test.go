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
	deployv1alpha1 "app-deploy/api/v1alpha1"
	"context"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"time"
)

var _ = Describe("MyAppResource Controller", func() {
	Context("When reconciling a resource", func() {
		const resourceName = "test-resource"
		const namespace = "default"

		ctx := context.Background()

		Context("Reconciling MyAppResource reading from CRD", func() {
			It("Will Reconcile MyAppResource", func() {
				By("Creating or Updating a MyAppResource object")
				key := types.NamespacedName{
					Name:      resourceName,
					Namespace: namespace,
				}
				resourceInstance := &deployv1alpha1.MyAppResource{
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceName,
						Namespace: namespace,
					},
					Spec: deployv1alpha1.MyAppResourceSpec{
						ReplicaCount: 2,
						Resources: deployv1alpha1.Resources{
							MemoryLimit: "64Mi",
							CPURequest:  "100m",
						},
						Image: deployv1alpha1.Image{
							Repository: "ghcr.io/stefanprodan/podinfo",
							Tag:        "latest",
						},
						Ui: deployv1alpha1.Ui{
							Color:   "#3374FF",
							Message: "App Message for the field",
						},
						Redis: deployv1alpha1.Redis{
							Enabled: true,
						},
					},
				}
				Expect(k8sClient.Create(ctx, resourceInstance)).Should(Succeed())

				By("Creating App Deployment, Redis Deployment and Redis Service")
				Eventually(func() error {
					appDeployment := &appsv1.Deployment{}
					err := k8sClient.Get(ctx, key, appDeployment)
					if err != nil {
						return err
					}

					redisDeployment := &appsv1.Deployment{}
					err = k8sClient.Get(ctx, types.NamespacedName{
						Name:      "redis",
						Namespace: namespace,
					}, redisDeployment)
					if err != nil {
						return err
					}

					redisSvc := &corev1.Service{}
					err = k8sClient.Get(ctx, types.NamespacedName{
						Name:      "redis",
						Namespace: namespace,
					}, redisSvc)
					if err != nil {
						return err
					}
					return nil
				}, time.Duration(40*time.Second), time.Duration(20*time.Second)).Should(Succeed())
			})
		})
	})
})
