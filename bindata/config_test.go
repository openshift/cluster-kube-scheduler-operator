package bindata

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

// TestConfigSync checks if the bootkube scheduler's bootstrap config is same as the newly created controlplane's
// scheduler's config. This is needed in order to make sure that there are no two active schedulers running at the same
// time.
// An example where the diff caused problems: https://bugzilla.redhat.com/show_bug.cgi?id=2062459
func TestConfigSync(t *testing.T) {
	ignorableFields := []string{"kubeconfig", "clientConnection"}
	bootstrapSchedulerConfig, err := getFileContent("bootkube/config/bootstrap-config-overrides.yaml")
	if err != nil {
		t.Errorf("bootstrap file opening failed with %v", err)
	}

	controlPlaneSchedulerConfig, err := getFileContent("assets/config/defaultconfig.yaml", ignorableFields...)
	if err != nil {
		t.Errorf("controlPlaneSchedulerConfig file opening failed with %v", err)
	}
	if diff := cmp.Diff(bootstrapSchedulerConfig, controlPlaneSchedulerConfig); len(diff) != 0 {
		t.Fatalf("expected both bootstrap scheduler config and controlplane scheduler config to be equal but "+
			"they are not at %v", diff)
	}
}

// lineInIgonrableFields checks if the given line can be ignored based on the ignorableFields passed
func lineInIgnorableFields(line string, ignorableFields ...string) bool {
	for _, ignorableField := range ignorableFields {
		if strings.Contains(line, ignorableField) {
			return true
		}
	}
	return false
}

// getFileContent gets the content of the file that was passed. It optionally takes ignorableFields arg which can
// ignore certain lines that are not needed.
func getFileContent(filePath string, ignorableFields ...string) ([]string, error) {
	var lines []string
	file, err := os.Open(filePath)
	if err != nil {
		return lines, fmt.Errorf("failed to open file %v", filePath)
	}
	defer func(file *os.File) {
		if err := file.Close(); err != nil {
			log.Printf("failed closing file %v", file.Name())
		}
	}(file)
	scanner := bufio.NewScanner(file)
	scanner.Split(bufio.ScanLines)
	for scanner.Scan() {
		line := scanner.Text()
		if !lineInIgnorableFields(line, ignorableFields...) {
			lines = append(lines, line)
		}
	}
	return lines, nil
}
