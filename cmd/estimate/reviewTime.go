package estimate

import (
	"context"
	"fmt"
	"github.com/google/go-github/v56/github"
	"github.com/spf13/cobra"
	"regexp"
)

const (
	base     = 1
	deletion = base / 2.0
)

var estimates = map[string]float64{
	"go":  1,
	"sum": 0.1,
	"mod": 0.5,

	"sh": 1.5,

	"yml":  2,
	"yaml": 2,
	"json": 2,

	"md": 0.2,
}

var (
	owner      string
	repository string
	prNumber   int

	human bool
)

var EstimateTimeToReviewCmd = &cobra.Command{
	Use:   "estimate-review",
	Short: "Estimate time needed to review a PR in seconds",
	RunE: func(cmd *cobra.Command, args []string) error {
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
		estimate := estimates[extension]
		if estimate == 0 {
			estimate = 2
		}

		result += float64(file.GetAdditions()) * estimate * base
		result += float64(file.GetDeletions()) * estimate * deletion
	}
	return int(result)
}

func init() {
	EstimateTimeToReviewCmd.Flags().StringVar(&owner, "owner", "redhat-appstudio", "owner of the repository")
	EstimateTimeToReviewCmd.Flags().StringVar(&repository, "repository", "e2e-tests", "name of the repository")
	EstimateTimeToReviewCmd.Flags().IntVar(&prNumber, "number", 1, "number of the pull request")
	err := EstimateTimeToReviewCmd.MarkFlagRequired("number")
	if err != nil { // silence golangci-lint
		return
	}

	EstimateTimeToReviewCmd.Flags().BoolVar(&human, "human", false, "human readable form")
}
