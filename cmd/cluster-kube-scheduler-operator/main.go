package main

import (
	goflag "flag"
	"fmt"
	"math/rand"
	"os"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	utilflag "k8s.io/component-base/cli/flag"
	"k8s.io/component-base/logs"

	"github.com/openshift/cluster-kube-scheduler-operator/cmd/render"
	operatorcmd "github.com/openshift/cluster-kube-scheduler-operator/pkg/cmd/operator"
	"github.com/openshift/cluster-kube-scheduler-operator/pkg/operator"
	"github.com/openshift/library-go/pkg/operator/staticpod/certsyncpod"
	"github.com/openshift/library-go/pkg/operator/staticpod/installerpod"
	"github.com/openshift/library-go/pkg/operator/staticpod/prune"
)

func main() {
	rand.Seed(time.Now().UTC().UnixNano())

	pflag.CommandLine.SetNormalizeFunc(utilflag.WordSepNormalizeFunc)
	pflag.CommandLine.AddGoFlagSet(goflag.CommandLine)

	logs.InitLogs()
	defer logs.FlushLogs()

	command := NewSchedulerOperatorCommand()
	if err := command.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
}

func NewSchedulerOperatorCommand() *cobra.Command {
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
	cmd.AddCommand(installerpod.NewInstaller())
	cmd.AddCommand(prune.NewPrune())
	cmd.AddCommand(certsyncpod.NewCertSyncControllerCommand(operator.CertConfigMaps, operator.CertSecrets))

	return cmd
}
