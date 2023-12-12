package prowjob

import (
	"bufio"
	"encoding/xml"
	"fmt"
	"github.com/redhat-appstudio/qe-tools/pkg/types"
	"os"
	"path/filepath"
	"strings"

	"github.com/GoogleCloudPlatform/testgrid/metadata"
	"github.com/redhat-appstudio/qe-tools/pkg/prow"

	"k8s.io/klog/v2"
	"sigs.k8s.io/yaml"

	reporters "github.com/onsi/ginkgo/v2/reporters"
	ginkgoTypes "github.com/onsi/ginkgo/v2/types"

	"github.com/redhat-appstudio-qe/junit2html/pkg/convert"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

const (
	buildLogFilename = "build-log.txt"
	finishedFilename = "finished.json"

	gcsBrowserURLPrefix = "https://gcsweb-ci.apps.ci.l2s4.p1.openshiftapps.com/gcs/origin-ci-test/"
)

// createReportCmd represents the createReport command
var createReportCmd = &cobra.Command{
	Use:   "create-report",
	Short: "Analyze specified prow job and create a report in junit/html format",
	PreRunE: func(cmd *cobra.Command, _ []string) error {
		if viper.GetString(types.ProwJobIDParamName) == "" {
			_ = cmd.Usage()
			return fmt.Errorf("parameter %q not provided, neither %s env var was set", types.ProwJobIDParamName, types.ProwJobIDEnv)
		}
		return nil
	},
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		jobID := viper.GetString(types.ProwJobIDParamName)

		cfg := prow.ScannerConfig{
			ProwJobID:      jobID,
			FileNameFilter: []string{finishedFilename, buildLogFilename, types.JunitFilename},
		}

		scanner, err := prow.NewArtifactScanner(cfg)
		if err != nil {
			return fmt.Errorf("failed to initialize artifact scanner: %+v", err)
		}

		if err := scanner.Run(); err != nil {
			return fmt.Errorf("failed to scan artifacts for prow job %s: %+v", jobID, err)
		}

		overallJUnitSuites := &reporters.JUnitTestSuites{}
		openshiftCiJunit := reporters.JUnitTestSuite{Name: "openshift-ci job", Properties: reporters.JUnitProperties{Properties: []reporters.JUnitProperty{}}}

		htmlReportLink := gcsBrowserURLPrefix + scanner.ObjectPrefix + "redhat-appstudio-report/artifacts/junit-summary.html"
		openshiftCiJunit.Properties.Properties = append(openshiftCiJunit.Properties.Properties, reporters.JUnitProperty{Name: "html-report-link", Value: htmlReportLink})

		for stepName, artifactsFilenameMap := range scanner.ArtifactStepMap {
			for artifactFilename, artifact := range artifactsFilenameMap {
				if artifactFilename == finishedFilename {
					if strings.Contains(string(stepName), "gather") {
						openshiftCiJunit.Properties.Properties = append(openshiftCiJunit.Properties.Properties, reporters.JUnitProperty{Name: string(stepName), Value: gcsBrowserURLPrefix + strings.TrimSuffix(artifact.FullName, finishedFilename) + "artifacts"})
					}

					finished := metadata.Finished{}
					err = yaml.Unmarshal([]byte(artifact.Content), &finished)
					if err != nil {
						return fmt.Errorf("cannot unmarshal %s into finished struct: %+v", artifact.Content, err)
					}

					var buildLog string
					if val, ok := artifactsFilenameMap[buildLogFilename]; ok {
						buildLog = val.Content
					}

					if *finished.Passed {
						openshiftCiJunit.TestCases = append(openshiftCiJunit.TestCases, reporters.JUnitTestCase{Name: string(stepName), Status: ginkgoTypes.SpecStatePassed.String(), SystemErr: buildLog})
					} else {
						failure := &reporters.JUnitFailure{Message: fmt.Sprintf("%s has failed", stepName)}
						tc := reporters.JUnitTestCase{Name: string(stepName), Status: ginkgoTypes.SpecStateFailed.String(), Failure: failure, SystemErr: buildLog}
						openshiftCiJunit.Failures++
						openshiftCiJunit.TestCases = append(openshiftCiJunit.TestCases, tc)
					}
					openshiftCiJunit.Tests++
				} else if strings.Contains(string(artifactFilename), ".xml") {
					if err = xml.Unmarshal([]byte(artifact.Content), overallJUnitSuites); err != nil {
						klog.Errorf("cannot decode JUnit suite %q into xml: %+v", artifactFilename, err)
					}
				}
			}
		}

		artifactDir := viper.GetString(types.ArtifactDirParamName)
		if artifactDir == "" {
			artifactDir = "./tmp/" + jobID
			klog.Warningf("path to artifact dir was not provided - using default %q\n", artifactDir)
		}

		if err := os.MkdirAll(artifactDir, 0o750); err != nil {
			return fmt.Errorf("failed to create directory for results '%s': %+v", artifactDir, err)
		}

		overallJUnitSuites.TestSuites = append(overallJUnitSuites.TestSuites, openshiftCiJunit)
		overallJUnitSuites.Failures += openshiftCiJunit.Failures
		overallJUnitSuites.Errors += openshiftCiJunit.Errors
		overallJUnitSuites.Tests += openshiftCiJunit.Tests

		generatedJunitFilepath := filepath.Clean(artifactDir + "/junit.xml")
		outFile, err := os.Create(generatedJunitFilepath)
		if err != nil {
			return fmt.Errorf("cannot create file '%s': %+v", generatedJunitFilepath, err)
		}

		if err := xml.NewEncoder(bufio.NewWriter(outFile)).Encode(overallJUnitSuites); err != nil {
			return fmt.Errorf("cannot encode JUnit suites struct '%+v' into file located at '%s': %+v", overallJUnitSuites, generatedJunitFilepath, err)
		}

		html, err := convert.Convert(overallJUnitSuites)
		if err != nil {
			return fmt.Errorf("failed to convert junit suite to html: %+v", err)
		}
		if err := os.WriteFile(artifactDir+"/junit-summary.html", []byte(html), 0o600); err != nil {
			return fmt.Errorf("failed to create HTML file with test summary: %+v", err)
		}

		klog.Infof("JUnit report saved to: %s/junit.xml", artifactDir)
		klog.Infof("HTML report saved to: %s/junit-summary.html", artifactDir)
		return nil
	},
}

func init() {
	createReportCmd.Flags().StringVar(&prowJobID, types.ProwJobIDParamName, "", "Prow job ID to analyze")

	_ = viper.BindPFlag(types.ArtifactDirParamName, createReportCmd.Flags().Lookup(types.ArtifactDirParamName))
	_ = viper.BindPFlag(types.ProwJobIDParamName, createReportCmd.Flags().Lookup(types.ProwJobIDParamName))
	// Bind environment variables to viper (in case the associated command's parameter is not provided)
	_ = viper.BindEnv(types.ProwJobIDParamName, types.ProwJobIDEnv)
	_ = viper.BindEnv(types.ArtifactDirParamName, types.ArtifactDirEnv)
}
