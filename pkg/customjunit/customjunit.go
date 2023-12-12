package customjunit

import (
	"encoding/xml"
	. "github.com/onsi/ginkgo/v2/reporters"
)

/*
These custom structures are copied from https://github.com/onsi/ginkgo/blob/v2.13.1/reporters/junit_report.go
These custom structures are modified to use Skipped field instead of Disabled and the Status field is removed
See RHTAP-1902 for more details.
*/
type TestSuites struct {
	XMLName xml.Name `xml:"testsuites"`
	// Tests maps onto the total number of specs in all test suites (this includes any suite nodes such as BeforeSuite)
	Tests int `xml:"tests,attr"`
	// Skipped maps onto specs that are pending and/or disabled
	Skipped int `xml:"skipped,attr"`
	// Errors maps onto specs that panicked or were interrupted
	Errors int `xml:"errors,attr"`
	// Failures maps onto specs that failed
	Failures int `xml:"failures,attr"`
	// Time is the time in seconds to execute all test suites
	Time float64 `xml:"time,attr"`

	//The set of all test suites
	TestSuites []TestSuite `xml:"testsuite"`
}

type TestSuite struct {
	// Name maps onto the description of the test suite - maps onto Report.SuiteDescription
	Name string `xml:"name,attr"`
	// Package maps onto the absolute path to the test suite - maps onto Report.SuitePath
	Package string `xml:"package,attr"`
	// Tests maps onto the total number of specs in the test suite (this includes any suite nodes such as BeforeSuite)
	Tests int `xml:"tests,attr"`
	// Skiped maps onto specs that are skipped/pending
	Skipped int `xml:"skipped,attr"`
	// Errors maps onto specs that panicked or were interrupted
	Errors int `xml:"errors,attr"`
	// Failures maps onto specs that failed
	Failures int `xml:"failures,attr"`
	// Time is the time in seconds to execute all the test suite - maps onto Report.RunTime
	Time float64 `xml:"time,attr"`
	// Timestamp is the ISO 8601 formatted start-time of the suite - maps onto Report.StartTime
	Timestamp string `xml:"timestamp,attr"`

	//Properties captures the information stored in the rest of the Report type (including SuiteConfig) as key-value pairs
	Properties JUnitProperties `xml:"properties"`

	//TestCases capture the individual specs
	TestCases []TestCase `xml:"testcase"`
}

type TestCase struct {
	// Name maps onto the full text of the spec - equivalent to "[SpecReport.LeafNodeType] SpecReport.FullText()"
	Name string `xml:"name,attr"`
	// Classname maps onto the name of the test suite - equivalent to Report.SuiteDescription
	Classname string `xml:"classname,attr"`
	// Time is the time in seconds to execute the spec - maps onto SpecReport.RunTime
	Time float64 `xml:"time,attr"`
	//Skipped is populated with a message if the test was skipped or pending
	Skipped *JUnitSkipped `xml:"skipped,omitempty"`
	//Error is populated if the test panicked or was interrupted
	Error *JUnitError `xml:"error,omitempty"`
	//Failure is populated if the test failed
	Failure *JUnitFailure `xml:"failure,omitempty"`
	//SystemOut maps onto any captured stdout/stderr output - maps onto SpecReport.CapturedStdOutErr
	SystemOut string `xml:"system-out,omitempty"`
	//SystemOut maps onto any captured GinkgoWriter output - maps onto SpecReport.CapturedGinkgoWriterOutput
	SystemErr string `xml:"system-err,omitempty"`
}
