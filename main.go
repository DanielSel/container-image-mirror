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

package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"runtime"
	"strings"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	k8sRuntime "k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	ctrl "sigs.k8s.io/controller-runtime"
	ctrlzap "sigs.k8s.io/controller-runtime/pkg/log/zap"

	cimv1alpha1 "github.com/danielsel/container-image-mirror/api/v1alpha1"
	"github.com/danielsel/container-image-mirror/controllers"
	"github.com/danielsel/container-image-mirror/pkg/cim"
	"github.com/rs/xid"
	// +kubebuilder:scaffold:imports
)

var (
	scheme   = k8sRuntime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	_ = clientgoscheme.AddToScheme(scheme)

	_ = cimv1alpha1.AddToScheme(scheme)
	// +kubebuilder:scaffold:scheme
}

func main() {
	// Flag-based configuration
	// General
	var logLevel string
	var development bool
	var parallel uint
	var maxMemMb uint64
	flag.StringVar(&logLevel, "log-level", "info", "The verbosity of log outputs: [error, info, debug, verbose]")
	flag.BoolVar(&development, "development", false,
		"Enable local development mode. "+
			"Configures more readable logging.")
	flag.UintVar(&parallel, "parallel", 4*uint(runtime.NumCPU()), "Max. number of parallel copy jobs.")
	flag.Uint64Var(&maxMemMb, "memlimit", 6*1024, "Mex. allocated memory for copy jobs.")

	// Single Job
	var config string
	flag.StringVar(&config, "config", "", "Path to MirrorConfig .yaml file. ATTENTION: This will disable the scheduling and controller features of CIM, but allows it to run outside of k8s.")

	// Operator
	var metricsAddr string
	var enableLeaderElection bool
	flag.StringVar(&metricsAddr, "metrics-addr", ":8080", "The address the metric endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "enable-leader-election", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	// Execute
	flag.Parse()

	// Configure Logging
	lvl := zap.NewAtomicLevelAt(parseLogLevel(logLevel))
	optsFnc := func(opts *ctrlzap.Options) {
		opts.Level = &lvl
		opts.Development = development
	}
	ctrl.SetLogger(ctrlzap.New(optsFnc))

	// If config parameter is set -> runSingleJob - don't go into controller mode
	if config != "" {
		runSingleJob(config, parallel, maxMemMb*1024*1024)
	}

	// Operator config (kubebuilder generated)
	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:             scheme,
		MetricsBindAddress: metricsAddr,
		Port:               9443,
		LeaderElection:     enableLeaderElection,
		LeaderElectionID:   "c81e1f11.zerocloud.io",
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	if err = (&controllers.MirrorConfigReconciler{
		Client: mgr.GetClient(),
		Log:    ctrl.Log.WithName("controllers").WithName("MirrorConfig"),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "MirrorConfig")
		os.Exit(1)
	}
	// +kubebuilder:scaffold:builder

	setupLog.Info("starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}

func runSingleJob(configPath string, maxParallelCopy uint, maxMem uint64) {
	jobId := newReqId()
	ctx := context.WithValue(context.Background(), "JobId", jobId)
	log := ctrl.Log.WithName("CIM").WithValues("JobId", jobId)
	log.Info("Running single one-shot job for MirrorConfig supplied using --config",
		"config", configPath)
	cfg, err := cim.MirrorConfigFromFile(configPath)
	if err != nil {
		log.Error(err, "error reading MirrorConfig file",
			"config", configPath)
		os.Exit(1)
	}

	// Runner config
	cim.ConfigNumParallelCopyJobs = maxParallelCopy
	cim.ConfigMaxMemBytes = maxMem

	timerStart := time.Now()
	if err := cim.Run(cfg, ctx, log); err != nil {
		log.Error(err, "mirror job failed",
			"config", configPath)
		os.Exit(1)
	}
	timerEnd := time.Now()
	executionTime := timerEnd.Sub(timerStart)
	log.Info(fmt.Sprintf("Successfully finished. Execution time: %s", executionTime),
		"ExecutionTime", executionTime)
	os.Exit(0)
}

func parseLogLevel(level string) zapcore.Level {
	var logrLvl = 0
	switch {
	case strings.EqualFold(level, "error"):
		logrLvl = -2
	case strings.EqualFold(level, "info"):
		logrLvl = 0
	case strings.EqualFold(level, "debug"):
		logrLvl = 1
	case strings.EqualFold(level, "verbose"):
		logrLvl = 2
	}
	// Zap uses inverse log levels from logr
	return zapcore.Level(logrLvl * -1)
}

func newReqId() string {
	return xid.New().String()
}
