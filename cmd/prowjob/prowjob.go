package prowjob

import (
	"github.com/konflux-ci/qe-tools/pkg/types"
	"github.com/spf13/cobra"
)

const (
	failIfUnhealthyParamName string = "fail-if-unhealthy"
	notifyOnPRParamName      string = "notify-on-pr"
)

var (
	artifactDir     string
	failIfUnhealthy bool
	notifyOnPR      bool
	prowJobID       string
)

// ProwjobCmd represents the prowjob command
var ProwjobCmd = &cobra.Command{
	Use:   "prowjob",
	Short: "Commands for processing Prow jobs",
}

func init() {
	ProwjobCmd.AddCommand(periodicReportCmd)
	ProwjobCmd.AddCommand(createReportCmd)
	ProwjobCmd.AddCommand(healthCheckCmd)

	createReportCmd.Flags().StringVar(&artifactDir, types.ArtifactDirParamName, "", "Path to the folder where to store produced files")
	healthCheckCmd.Flags().StringVar(&artifactDir, types.ArtifactDirParamName, "", "Path to the folder where to store produced files")
}
