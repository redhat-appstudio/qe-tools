package estimate

import (
	"context"
	"fmt"
	"github.com/google/go-github/v56/github"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
	"k8s.io/klog/v2"
	"os"
	"regexp"
)

const (
	defaultBaseWeight      = 1.0
	defaultDeletionWeight  = 0.5
	defaultExtensionWeight = 2.0
)

type configFile struct {
	Base       *float64           `yaml:"base"`
	Deletion   *float64           `yaml:"deletion"`
	Extensions map[string]float64 `yaml:"extensions"`
}

var (
	config     configFile
	configPath string

	owner      string
	repository string
	prNumber   int

	human bool
)

var EstimateTimeToReviewCmd = &cobra.Command{
	Use:   "estimate-review",
	Short: "Estimate time needed to review a PR in seconds",
	RunE: func(cmd *cobra.Command, args []string) error {
		err := parseConfig(configPath, &config)
		if err != nil {
			return err
		}

		review, err := EstimateTimeToReview(owner, repository, prNumber)
		if err != nil {
			return err
		}

		if human {
			fmt.Printf("Estimated time to review %s/%s#%d is %d seconds (~%d minutes)\n", owner, repository, prNumber, review, review/60)
		} else {
			fmt.Println(review)
		}
		return nil
	},
}

func EstimateTimeToReview(owner, repository string, number int) (int, error) {
	client := github.NewClient(nil)

	files, err := getChangedFiles(client, owner, repository, number)
	if err != nil {
		return -1, err
	}

	commitCount, err := countCommits(client, owner, repository, number)
	if err != nil {
		return -1, err
	}

	commitCoefficient := 1 + 0.1*(float64(commitCount)-1)
	fileCoefficient := 1 + 0.1*(float64(len(files))-1)
	if commitCoefficient > 2 {
		commitCoefficient = 2
	}
	if fileCoefficient > 2 {
		fileCoefficient = 2
	}

	result := int(commitCoefficient * fileCoefficient * float64(estimateFileTimes(files)))
	return result, nil
}

func countCommits(client *github.Client, owner, repository string, number int) (int, error) {
	commits, _, err := client.PullRequests.ListCommits(context.Background(), owner, repository, number, nil)
	return len(commits), err
}

func getChangedFiles(client *github.Client, owner, repository string, number int) ([]*github.CommitFile, error) {
	files, _, err := client.PullRequests.ListFiles(context.Background(), owner, repository, number, nil)
	return files, err
}

func getFileExtension(filename string) string {
	regex := regexp.MustCompile(`\.[^.]*$`)
	return regex.FindString(filename)
}

func estimateFileTimes(files []*github.CommitFile) int {
	var result float64 = 0
	for _, file := range files {
		extension := getFileExtension(file.GetFilename())
		if len(extension) > 0 {
			extension = extension[1:]
		}
		estimate := config.Extensions[extension]
		if estimate == 0 {
			estimate = defaultExtensionWeight
			klog.Warningf("Weight for '%s' extension not specified. Using default weight '%.1f'.\n", extension, defaultExtensionWeight)
		}

		result += float64(file.GetAdditions()) * estimate * *config.Base
		result += float64(file.GetDeletions()) * estimate * *config.Deletion
	}
	return int(result)
}

func parseConfig(configPath string, cf *configFile) error {
	yamlFile, err := os.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("error reading file at '%s': %v", configPath, err)
	}
	err = yaml.Unmarshal(yamlFile, &cf)
	if len(cf.Extensions) == 0 {
		klog.Warningf("'extensions' list not specified")
	}
	if cf.Base == nil {
		klog.Warningf("Weight for 'base' not specified. Using default weight '%.1f'", defaultBaseWeight)
		*cf.Base = defaultBaseWeight
	}
	if cf.Deletion == nil {
		klog.Warningf("Weight for 'deletion' not specified. Using default weight '%.1f'", defaultDeletionWeight)
		*cf.Deletion = defaultDeletionWeight
	}
	if err != nil {
		return fmt.Errorf("error during unmarshaling %v", err)
	}
	return nil
}

func init() {
	EstimateTimeToReviewCmd.Flags().StringVar(&owner, "owner", "redhat-appstudio", "owner of the repository")
	EstimateTimeToReviewCmd.Flags().StringVar(&repository, "repository", "e2e-tests", "name of the repository")
	EstimateTimeToReviewCmd.Flags().IntVar(&prNumber, "number", 1, "number of the pull request")
	err := EstimateTimeToReviewCmd.MarkFlagRequired("number")
	if err != nil { // silence golangci-lint
		return
	}
	EstimateTimeToReviewCmd.Flags().StringVar(&configPath, "config", "", "path to the yaml config file")
	err = EstimateTimeToReviewCmd.MarkFlagRequired("config")
	if err != nil { // silence golangci-lint
		return
	}

	EstimateTimeToReviewCmd.Flags().BoolVar(&human, "human", false, "human readable form")
}
