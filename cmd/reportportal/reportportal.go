package reportportal

import (
	"bufio"
	"encoding/xml"
	"fmt"
	"github.com/onsi/ginkgo/v2/reporters"
	"github.com/redhat-appstudio/qe-tools/pkg/customjunit"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"io"
	"k8s.io/klog/v2"
	"os"
	"path/filepath"
	"regexp"
)

var (
	reportPath string
	outPath    string
)

const (
	ReportPathParamName = "report-path"
	ReportPathEnv       = "REPORT_PATH"

	OutPathParamName = "out-path"
	OutEnv           = "OUT_PATH"
)

// PrepareRPCmd Removes `disabled` and `Status` fields for Report Portal
var PrepareRPCmd = &cobra.Command{
	Use:   "prepare-rp",
	Short: "Prepare junit file for upload to Report Portal",
	PreRunE: func(cmd *cobra.Command, args []string) error {
		xmlRegex := regexp.MustCompile(`.+\.xml$`)

		viperReportPath := viper.GetString(ReportPathParamName)
		viperOutPath := viper.GetString(OutPathParamName)
		if viperReportPath == "" {
			return fmt.Errorf("neither parameter '%s' nor env var `%s` were provided", ReportPathParamName, ReportPathEnv)
		}
		if !xmlRegex.MatchString(viperReportPath) {
			return fmt.Errorf("report path does not seem to be a xml file")
		}
		if viperOutPath != "" && !xmlRegex.MatchString(viperOutPath) {
			return fmt.Errorf("out path does not seem to be a xml file")
		}
		return nil
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		overallJUnitSuites := &reporters.JUnitTestSuites{}
		customJUnitSuites := &customjunit.TestSuites{}

		if reportFile := viper.GetString(ReportPathParamName); reportFile != "" {
			if err := readXMLFile(reportFile, overallJUnitSuites); err != nil {
				return err
			}

			if err := readXMLFile(reportFile, customJUnitSuites); err != nil {
				return err
			}
		} else {
			return fmt.Errorf("no valid parameter provided")
		}

		changeDisabledToSkipped(overallJUnitSuites, customJUnitSuites)

		receivedOutPath := viper.GetString(OutPathParamName)
		generatedRPJunitFilepath := filepath.Clean(receivedOutPath)
		outFile, err := os.Create(generatedRPJunitFilepath)
		if err != nil {
			return fmt.Errorf("cannot create file '%s': %+v", generatedRPJunitFilepath, err)
		}

		if err := xml.NewEncoder(bufio.NewWriter(outFile)).Encode(customJUnitSuites); err != nil {
			return fmt.Errorf("cannot encode JUnit suites struct '%+v' into file located at '%s': %+v", customJUnitSuites, generatedRPJunitFilepath, err)
		}

		return nil
	},
}

func changeDisabledToSkipped(original *reporters.JUnitTestSuites, custom *customjunit.TestSuites) {
	totalSkipped := 0
	for _, suite := range original.TestSuites {
		if suite.Disabled != 0 {
			for i := range custom.TestSuites {
				if custom.TestSuites[i].Name == suite.Name {
					custom.TestSuites[i].Skipped += suite.Disabled
				}
				totalSkipped += custom.TestSuites[i].Skipped
			}
		}
	}
	custom.Skipped = totalSkipped
}

func readXMLFile(xmlPath string, result any) error {
	xmlFile, err := os.Open(xmlPath)
	if err != nil {
		return fmt.Errorf("Could not open file '%s', error: %v\n", xmlPath, err)
	}
	defer xmlFile.Close()

	xmlBytes, err := io.ReadAll(xmlFile)
	if err != nil {
		return err
	}

	if err = xml.Unmarshal(xmlBytes, &result); err != nil {
		klog.Errorf("cannot decode JUnit suite %q into xml: %+v", xmlPath, err)
	}

	return nil
}

func init() {
	PrepareRPCmd.Flags().StringVar(&reportPath, ReportPathParamName, "", "Path to the XML file to be prepared for Report Portal")
	PrepareRPCmd.Flags().StringVar(&outPath, OutPathParamName, "junit-rp.xml", "Path where to generate report for Report Portal")

	_ = viper.BindPFlag(ReportPathParamName, PrepareRPCmd.Flags().Lookup(ReportPathParamName))
	_ = viper.BindPFlag(OutPathParamName, PrepareRPCmd.Flags().Lookup(OutPathParamName))
	// Bind environment variables to viper (in case the associated command's parameter is not provided)
	_ = viper.BindEnv(ReportPathParamName, ReportPathEnv)
	_ = viper.BindEnv(OutPathParamName, OutEnv)
}
