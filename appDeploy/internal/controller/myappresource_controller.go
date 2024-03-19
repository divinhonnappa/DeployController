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
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	deployv1alpha1 "app-deploy/api/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
)

// MyAppResourceReconciler reconciles a MyAppResource object
type MyAppResourceReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=deploy.my.api.group,resources=myappresources,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=deploy.my.api.group,resources=myappresources/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=deploy.my.api.group,resources=myappresources/finalizers,verbs=update
//+kubebuilder:rbac:groups="apps",resources=deployments,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups="",resources=services;pods,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// the MyAppResource object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.17.0/pkg/reconcile

// Set default environment variables from crd for app pod
func SetEnvVars(appResource *deployv1alpha1.MyAppResource) []corev1.EnvVar {
	return []corev1.EnvVar{
		{
			// Hardcoded keys. TO:DO Collect list of key-value data from CRD and iterate to create env vars
			Name:  "PODINFO_UI_COLOR",
			Value: appResource.Spec.Ui.Color,
		},
		{
			Name:  "PODINFO_UI_MESSAGE",
			Value: appResource.Spec.Ui.Message,
		},
	}
}

// Hardcoded redis config for the purpose of assignment
func ConfigureRedis(e *[]corev1.EnvVar, request ctrl.Request, redisEnabled bool) (*corev1.Service, *appsv1.Deployment) {
	// Update env vars with redis service config
	if redisEnabled {
		// Hardcoded value based on default redis configuration
		*e = append(*e, corev1.EnvVar{
			Name:  "PODINFO_CACHE_SERVER",
			Value: "tcp://redis:6379",
		})
	}

	// Create redis deployment and service
	redisReplicas := int32(1)
	redisDeploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "redis",
			Namespace: request.Namespace,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &redisReplicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": "redis",
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app": "redis",
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "redis",
							Image: "redis:latest",
							Ports: []corev1.ContainerPort{
								{
									ContainerPort: 6379, // Redis default port
								},
							},
						},
					},
				},
			},
		},
	}
	// redis service configuration
	redisSvc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "redis",
			Namespace: request.Namespace,
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{
				"app": "redis",
			},
			Ports: []corev1.ServicePort{
				{
					Port:       6379,
					TargetPort: intstr.FromInt32(6379),
				},
			},
		},
	}
	return redisSvc, redisDeploy
}

// Reconcile TO:DO add retry logic to reconcile if facing errors when resources are in middle of change operation
func (r *MyAppResourceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	l := log.FromContext(ctx)
	l.Info("Started custom controller reconciler")
	myAppResource := &deployv1alpha1.MyAppResource{}
	err := r.Get(ctx, req.NamespacedName, myAppResource)
	if err != nil {
		return reconcile.Result{}, err
	}

	// Set env vars
	envVars := SetEnvVars(myAppResource)

	// Redis configurations
	redisService, redisDeploy := ConfigureRedis(&envVars, req, myAppResource.Spec.Redis.Enabled)

	if myAppResource.Spec.Redis.Enabled {
		// Set MyAppResource instance as the owner of the redis Service and deployment
		if err := controllerutil.SetControllerReference(myAppResource, redisService, r.Scheme); err != nil {
			return reconcile.Result{}, err
		}
		if err := controllerutil.SetControllerReference(myAppResource, redisDeploy, r.Scheme); err != nil {
			return reconcile.Result{}, err
		}
	}

	// Check if the service already exists, if not create a new one based on enable flag for redis
	existingRedisService := &corev1.Service{}
	err = r.Get(ctx, client.ObjectKey{Name: redisService.Name, Namespace: redisService.Namespace}, existingRedisService)
	if err != nil && client.IgnoreNotFound(err) != nil {
		return reconcile.Result{}, err
	}

	// Update/create/delete redis svc based on enabled flag
	if err == nil {
		existingRedisService.Spec = redisService.Spec
		if myAppResource.Spec.Redis.Enabled {
			err = r.Update(ctx, existingRedisService)
		} else {
			err = r.Delete(ctx, existingRedisService)
		}
		if err != nil {
			return reconcile.Result{}, err
		}

	} else if myAppResource.Spec.Redis.Enabled {
		err = r.Create(ctx, redisService)
		if err != nil {
			return reconcile.Result{}, err
		}
	}

	// Check if the deployment already exists, if not create a new one
	existingRedisDeployment := &appsv1.Deployment{}
	err = r.Get(ctx, client.ObjectKey{Name: redisDeploy.Name, Namespace: redisDeploy.Namespace}, existingRedisDeployment)
	if err != nil && client.IgnoreNotFound(err) != nil {
		return reconcile.Result{}, err
	}

	// Update/create/delete redis deployment based on enabled flag
	if err == nil {
		existingRedisDeployment.Spec = redisDeploy.Spec
		if myAppResource.Spec.Redis.Enabled {
			err = r.Update(ctx, existingRedisDeployment)
		} else {
			err = r.Delete(ctx, existingRedisDeployment)
		}
		if err != nil {
			return reconcile.Result{}, err
		}
	} else if myAppResource.Spec.Redis.Enabled {
		err = r.Create(ctx, redisDeploy)
		if err != nil {
			return reconcile.Result{}, err
		}
	}

	// Application deployment configuration
	appDeployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      req.Name,
			Namespace: req.Namespace,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &myAppResource.Spec.ReplicaCount,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": req.Name,
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app": req.Name,
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  req.Name,
							Image: myAppResource.Spec.Image.Repository + ":" + myAppResource.Spec.Image.Tag,
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse(myAppResource.Spec.Resources.CPURequest),
									corev1.ResourceMemory: resource.MustParse(myAppResource.Spec.Resources.MemoryLimit),
								},
							},
							Env: envVars,
						},
					},
				},
			},
		},
	}

	// Set MyAppResource instance as the owner of the app deployment
	if err := controllerutil.SetControllerReference(myAppResource, appDeployment, r.Scheme); err != nil {
		return reconcile.Result{}, err
	}

	// Check if the deployment already exists, if not create a new one
	existingAppDeployment := &appsv1.Deployment{}
	err = r.Get(ctx, client.ObjectKey{Name: appDeployment.Name, Namespace: appDeployment.Namespace}, existingAppDeployment)
	if err != nil && client.IgnoreNotFound(err) != nil {
		return reconcile.Result{}, err
	}

	if err == nil {
		existingAppDeployment.Spec = appDeployment.Spec
		err = r.Update(ctx, existingAppDeployment)
	} else {
		err = r.Create(ctx, appDeployment)
	}
	if err != nil {
		return reconcile.Result{}, err
	}
	l.Info("Completed custom controller reconciler")
	return reconcile.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *MyAppResourceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&deployv1alpha1.MyAppResource{}).
		Owns(&appsv1.Deployment{}).Owns(&corev1.Service{}).
		WithEventFilter(predicate.GenerationChangedPredicate{}).
		Complete(r)
}

var _ reconcile.Reconciler = &MyAppResourceReconciler{}
