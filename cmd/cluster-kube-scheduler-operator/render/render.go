package render

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/golang/glog"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/openshift/cluster-kube-scheduler-operator/pkg/operator/v311_00_assets"
	"github.com/openshift/library-go/pkg/assets"
	genericrender "github.com/openshift/library-go/pkg/operator/render"
	genericrenderoptions "github.com/openshift/library-go/pkg/operator/render/options"
)

const (
	bootstrapVersion = "v3.11.0"
)

// renderOpts holds values to drive the render command.
type renderOpts struct {
	manifest genericrenderoptions.ManifestOptions
	generic  genericrenderoptions.GenericOptions

	disablePhase2 bool
}

// NewRenderCommand creates a render command.
func NewRenderCommand() *cobra.Command {
	renderOpts := renderOpts{
		generic:  *genericrenderoptions.NewGenericOptions(),
		manifest: *genericrenderoptions.NewManifestOptions("kube-scheduler", "openshift/origin-hyperkube:latest"),
	}
	cmd := &cobra.Command{
		Use:   "render",
		Short: "Render kube-scheduler bootstrap manifests, secrets and configMaps",
		Run: func(cmd *cobra.Command, args []string) {
			if err := renderOpts.Validate(); err != nil {
				glog.Fatal(err)
			}
			if err := renderOpts.Run(); err != nil {
				glog.Fatal(err)
			}
		},
	}

	renderOpts.AddFlags(cmd.Flags())

	return cmd
}

func (r *renderOpts) AddFlags(fs *pflag.FlagSet) {
	r.manifest.AddFlags(fs, "scheduler")
	r.generic.AddFlags(fs, schema.GroupVersionKind{Group: "componentconfig", Version: "v1alpha1", Kind: "KubeSchedulerConfiguration"})

	// TODO: remove after the transition in the installer to a phase-2 free bootstrapping
	fs.BoolVar(&r.disablePhase2, "disable-phase-2", r.disablePhase2, "Disable rendering of the phase 2 daemonset and dependencies.")
	fs.MarkHidden("disable-phase-2")
	fs.MarkDeprecated("disable-phase-2", "Only used temporarily to synchronize roll out of the phase 2 removal.")
}

// Validate verifies the inputs.
func (r *renderOpts) Validate() error {
	if err := r.manifest.Validate(); err != nil {
		return err
	}
	if err := r.generic.Validate(); err != nil {
		return err
	}

	return nil
}

// Complete fills in missing values before command execution.
func (r *renderOpts) Complete() error {
	if err := r.manifest.Complete(); err != nil {
		return err
	}
	if err := r.generic.Complete(); err != nil {
		return err
	}
	return nil
}

type TemplateData struct {
	genericrenderoptions.ManifestConfig
	genericrenderoptions.FileConfig
}

// Run contains the logic of the render command.
func (r *renderOpts) Run() error {
	if err := r.Complete(); err != nil {
		return err
	}

	renderConfig := TemplateData{}
	if err := r.manifest.ApplyTo(&renderConfig.ManifestConfig); err != nil {
		return err
	}
	if err := r.generic.ApplyTo(
		&renderConfig.FileConfig,
		genericrenderoptions.Template{FileName: "defaultconfig.yaml", Content: v311_00_assets.MustAsset(filepath.Join(bootstrapVersion, "kube-scheduler", "defaultconfig.yaml"))},
		mustReadTemplateFile(filepath.Join(r.generic.TemplatesDir, "config", "bootstrap-config-overrides.yaml")),
		mustReadTemplateFile(filepath.Join(r.generic.TemplatesDir, "config", "config-overrides.yaml")),
		&renderConfig,
	); err != nil {
		return err
	}

	// add additional kubeconfig asset
	if kubeConfig, err := r.readBootstrapSecretsKubeconfig(); err != nil {
		return fmt.Errorf("failed to read %s/kubeconfig: %v", r.manifest.SecretsHostPath, err)
	} else {
		renderConfig.Assets["kubeconfig"] = kubeConfig
	}

	// TODO: remove after the transition in the installer to a phase-2 free bootstrapping
	var filters []assets.FileInfoPredicate
	if r.disablePhase2 {
		filters = append(filters, func(info os.FileInfo) bool {
			if strings.HasPrefix(info.Name(), "kube-system-") {
				return false
			}
			if info.Name() == "kube-scheduler-daemonset.yaml" {
				return false
			}
			return true
		})
	}

	return genericrender.WriteFiles(&r.generic, &renderConfig.FileConfig, renderConfig, filters...)
}

func (r *renderOpts) readBootstrapSecretsKubeconfig() ([]byte, error) {
	return ioutil.ReadFile(filepath.Join(r.generic.AssetInputDir, "..", "auth", "kubeconfig"))
}

func mustReadTemplateFile(fname string) genericrenderoptions.Template {
	bs, err := ioutil.ReadFile(fname)
	if err != nil {
		panic(fmt.Sprintf("Failed to load %q: %v", fname, err))
	}
	return genericrenderoptions.Template{FileName: fname, Content: bs}
}
