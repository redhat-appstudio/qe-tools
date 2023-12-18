package prow

import (
	"cloud.google.com/go/storage"
)

const (
	// The name of the openshift-ci step where the "createReport" command is used
	reportStepName    = "redhat-appstudio-report"
	bucketName        = "origin-ci-test"
	prowJobYAMLPrefix = "https://prow.ci.openshift.org/prowjob?prowjob="
)

// ArtifactScanner is used for initializing
// GCS client and scanning and storing
// files found in defined storage
type ArtifactScanner struct {
	bucketHandle *storage.BucketHandle
	Client       *storage.Client
	config       ScannerConfig
	/* Example:
	{
	  "gather-extra": {"build-log.txt": {Content: "<content>", FullName: "/full/gcs/path/build-log.txt"}, "finished.json": ...},
	  "e2e-tests": {"build-log.txt": ...},
	}
	*/
	ArtifactStepMap map[ArtifactStepName]ArtifactFilenameMap
	ArtifactDirectoryPrefix    string
}

// ScannerConfig contains fields required
// for scaning files with ArtifactScanner
type ScannerConfig struct {
	ProwJobID      string
	ProwJobURL     string
	FileNameFilter []string
}

// ArtifactStepName represents the openshift-ci step name
type ArtifactStepName string

// ArtifactFilenameMap - e.g. "build-log.txt": {Content: "<file-content>", FullName: "/full/gcs/path/build-log.txt"}
type ArtifactFilenameMap map[ArtifactFilename]Artifact

// ArtifactFilename represents the name of the file (including file extension)
type ArtifactFilename string

// Artifact stores the full name of the artifact (in GCS) and the content of the file
type Artifact struct {
	Content  string
	FullName string
}
