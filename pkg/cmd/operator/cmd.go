package operator

import (
	"github.com/spf13/cobra"

	"github.com/openshift/cluster-kube-scheduler-operator/pkg/operator"
	"github.com/openshift/cluster-kube-scheduler-operator/pkg/version"
	"github.com/openshift/library-go/pkg/controller/controllercmd"

	"k8s.io/utils/clock"
)

func NewOperator() *cobra.Command {
	cmd := controllercmd.
		NewControllerCommandConfig("openshift-cluster-kube-scheduler-operator", version.Get(), operator.RunOperator, clock.RealClock{}).
		NewCommand()
	cmd.Use = "operator"
	cmd.Short = "Start the Cluster kube-scheduler Operator"

	return cmd
}
