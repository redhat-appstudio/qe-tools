package prowjob

import (
	"fmt"
	"github.com/redhat-appstudio/qe-tools/pkg/prow"
	"github.com/redhat-appstudio/qe-tools/pkg/webhook"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"k8s.io/klog/v2"
	"os"
	"strings"
)

// AppStudio QE webhook configuration values will be used by default (if none are provided via env vars)
const (
	appstudioQESaltSecret       = "123456789"
	appstudioQEWebhookTargetURL = "https://hook.pipelinesascode.com/EyFYTakxEgEy"

	jobTypeParamName          = "job-type"
	jobNameParamName          = "job-name"
	repoOwnerParamName        = "repo-owner"
	repoNameParamName         = "repo-name"
	prNumberParamName         = "pr-number"
	saltSecretParamName       = "salt-secret"
	webhookTargetUrlParamName = "target-url"
	jobSpecParamName          = "job-spec"

	jobTypeEnv          = "JOB_TYPE"
	jobNameEnv          = "JOB_NAME"
	repoOwnerEnv        = "REPO_OWNER"
	repoNameEnv         = "REPO_NAME"
	prNumberEnv         = "PULL_NUMBER"
	saltSecretEnv       = "WEBHOOK_SALT_SECRET"
	webhookTargetUrlEnv = "WEBHOOK_TARGET_URL"
	jobSpecEnv          = "JOB_SPEC"
)

var (
	openshiftJobSpec *prow.OpenshiftJobSpec
	jobType          string
	jobName          string
	repoOwner        string
	repoName         string
	prNumber         string
	saltSecret       string
	webhookTargetUrl string
	jobSpec          string
)

var reportPortalWebhookCmd = &cobra.Command{
	Use: "rp-webhook",
	PreRunE: func(cmd *cobra.Command, args []string) error {
		if jobType == "" {
			return fmt.Errorf("parameter '%s' and env var '%s' is empty", jobTypeParamName, jobTypeEnv)

		}
		if jobName == "" {
			return fmt.Errorf("parameter '%s' and env var '%s' is empty", jobNameParamName, jobNameEnv)
		}
		if repoOwner == "" {
			return fmt.Errorf("parameter '%s' and env var '%s' is empty", repoOwnerParamName, repoOwnerEnv)
		}
		if repoName == "" {
			return fmt.Errorf("parameter '%s' and env var '%s' is empty", repoNameParamName, repoNameEnv)
		}
		if prNumber == "" {
			return fmt.Errorf("parameter '%s' and env var '%s' is empty", prNumberParamName, prNumberEnv)
		}
		if jobSpec == "" {
			return fmt.Errorf("parameter '%s' and env var '%s' is empty", jobSpecParamName, jobSpecEnv)
		}

		var err error
		openshiftJobSpec, err = prow.ParseJobSpec(jobSpec)
		if err != nil {
			return fmt.Errorf("error parsing openshift job spec: %+v", err)
		}
		return nil
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		var repoURL string

		if strings.Contains(jobName, "hacbs-e2e-periodic") {
			// TODO configure webhook channel for sending HACBS test results
			klog.Infof("not sending webhook for HACBS periodic job yet")
			return nil
		}

		if jobType == "periodic" {
			repoURL = "https://github.com/redhat-appstudio/infra-deployments"
			repoOwner = "redhat-appstudio"
			repoName = "infra-deployments"
			prNumber = "periodic"
		} else if repoName == "e2e-tests" || repoName == "infra-deployments" {
			repoURL = openshiftJobSpec.Refs.RepoLink
		} else {
			klog.Infof("sending webhook for jobType %s, jobName %s is not supported", jobType, jobName)
			return nil
		}

		path, err := os.Executable()
		if err != nil {
			return fmt.Errorf("error when sending webhook: %+v", err)
		}

		wh := webhook.Webhook{
			Path: path,
			Repository: webhook.Repository{
				FullName:   fmt.Sprintf("%s/%s", repoOwner, repoName),
				PullNumber: prNumber,
			},
			RepositoryURL: repoURL,
		}
		resp, err := wh.CreateAndSend(saltSecret, webhookTargetUrl)
		if err != nil {
			return fmt.Errorf("error sending webhook: %+v", err)
		}
		klog.Infof("webhook response: %+v", resp)

		return nil
	},
}

func init() {
	reportPortalWebhookCmd.Flags().StringVar(&jobType, jobTypeParamName, "", "Type of the job")
	reportPortalWebhookCmd.Flags().StringVar(&jobName, jobNameParamName, "", "Name of the job")
	reportPortalWebhookCmd.Flags().StringVar(&repoOwner, repoOwnerParamName, "", "Owner of the repository")
	reportPortalWebhookCmd.Flags().StringVar(&repoName, repoNameParamName, "", "Name of the repository")
	reportPortalWebhookCmd.Flags().StringVar(&prNumber, prNumberParamName, "", "Number of the pull request")
	reportPortalWebhookCmd.Flags().StringVar(&saltSecret, saltSecretParamName, appstudioQESaltSecret, "Salt for webhook config")
	reportPortalWebhookCmd.Flags().StringVar(&webhookTargetUrl, webhookTargetUrlParamName, appstudioQEWebhookTargetURL, "Target URL for webhook")
	reportPortalWebhookCmd.Flags().StringVar(&jobSpec, jobSpecParamName, "", "Job spec")

	_ = viper.BindEnv(jobTypeParamName, jobTypeEnv)
	_ = viper.BindEnv(jobNameParamName, jobNameEnv)
	_ = viper.BindEnv(repoOwnerParamName, repoOwnerEnv)
	_ = viper.BindEnv(repoNameParamName, repoNameEnv)
	_ = viper.BindEnv(prNumberParamName, prNumberEnv)
	_ = viper.BindEnv(saltSecretParamName, saltSecretEnv)
	_ = viper.BindEnv(webhookTargetUrlParamName, webhookTargetUrlEnv)
	_ = viper.BindEnv(jobSpecParamName, jobSpecEnv)

	jobType = viper.GetString(jobTypeParamName)
	jobName = viper.GetString(jobNameParamName)
	repoOwner = viper.GetString(repoOwnerParamName)
	repoName = viper.GetString(repoNameParamName)
	prNumber = viper.GetString(prNumberParamName)
	saltSecret = viper.GetString(saltSecretParamName)
	webhookTargetUrl = viper.GetString(webhookTargetUrlParamName)
	jobSpec = viper.GetString(jobSpecParamName)
}
