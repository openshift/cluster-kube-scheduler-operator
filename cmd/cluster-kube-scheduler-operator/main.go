package main

import (
	"context"
	"os"

	"github.com/openshift/cluster-kube-scheduler-operator/cmd/render"
	operatorcmd "github.com/openshift/cluster-kube-scheduler-operator/pkg/cmd/operator"
	"github.com/openshift/cluster-kube-scheduler-operator/pkg/cmd/recoverycontroller"
	"github.com/openshift/cluster-kube-scheduler-operator/pkg/operator"
	"github.com/openshift/library-go/pkg/operator/staticpod/certsyncpod"
	"github.com/openshift/library-go/pkg/operator/staticpod/installerpod"
	"github.com/openshift/library-go/pkg/operator/staticpod/prune"
	"github.com/spf13/cobra"
	"k8s.io/component-base/cli"
)

func main() {
	command := NewSchedulerOperatorCommand(context.Background())
	code := cli.Run(command)
	os.Exit(code)
}

func NewSchedulerOperatorCommand(ctx context.Context) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "cluster-kube-scheduler-operator",
		Short: "OpenShift cluster kube-scheduler operator",
		Run: func(cmd *cobra.Command, args []string) {
			cmd.Help()
			os.Exit(1)
		},
	}

	cmd.AddCommand(operatorcmd.NewOperator())
	cmd.AddCommand(render.NewRenderCommand())
	cmd.AddCommand(installerpod.NewInstaller(ctx))
	cmd.AddCommand(prune.NewPrune())
	cmd.AddCommand(certsyncpod.NewCertSyncControllerCommand(operator.CertConfigMaps, operator.CertSecrets))
	cmd.AddCommand(recoverycontroller.NewCertRecoveryControllerCommand(ctx))

	return cmd
}
