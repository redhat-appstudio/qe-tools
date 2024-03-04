package prow

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"cloud.google.com/go/storage"
	"golang.org/x/exp/slices"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
	"k8s.io/klog/v2"
	v1 "k8s.io/test-infra/prow/apis/prowjobs/v1"
	"sigs.k8s.io/yaml"
)

// NewArtifactScanner creates a new instance of ArtifactScanner,
// requires a valid ScannerConfig
func NewArtifactScanner(cfg ScannerConfig) (*ArtifactScanner, error) {
	ctx := context.Background()
	client, err := storage.NewClient(ctx, option.WithoutAuthentication())
	if err != nil {
		return nil, fmt.Errorf("failed to create new GCS client: %+v", err)
	}

	return &ArtifactScanner{
		Client: client,
		config: cfg,
	}, nil
}

// Run processes the artifacts associated with the Prow job and stores required files
// with their associated openshift-ci step names and their content in ArtifactStepMap.
func (as *ArtifactScanner) Run() error {
	// Determine job target and Prow job URL.
	jobTarget, pjURL, err := as.determineJobDetails()
	if err != nil {
		return fmt.Errorf("failed to determine job details: %+v", err)
	}

	artifactDirectoryPrefix, err := getArtifactsDirectoryPrefix(as, pjURL, jobTarget)
	if err != nil {
		return fmt.Errorf("failed to get artifact directory prefix: %+v", err)
	}

	// Iterate over storage objects.
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute*2)
	defer cancel()
	as.bucketHandle = as.Client.Bucket(bucketName)
	it := as.bucketHandle.Objects(ctx, &storage.Query{Prefix: artifactDirectoryPrefix})

	// Process storage objects.
	if err := as.processStorageObjects(ctx, it, artifactDirectoryPrefix, pjURL); err != nil {
		return fmt.Errorf("failed to process storage objects: %+v", err)
	}

	return nil
}

// Helper function to determine job details.
func (as *ArtifactScanner) determineJobDetails() (jobTarget, pjURL string, err error) {
	switch {
	case as.config.ProwJobID != "":
		pjYAML, err := getProwJobYAML(as.config.ProwJobID)
		if err != nil {
			return "", "", fmt.Errorf("failed to get Prow job YAML: %+v", err)
		}
		jobTarget, err = determineJobTargetFromYAML(pjYAML)
		if err != nil {
			return "", "", fmt.Errorf("failed to determine job target from YAML: %+v", err)
		}
		pjURL = pjYAML.Status.URL

	case as.config.ProwJobURL != "":
		pjURL = as.config.ProwJobURL
		jobTarget, err = determineJobTargetFromProwJobURL(pjURL)
		if err != nil {
			return "", "", fmt.Errorf("failed to determine job target from Prow job URL: %+v", err)
		}

	default:
		return "", "", fmt.Errorf("ScannerConfig doesn't contain either ProwJobID or ProwJobURL")
	}

	return jobTarget, pjURL, nil
}

// Helper function to process storage objects.
func (as *ArtifactScanner) processStorageObjects(ctx context.Context, it *storage.ObjectIterator, artifactDirectoryPrefix, pjURL string) error {
	var objectAttrs *storage.ObjectAttrs
	var err error

	objectAttrs, err = it.Next()
	if err == iterator.Done {
		// No files present within the target directory - get the root build-log.txt instead.
		if err := as.handleEmptyDirectory(ctx, pjURL, artifactDirectoryPrefix); err != nil {
			return err
		}
	} else {
		// Iterate over storage objects.
		for {
			if err != nil {
				return fmt.Errorf("failed to iterate over storage objects: %+v", err)
			}
			fullArtifactName := objectAttrs.Name
			if as.isRequiredFile(fullArtifactName) {
				if err := as.processRequiredFile(fullArtifactName, artifactDirectoryPrefix); err != nil {
					return err
				}
			}

			objectAttrs, err = it.Next()
			if err == iterator.Done {
				break
			}
		}
	}

	return nil
}

// Helper function to handle an empty directory.
func (as *ArtifactScanner) handleEmptyDirectory(ctx context.Context, pjURL, artifactDirectoryPrefix string) error {
	klog.Infof("For the job (%s), there are no files present within the directory with prefix: `%s`", pjURL, artifactDirectoryPrefix)

	// Set up default file name filter.
	fileName := "build-log.txt"
	as.config.FileNameFilter = []string{fileName}

	// Check for build log file.
	sp := strings.Split(pjURL, "/"+bucketName+"/")
	if len(sp) != 2 {
		return fmt.Errorf("failed to determine artifact directory's prefix - Prow job URL: '%s', bucket name: '%s'", pjURL, bucketName)
	}
	buildLogPrefix := sp[1] + "/" + fileName

	// Iterate over build log files.
	it := as.bucketHandle.Objects(ctx, &storage.Query{Prefix: buildLogPrefix})
	for {
		attrs, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return fmt.Errorf("failed to iterate over storage objects: %+v", err)
		}
		fullArtifactName := attrs.Name

		if err := as.initArtifactStepMap(ctx, fileName, fullArtifactName, "/"); err != nil {
			return err
		}
	}

	return nil
}

// Helper function to process a required file.
func (as *ArtifactScanner) processRequiredFile(fullArtifactName, artifactDirectoryPrefix string) error {
	parentStepName, err := getParentStepName(fullArtifactName, artifactDirectoryPrefix)
	if err != nil {
		return err
	}

	if slices.Contains(as.config.StepsToSkip, parentStepName) {
		klog.Infof("Skipping step name %s", parentStepName)
		return nil
	}

	fileName := getFileName(fullArtifactName, artifactDirectoryPrefix)

	if err := as.initArtifactStepMap(context.Background(), fileName, fullArtifactName, parentStepName); err != nil {
		return err
	}

	return nil
}

// Helper function to initialise/update the ArtifactStepMap with content
// of a file with given 'fileName', within the given 'parentStepName'
func (as *ArtifactScanner) initArtifactStepMap(ctx context.Context, fileName, fullArtifactName, parentStepName string) error {
	rc, err := as.bucketHandle.Object(fullArtifactName).NewReader(ctx)
	if err != nil {
		return fmt.Errorf("failed to create objecthandle for %s: %+v", fullArtifactName, err)
	}
	data, err := io.ReadAll(rc)
	if err != nil {
		return fmt.Errorf("cannot read from storage reader: %+v", err)
	}

	artifact := Artifact{Content: string(data), FullName: fullArtifactName}

	// No artifact step map not initialized yet
	if as.ArtifactStepMap == nil {
		newArtifactMap := ArtifactFilenameMap{ArtifactFilename(fileName): artifact}
		as.ArtifactStepMap = map[ArtifactStepName]ArtifactFilenameMap{ArtifactStepName(parentStepName): newArtifactMap}
	} else {
		// Already have a record of an artifact being mapped to a step name
		if afMap, ok := as.ArtifactStepMap[ArtifactStepName(parentStepName)]; ok {
			afMap[ArtifactFilename(fileName)] = artifact
			as.ArtifactStepMap[ArtifactStepName(parentStepName)] = afMap
		} else { // Artifact map initialized, but the artifact filename does not belong to any collected step
			as.ArtifactStepMap[ArtifactStepName(parentStepName)] = ArtifactFilenameMap{ArtifactFilename(fileName): artifact}
		}
	}

	return nil
}

// Helper function to check if a file with given 'fullArtifactName',
// matches the file-name filter(s) defined within ScannerConfig struct
func (as *ArtifactScanner) isRequiredFile(fullArtifactName string) bool {
	return slices.ContainsFunc(as.config.FileNameFilter, func(s string) bool {
		re := regexp.MustCompile(s)
		return re.MatchString(fullArtifactName)
	})
}

func getProwJobYAML(jobID string) (*v1.ProwJob, error) {
	r, err := http.Get(prowJobYAMLPrefix + jobID)
	errTemplate := "failed to get prow job YAML:"
	if err != nil {
		return nil, fmt.Errorf("%s %s", errTemplate, err)
	}
	if r.StatusCode > 299 {
		return nil, fmt.Errorf("%s got response status code %v", errTemplate, r.StatusCode)
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, fmt.Errorf("%s %s", errTemplate, err)
	}
	var pj v1.ProwJob
	err = yaml.Unmarshal(body, &pj)
	if err != nil {
		return nil, fmt.Errorf("%s %s", errTemplate, err)
	}
	return &pj, nil
}

func determineJobTargetFromYAML(pjYAML *v1.ProwJob) (jobTarget string, err error) {
	errPrefix := "failed to determine job target:"
	args := pjYAML.Spec.PodSpec.Containers[0].Args
	for _, arg := range args {
		if strings.Contains(arg, "--target") {
			sp := strings.Split(arg, "=")
			if len(sp) != 2 {
				return "", fmt.Errorf("%s expected %v to have len 2", errPrefix, sp)
			}
			jobTarget = sp[1]
			return
		}
	}
	return "", fmt.Errorf("%s expected %+v to contain arg --target", errPrefix, args)
}

// ParseJobSpec parses and then returns the openshift job spec data
func ParseJobSpec(jobSpecData string) (*OpenshiftJobSpec, error) {
	openshiftJobSpec := &OpenshiftJobSpec{}

	if err := json.Unmarshal([]byte(jobSpecData), openshiftJobSpec); err != nil {
		return nil, fmt.Errorf("error occurred when parsing openshift job spec data: %v", err)
	}
	return openshiftJobSpec, nil
}

func determineJobTargetFromProwJobURL(prowJobURL string) (jobTarget string, err error) {
	switch {
	case strings.Contains(prowJobURL, "pull-ci-redhat-appstudio-infra-deployments"):
		// prow URL is from infra-deployments repo
		jobTarget = "appstudio-e2e-tests"
	case strings.Contains(prowJobURL, "pull-ci-redhat-appstudio-e2e-tests"):
		// prow URL is from e2e-tests repo
		jobTarget = "redhat-appstudio-e2e"
	case strings.Contains(prowJobURL, "pull-ci-redhat-appstudio-integration-service"):
		// prow URL is from integration-service repo
		jobTarget = "integration-service-e2e"
	default:
		return "", fmt.Errorf("unable to determine the target from the ProwJobURL: %s", prowJobURL)
	}

	return jobTarget, nil
}

func getArtifactsDirectoryPrefix(artifactScanner *ArtifactScanner, prowJobURL, jobTarget string) (string, error) {
	// => e.g. [ "https://prow.ci.openshift.org/view/gs", "pr-logs/pull/redhat-appstudio_infra-deployments/123/pull-ci-redhat-appstudio-infra-deployments-main-appstudio-e2e-tests/123" ]
	sp := strings.Split(prowJobURL, "/"+bucketName+"/")
	if len(sp) != 2 {
		return "", fmt.Errorf("failed to determine artifact directory's prefix - prow job url: '%s', bucket name: '%s'", prowJobURL, bucketName)
	}

	// => e.g. "pr-logs/pull/redhat-appstudio_infra-deployments/123/pull-ci-redhat-appstudio-infra-deployments-main-appstudio-e2e-tests/123/artifacts/appstudio-e2e-tests/"
	artifactDirectoryPrefix := sp[1] + "/artifacts/" + jobTarget + "/"
	artifactScanner.ArtifactDirectoryPrefix = artifactDirectoryPrefix

	return artifactDirectoryPrefix, nil
}

func getParentStepName(fullArtifactName, artifactDirectoryPrefix string) (string, error) {
	// => e.g. [ "", "redhat-appstudio-e2e/artifacts/e2e-report.xml" ]
	sp := strings.Split(fullArtifactName, artifactDirectoryPrefix)
	if len(sp) != 2 {
		return "", fmt.Errorf("cannot determine filepath - object name: %s, object prefix: %s", fullArtifactName, artifactDirectoryPrefix)
	}
	parentStepFilePath := sp[1]

	// => e.g. [ "redhat-appstudio-e2e", "artifacts", "e2e-report.xml" ]
	sp = strings.Split(parentStepFilePath, "/")
	parentStepName := sp[0]

	return parentStepName, nil
}

func getFileName(fullArtifactName, artifactDirectoryPrefix string) string {
	sp := strings.Split(fullArtifactName, artifactDirectoryPrefix)
	parentStepFilePath := sp[1]

	sp = strings.Split(parentStepFilePath, "/")
	fileName := sp[len(sp)-1]

	return fileName
}
