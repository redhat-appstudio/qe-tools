package estimate

import (
	"context"
	"errors"
	"fmt"
	"regexp"

	"github.com/google/go-github/v56/github"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"k8s.io/klog/v2"
)

const (
	defaultBaseWeight     = 1.0
	defaultDeletionWeight = 0.5

	defaultExtensionWeight = 2.0

	defaultCommitWeight  = 0.05
	defaultCommitCeiling = 2

	defaultFileChangeWeight  = 0.1
	defaultFileChangeCeiling = 2

	defaultConfigPath = "./config/estimate"
)

// TimeLabel represents a label describing estimated time to review a PR
type TimeLabel struct {
	Name string `yaml:"name"`
	Time int    `yaml:"time"`
}

// CoefficientConfig represents coefficients used in estimation of time required for a PR review
type CoefficientConfig struct {
	Weight  float64 `yaml:"weight"`
	Ceiling float64 `yaml:"ceiling"`
}

type configFile struct {
	Base       float64            `yaml:"base"`
	Deletion   float64            `yaml:"deletion"`
	Commit     CoefficientConfig  `yaml:"commit"`
	Files      CoefficientConfig  `yaml:"files"`
	Extensions map[string]float64 `yaml:"extensions"`
	Labels     []TimeLabel        `yaml:"labels"`
}

var (
	config = configFile{
		Base:     defaultBaseWeight,
		Deletion: defaultDeletionWeight,
		Commit: CoefficientConfig{
			Weight:  defaultCommitWeight,
			Ceiling: defaultCommitCeiling,
		},
		Files: CoefficientConfig{
			Weight:  defaultFileChangeWeight,
			Ceiling: defaultFileChangeCeiling,
		},
	}
	owner      string
	repository string
	prNumber   int
	ghToken    string

	human    bool
	addLabel bool

	errEmptyLabels = errors.New("zero labels specified in config, make sure there is a non-empty 'labels' list")
)

// EstimateTimeToReviewCmd is a cobra command that estimates time needed to review a PR
var EstimateTimeToReviewCmd = &cobra.Command{
	Use:   "estimate-review",
	Short: "Estimate time needed to review a PR in seconds",
	PreRunE: func(cmd *cobra.Command, args []string) error {
		if addLabel && ghToken == "" {
			return fmt.Errorf("github token needs to be specified to add a label")
		}
		viper.AddConfigPath(defaultConfigPath)
		if err := viper.ReadInConfig(); err != nil {
			return fmt.Errorf("error reading in config: %+v", err)
		}
		if err := viper.Unmarshal(&config); err != nil {
			return fmt.Errorf("failed to parse config: %+v", err)
		}
		return nil
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		client := github.NewClient(nil)
		if ghToken != "" {
			client.WithAuthToken(ghToken)
		}

		review, err := TimeToReview(client, owner, repository, prNumber)
		if err != nil {
			return err
		}

		if human {
			fmt.Printf("Estimated time to review %s/%s#%d is %d seconds (~%d minutes)\n", owner, repository, prNumber, review, review/60)
		} else {
			fmt.Println(review)
		}
		if addLabel {
			err := addLabelToPR(client, review)
			if err != nil {
				return err
			}
		}
		return nil
	},
}

// TimeToReview estimates time needed to review a PR
func TimeToReview(client *github.Client, owner, repository string, number int) (int, error) {
	files, err := getChangedFiles(client, owner, repository, number)
	if err != nil {
		return -1, err
	}

	commitCount, err := countCommits(client, owner, repository, number)
	if err != nil {
		return -1, err
	}

	commitCoefficient := 1 + config.Commit.Weight*(float64(commitCount)-1)
	fileCoefficient := 1 + config.Files.Weight*(float64(len(files))-1)

	if commitCoefficient > config.Commit.Ceiling {
		commitCoefficient = config.Commit.Ceiling
	}

	if fileCoefficient > config.Files.Ceiling {
		fileCoefficient = config.Files.Ceiling
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
		estimate, included := config.Extensions[extension]
		if !included {
			var defaultIncluded bool
			estimate, defaultIncluded = config.Extensions["default"]
			if !defaultIncluded {
				estimate = defaultExtensionWeight
			}
			klog.Warningf("Weight for '%s' extension not specified. Using default weight '%.1f'.\n", extension, estimate)
		}

		result += float64(file.GetAdditions()) * estimate * config.Base
		result += float64(file.GetDeletions()) * estimate * config.Deletion
	}
	return int(result)
}

func addLabelToPR(client *github.Client, reviewTime int) error {
	existingLabels, err := listLabels(client)
	if err != nil {
		return err
	}
	calculatedLabel, err := getLabelBasedOnTime(reviewTime)
	if err != nil {
		return err
	}
	klog.Infof("Calculated label '%s'\n", calculatedLabel.Name)

	for _, existingLabel := range existingLabels {
		if *existingLabel.Name == calculatedLabel.Name {
			klog.Infof("The issue already has the same label '%s'. Skipping addition.\n", *existingLabel.Name)
			return nil
		}
		// Remove outdated label if the estimation changed
		for _, timeLabel := range config.Labels {
			if timeLabel.Name == *existingLabel.Name {
				_, err := client.Issues.RemoveLabelForIssue(context.Background(), owner, repository, prNumber, timeLabel.Name)
				if err != nil {
					return err
				}
				klog.Infof("Removed outdated label '%s'", timeLabel.Name)
			}
		}
	}
	_, _, err = client.Issues.AddLabelsToIssue(context.Background(), owner, repository, prNumber, []string{calculatedLabel.Name})
	if err != nil {
		return err
	}
	klog.Infof("Added label '%s'\n", calculatedLabel.Name)
	return nil
}

func listLabels(client *github.Client) ([]*github.Label, error) {
	labels, _, err := client.Issues.ListLabelsByIssue(context.Background(), owner, repository, prNumber, nil)
	if err != nil {
		return nil, err
	}
	return labels, nil
}

func getLabelBasedOnTime(reviewTime int) (*TimeLabel, error) {
	if len(config.Labels) == 0 {
		return nil, errEmptyLabels
	}
	maxLabel := TimeLabel{Time: -1}
	for _, label := range config.Labels {
		if label.Time <= reviewTime && label.Time > maxLabel.Time {
			maxLabel = label
		}
	}
	return &maxLabel, nil
}

func init() {
	EstimateTimeToReviewCmd.Flags().StringVar(&owner, "owner", "redhat-appstudio", "owner of the repository")
	EstimateTimeToReviewCmd.Flags().StringVar(&repository, "repository", "e2e-tests", "name of the repository")
	EstimateTimeToReviewCmd.Flags().IntVar(&prNumber, "number", 1, "number of the pull request")
	err := EstimateTimeToReviewCmd.MarkFlagRequired("number")
	if err != nil { // silence golangci-lint
		return
	}
	EstimateTimeToReviewCmd.Flags().StringVar(&ghToken, "token", "", "GitHub token")

	EstimateTimeToReviewCmd.Flags().BoolVar(&addLabel, "add-label", false, "add label to the GitHub PR")
	EstimateTimeToReviewCmd.Flags().BoolVar(&human, "human", false, "human readable form")
}
