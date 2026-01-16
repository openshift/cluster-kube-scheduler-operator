package operator

import (
	"context"
	"fmt"
	"os"
	"time"

	configv1 "github.com/openshift/api/config/v1"
	operatorv1 "github.com/openshift/api/operator/v1"
	configv1client "github.com/openshift/client-go/config/clientset/versioned"
	configv1informers "github.com/openshift/client-go/config/informers/externalversions"
	applyoperatorv1 "github.com/openshift/client-go/operator/applyconfigurations/operator/v1"
	"github.com/openshift/cluster-kube-scheduler-operator/bindata"
	"github.com/openshift/cluster-kube-scheduler-operator/pkg/operator/configmetrics"
	"github.com/openshift/cluster-kube-scheduler-operator/pkg/operator/configobservation/configobservercontroller"
	"github.com/openshift/cluster-kube-scheduler-operator/pkg/operator/operatorclient"
	"github.com/openshift/cluster-kube-scheduler-operator/pkg/operator/resourcesynccontroller"
	"github.com/openshift/cluster-kube-scheduler-operator/pkg/operator/targetconfigcontroller"
	"github.com/openshift/library-go/pkg/controller/controllercmd"
	"github.com/openshift/library-go/pkg/operator/configobserver/featuregates"
	"github.com/openshift/library-go/pkg/operator/genericoperatorclient"
	"github.com/openshift/library-go/pkg/operator/resource/resourceapply"
	"github.com/openshift/library-go/pkg/operator/staticpod"
	"github.com/openshift/library-go/pkg/operator/staticpod/controller/common"
	"github.com/openshift/library-go/pkg/operator/staticpod/controller/installer"
	"github.com/openshift/library-go/pkg/operator/staticpod/controller/revision"
	"github.com/openshift/library-go/pkg/operator/staticresourcecontroller"
	"github.com/openshift/library-go/pkg/operator/status"
	"github.com/openshift/library-go/pkg/operator/v1helpers"

	policyv1 "k8s.io/api/policy/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"
	"k8s.io/utils/ptr"
)

func RunOperator(ctx context.Context, cc *controllercmd.ControllerContext) error {
	kubeClient, err := kubernetes.NewForConfig(cc.ProtoKubeConfig)
	if err != nil {
		return err
	}
	configClient, err := configv1client.NewForConfig(cc.KubeConfig)
	if err != nil {
		return err
	}

	configInformers := configv1informers.NewSharedInformerFactory(configClient, 10*time.Minute)
	kubeInformersForNamespaces := v1helpers.NewKubeInformersForNamespaces(kubeClient,
		"",
		operatorclient.GlobalUserSpecifiedConfigNamespace,
		operatorclient.GlobalMachineSpecifiedConfigNamespace,
		operatorclient.OperatorNamespace,
		operatorclient.TargetNamespace,
		"kube-system",
	)
	operatorClient, dynamicInformers, err := genericoperatorclient.NewStaticPodOperatorClient(
		cc.Clock,
		cc.KubeConfig,
		operatorv1.GroupVersion.WithResource("kubeschedulers"),
		operatorv1.GroupVersion.WithKind("KubeScheduler"),
		ExtractStaticPodOperatorSpec,
		ExtractStaticPodOperatorStatus,
	)
	if err != nil {
		return err
	}

	desiredVersion := status.VersionForOperatorFromEnv()
	missingVersion := "0.0.1-snapshot"

	// By default, this will exit(0) the process if the featuregates ever change to a different set of values.
	featureGateAccessor := featuregates.NewFeatureGateAccess(
		desiredVersion, missingVersion,
		configInformers.Config().V1().ClusterVersions(), configInformers.Config().V1().FeatureGates(),
		cc.EventRecorder,
	)
	go featureGateAccessor.Run(ctx)
	go configInformers.Start(ctx.Done())

	select {
	case <-featureGateAccessor.InitialFeatureGatesObserved():
		featureGates, _ := featureGateAccessor.CurrentFeatureGates()
		klog.Infof("FeatureGates initialized: knownFeatureGates=%v", featureGates.KnownFeatures())
	case <-time.After(1 * time.Minute):
		klog.Errorf("timed out waiting for FeatureGate detection")
		return fmt.Errorf("timed out waiting for FeatureGate detection")
	}

	featureGates, err := featureGateAccessor.CurrentFeatureGates()
	if err != nil {
		return err
	}

	resourceSyncController, err := resourcesynccontroller.NewResourceSyncController(
		operatorClient,
		kubeInformersForNamespaces,
		kubeClient,
		cc.EventRecorder,
	)
	if err != nil {
		return err
	}
	configObserver := configobservercontroller.NewConfigObserver(
		operatorClient,
		kubeInformersForNamespaces,
		configInformers,
		resourceSyncController,
		cc.EventRecorder,
	)

	staticResourceController := staticresourcecontroller.NewStaticResourceController(
		"KubeControllerManagerStaticResources",
		bindata.Asset,
		[]string{
			"assets/kube-scheduler/ns.yaml",
			"assets/kube-scheduler/kubeconfig-cert-syncer.yaml",
			"assets/kube-scheduler/leader-election-rolebinding.yaml",
			"assets/kube-scheduler/scheduler-clusterrolebinding.yaml",
			"assets/kube-scheduler/policyconfigmap-role.yaml",
			"assets/kube-scheduler/policyconfigmap-rolebinding.yaml",
			"assets/kube-scheduler/svc.yaml",
			"assets/kube-scheduler/sa.yaml",
			"assets/kube-scheduler/localhost-recovery-client-crb.yaml",
			"assets/kube-scheduler/localhost-recovery-sa.yaml",
			"assets/kube-scheduler/localhost-recovery-token.yaml",
		},
		(&resourceapply.ClientHolder{}).WithKubernetes(kubeClient),
		operatorClient,
		cc.EventRecorder,
	).AddKubeInformers(kubeInformersForNamespaces)

	targetConfigController := targetconfigcontroller.NewTargetConfigController(
		os.Getenv("IMAGE"),
		os.Getenv("OPERATOR_IMAGE"),
		os.Getenv("OPERATOR_IMAGE_VERSION"),
		featureGates,
		operatorClient,
		kubeInformersForNamespaces,
		configInformers,
		operatorClient,
		kubeClient,
		cc.EventRecorder,
	)

	// don't change any versions until we sync
	versionRecorder := status.NewVersionGetter()
	clusterOperator, err := configClient.ConfigV1().ClusterOperators().Get(ctx, "kube-scheduler", metav1.GetOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		return err
	}
	for _, version := range clusterOperator.Status.Versions {
		versionRecorder.SetVersion(version.Name, version.Version)
	}
	versionRecorder.SetVersion("raw-internal", status.VersionForOperatorFromEnv())

	staticPodControllers, err := staticpod.NewBuilder(operatorClient, kubeClient, kubeInformersForNamespaces, kubeInformersForNamespaces.InformersFor(""), configInformers, cc.Clock).
		WithEvents(cc.EventRecorder).
		WithInstaller([]string{"cluster-kube-scheduler-operator", "installer"}).
		WithPruning([]string{"cluster-kube-scheduler-operator", "prune"}, "kube-scheduler-pod").
		WithRevisionedResources(operatorclient.TargetNamespace, "openshift-kube-scheduler", deploymentConfigMaps, deploymentSecrets).
		WithUnrevisionedCerts("kube-scheduler-certs", CertConfigMaps, CertSecrets).
		WithVersioning("kube-scheduler", versionRecorder).
		WithPodDisruptionBudgetGuard(
			"openshift-kube-scheduler-operator",
			"cluster-kube-scheduler-operator",
			"10259",
			"healthz",
			ptr.To(policyv1.AlwaysAllow),
			func() (bool, bool, error) {
				isSNO, precheckSucceeded, err := common.NewIsSingleNodePlatformFn(configInformers.Config().V1().Infrastructures())()
				// create only when not a single node topology
				return !isSNO, precheckSucceeded, err
			},
		).
		WithOperandPodLabelSelector(labels.Set{"app": "openshift-kube-scheduler"}.AsSelector()).
		ToControllers()
	if err != nil {
		return err
	}

	clusterOperatorStatus := status.NewClusterOperatorStatusController(
		"kube-scheduler",
		[]configv1.ObjectReference{
			{Group: "operator.openshift.io", Resource: "kubeschedulers", Name: "cluster"},
			{Group: "config.openshift.io", Resource: "schedulers"},
			{Resource: "namespaces", Name: operatorclient.GlobalUserSpecifiedConfigNamespace},
			{Resource: "namespaces", Name: operatorclient.GlobalMachineSpecifiedConfigNamespace},
			{Resource: "namespaces", Name: operatorclient.TargetNamespace},
			{Resource: "namespaces", Name: "openshift-kube-scheduler-operator"},
			{Group: "controlplane.operator.openshift.io", Resource: "podnetworkconnectivitychecks", Namespace: "openshift-kube-apiserver"},
			{Group: "rbac.authorization.k8s.io", Resource: "clusterrolebindings", Name: "system:openshift:operator:cluster-kube-scheduler-operator"},
		},
		configClient.ConfigV1(),
		configInformers.Config().V1().ClusterOperators(),
		operatorClient,
		versionRecorder,
		cc.EventRecorder,
		cc.Clock,
	)

	configmetrics.Register(configInformers)

	kubeInformersForNamespaces.Start(ctx.Done())
	configInformers.Start(ctx.Done())
	dynamicInformers.Start(ctx.Done())

	go staticPodControllers.Start(ctx)
	go staticResourceController.Run(ctx, 1)
	go resourceSyncController.Run(ctx, 1)
	go targetConfigController.Run(ctx, 1)
	go configObserver.Run(ctx, 1)
	go clusterOperatorStatus.Run(ctx, 1)

	<-ctx.Done()
	return nil
}

// deploymentConfigMaps is a list of configmaps that are directly copied for the current values.  A different actor/controller modifies these.
// the first element should be the configmap that contains the static pod manifest
var deploymentConfigMaps = []revision.RevisionResource{
	{Name: "kube-scheduler-pod"},
	{Name: "config"},
	{Name: "serviceaccount-ca"},
	{Name: "policy-configmap", Optional: true},

	{Name: "scheduler-kubeconfig"},
	{Name: "kube-scheduler-cert-syncer-kubeconfig"},
}

// deploymentSecrets is a list of secrets that are directly copied for the current values.  A different actor/controller modifies these.
var deploymentSecrets = []revision.RevisionResource{
	{Name: "serving-cert", Optional: true},
	{Name: "localhost-recovery-client-token"},
}

var CertConfigMaps = []installer.UnrevisionedResource{}

var CertSecrets = []installer.UnrevisionedResource{
	{Name: "kube-scheduler-client-cert-key"},
}

func ExtractStaticPodOperatorSpec(obj *unstructured.Unstructured, fieldManager string) (*applyoperatorv1.StaticPodOperatorSpecApplyConfiguration, error) {
	castObj := &operatorv1.KubeScheduler{}
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(obj.Object, castObj); err != nil {
		return nil, fmt.Errorf("unable to convert to KubeScheduler: %w", err)
	}
	ret, err := applyoperatorv1.ExtractKubeScheduler(castObj, fieldManager)
	if err != nil {
		return nil, fmt.Errorf("unable to extract fields for %q: %w", fieldManager, err)
	}
	if ret.Spec == nil {
		return nil, nil
	}
	return &ret.Spec.StaticPodOperatorSpecApplyConfiguration, nil
}

func ExtractStaticPodOperatorStatus(obj *unstructured.Unstructured, fieldManager string) (*applyoperatorv1.StaticPodOperatorStatusApplyConfiguration, error) {
	castObj := &operatorv1.KubeScheduler{}
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(obj.Object, castObj); err != nil {
		return nil, fmt.Errorf("unable to convert to KubeScheduler: %w", err)
	}
	ret, err := applyoperatorv1.ExtractKubeSchedulerStatus(castObj, fieldManager)
	if err != nil {
		return nil, fmt.Errorf("unable to extract fields for %q: %w", fieldManager, err)
	}

	if ret.Status == nil {
		return nil, nil
	}
	return &ret.Status.StaticPodOperatorStatusApplyConfiguration, nil
}
