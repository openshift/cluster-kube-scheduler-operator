package recoverycontroller

import (
	"context"
	"fmt"

	operatorv1 "github.com/openshift/api/operator/v1"
	"github.com/openshift/cluster-kube-scheduler-operator/pkg/operator"
	"github.com/openshift/cluster-kube-scheduler-operator/pkg/operator/operatorclient"
	operatorresourcesynccontroller "github.com/openshift/cluster-kube-scheduler-operator/pkg/operator/resourcesynccontroller"
	"github.com/openshift/library-go/pkg/controller/controllercmd"
	"github.com/openshift/library-go/pkg/operator/genericoperatorclient"
	"github.com/openshift/library-go/pkg/operator/resourcesynccontroller"
	"github.com/openshift/library-go/pkg/operator/v1helpers"
	"github.com/spf13/cobra"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/pkg/version"
	"k8s.io/utils/clock"
)

type Options struct {
	controllerContext *controllercmd.ControllerContext
}

func NewCertRecoveryControllerCommand(ctx context.Context) *cobra.Command {
	o := &Options{}
	c := clock.RealClock{}

	ccc := controllercmd.NewControllerCommandConfig("cert-recovery-controller", version.Get(), func(ctx context.Context, controllerContext *controllercmd.ControllerContext) error {
		o.controllerContext = controllerContext

		err := o.Validate(ctx)
		if err != nil {
			return err
		}

		err = o.Complete(ctx)
		if err != nil {
			return err
		}

		err = o.Run(ctx, c)
		if err != nil {
			return err
		}

		return nil
	}, c)

	// Disable serving for recovery as it introduces a dependency on kube-system::extension-apiserver-authentication
	// configmap which prevents it to start as the CA bundle is expired.
	// TODO: Remove when the internal logic can start serving without extension-apiserver-authentication
	//  	 and live reload extension-apiserver-authentication after it is available
	ccc.DisableServing = true

	cmd := ccc.NewCommandWithContext(ctx)
	cmd.Use = "cert-recovery-controller"
	cmd.Short = "Start the Cluster Certificate Recovery Controller"

	return cmd
}

func (o *Options) Validate(ctx context.Context) error {
	return nil
}

func (o *Options) Complete(ctx context.Context) error {
	return nil
}

func (o *Options) Run(ctx context.Context, clock clock.Clock) error {
	kubeClient, err := kubernetes.NewForConfig(o.controllerContext.ProtoKubeConfig)
	if err != nil {
		return fmt.Errorf("can't build kubernetes client: %w", err)
	}

	kubeInformersForNamespaces := v1helpers.NewKubeInformersForNamespaces(
		kubeClient,
		operatorclient.GlobalMachineSpecifiedConfigNamespace,
		operatorclient.TargetNamespace,
	)

	operatorClient, dynamicInformers, err := genericoperatorclient.NewStaticPodOperatorClient(
		clock,
		o.controllerContext.KubeConfig,
		operatorv1.GroupVersion.WithResource("kubeschedulers"),
		operatorv1.GroupVersion.WithKind("KubeScheduler"),
		operator.ExtractStaticPodOperatorSpec,
		operator.ExtractStaticPodOperatorStatus,
	)
	if err != nil {
		return err
	}

	resourceSyncController := resourcesynccontroller.NewResourceSyncController(
		"kube-scheduler",
		operatorClient,
		kubeInformersForNamespaces,
		v1helpers.CachedSecretGetter(kubeClient.CoreV1(), kubeInformersForNamespaces),
		v1helpers.CachedConfigMapGetter(kubeClient.CoreV1(), kubeInformersForNamespaces),
		o.controllerContext.EventRecorder,
	)
	err = operatorresourcesynccontroller.AddSyncClientCertKeySecret(resourceSyncController)
	if err != nil {
		return err
	}

	kubeInformersForNamespaces.Start(ctx.Done())
	dynamicInformers.Start(ctx.Done())

	// FIXME: These are missing a wait group to track goroutines and handle graceful termination
	// (@deads2k wants time to think it through)
	go func() {
		resourceSyncController.Run(ctx, 1)
	}()

	<-ctx.Done()

	return nil
}
