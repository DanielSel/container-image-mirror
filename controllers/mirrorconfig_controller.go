/*
Copyright 2020 Daniel Sel.

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
	"context"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	cimapi "github.com/danielsel/container-image-mirror/api"
)

// MirrorConfigReconciler reconciles a MirrorConfig object
type MirrorConfigReconciler struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=cim.zerocloud.io,resources=mirrorconfigs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=cim.zerocloud.io,resources=mirrorconfigs/status,verbs=get;update;patch

func (r *MirrorConfigReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	_ = context.Background()
	_ = r.Log.WithValues("mirrorconfig", req.NamespacedName)

	// your logic here

	return ctrl.Result{}, nil
}

func (r *MirrorConfigReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&cimapi.MirrorConfig{}).
		Complete(r)
}
