/*
Copyright 2021.
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

package controllers

import (
	"reflect"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"context"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	cachev1alpha1 "github.com/pkoska-corona/labappcleaner-operator/api/v1"
)

// LabappCleanerReconciler reconciles a LabappCleaner object
type LabappCleanerReconciler struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme
}

// generate rbac to get, list, watch, create, update and patch the labappcleaner status the nencached resource
// +kubebuilder:rbac:groups=cache.example.com,resources=labappcleaners,verbs=get;list;watch;create;update;patch;delete

// generate rbac to get, update and patch the labappcleaner status the labappcleaner/finalizers
// +kubebuilder:rbac:groups=cache.example.com,resources=labappcleaners/status,verbs=get;update;patch

// generate rbac to update the labappcleaner/finalizers
// +kubebuilder:rbac:groups=cache.example.com,resources=labappcleaners/finalizers,verbs=update

// generate rbac to get, list, watch, create, update, patch, and delete deployments
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete

// generate rbac to get,list, and watch pods
// +kubebuilder:rbac:groups=core,resources=pods,verbs=get;list;watch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the LabappCleaner object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.7.0/pkg/reconcile
func (r *LabappCleanerReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := r.Log.WithValues("labappcleaner", req.NamespacedName)

	// Fetch the LabappCleaner instance
	labappcleaner := &cachev1alpha1.LabappCleaner{}
	err := r.Get(ctx, req.NamespacedName, labappcleaner)
	if err != nil {
		if errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			log.Info("LabappCleaner resource not found. Ignoring since object must be deleted")
			return ctrl.Result{}, nil
		}
		// Error reading the object - requeue the request.
		log.Error(err, "Failed to get LabappCleaner")
		return ctrl.Result{}, err
	}

	// Check if the deployment already exists, if not create a new one
	found := &appsv1.Deployment{}
	err = r.Get(ctx, req.NamespacedName, found)
	if err != nil && errors.IsNotFound(err) {
		// Define a new deployment
		dep := r.deploymentForLabappCleaner(labappcleaner)
		log.Info("Creating a new Deployment", "Deployment.Namespace", dep.Namespace, "Deployment.Name", dep.Name)
		err = r.Create(ctx, dep)
		if err != nil {
			log.Error(err, "Failed to create new Deployment", "Deployment.Namespace", dep.Namespace, "Deployment.Name", dep.Name)
			return ctrl.Result{}, err
		}
		// Deployment created successfully - return and requeue
		return ctrl.Result{Requeue: true}, nil
	} else if err != nil {
		log.Error(err, "Failed to get Deployment")
		return ctrl.Result{}, err
	}

	// Ensure the deployment size is the same as the spec
	size := labappcleaner.Spec.Count
	if *found.Spec.Replicas != size {
		found.Spec.Replicas = &size
		err = r.Update(ctx, found)
		if err != nil {
			log.Error(err, "Failed to update Deployment", "Deployment.Namespace", found.Namespace, "Deployment.Name", found.Name)
			return ctrl.Result{}, err
		}
		// Spec updated - return and requeue
		return ctrl.Result{Requeue: true}, nil
	}

	// Update the LabappCleaner status with the pod names
	// List the pods for this labappcleaner's deployment
	podList := &corev1.PodList{}
	listOpts := []client.ListOption{
		client.InNamespace(labappcleaner.Namespace),
		client.MatchingLabels(labelsForLabappCleaner(labappcleaner.Name)),
	}
	if err = r.List(ctx, podList, listOpts...); err != nil {
		log.Error(err, "Failed to list pods", "LabappCleaner.Namespace", labappcleaner.Namespace, "LabappCleaner.Name", labappcleaner.Name)
		return ctrl.Result{}, err
	}
	podNames := getPodNames(podList.Items)

	// Update status.Nodes if needed
	if !reflect.DeepEqual(podNames, labappcleaner.Status.Nodes) {
		labappcleaner.Status.Nodes = podNames
		err := r.Status().Update(ctx, labappcleaner)
		if err != nil {
			log.Error(err, "Failed to update LabappCleaner status")
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{}, nil
}

// deploymentForLabappCleaner returns a labappcleaner Deployment object
func (r *LabappCleanerReconciler) deploymentForLabappCleaner(m *cachev1alpha1.LabappCleaner) *appsv1.Deployment {
	ls := labelsForLabappCleaner(m.Name)
	replicas := m.Spec.Count

	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      m.Name,
			Namespace: m.Namespace,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: ls,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: ls,
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Image:   "memcached:1.4.36-alpine",
						Name:    "labappcleaner",
						Command: []string{"memcached", "-m=64", "-o", "modern", "-v"},
						Ports: []corev1.ContainerPort{{
							ContainerPort: 11211,
							Name:          "labappcleaner",
						}},
					}},
				},
			},
		},
	}
	// Set LabappCleaner instance as the owner and controller
	ctrl.SetControllerReference(m, dep, r.Scheme)
	return dep
}

// labelsForLabappCleaner returns the labels for selecting the resources
// belonging to the given labappcleaner CR name.
func labelsForLabappCleaner(name string) map[string]string {
	return map[string]string{"app": "labappcleaner", "labappcleaner_cr": name}
}

// getPodNames returns the pod names of the array of pods passed in
func getPodNames(pods []corev1.Pod) []string {
	var podNames []string
	for _, pod := range pods {
		podNames = append(podNames, pod.Name)
	}
	return podNames
}

// SetupWithManager sets up the controller with the Manager.
func (r *LabappCleanerReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&cachev1alpha1.LabappCleaner{}).
		Owns(&appsv1.Deployment{}).
		Complete(r)
}
