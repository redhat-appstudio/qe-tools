package webhook

import (
	"fmt"
	"os"
	"strconv"

	"github.com/konflux-ci/qe-tools/pkg/prow"
	"github.com/konflux-ci/qe-tools/pkg/types"
	"github.com/konflux-ci/qe-tools/pkg/webhook"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"k8s.io/klog/v2"
)

var (
	openshiftJobSpec *prow.OpenshiftJobSpec
	parameters       = []*types.CmdParameter[string]{jobSpec, saltSecret, webhookTargetURL}
	jobSpec          = &types.CmdParameter[string]{
		Name:  "job-spec",
		Env:   "JOB_SPEC",
		Usage: "Job spec",
	}
	saltSecret = &types.CmdParameter[string]{
		Name:         "salt-secret",
		Env:          "SALT_SECRET",
		DefaultValue: "123456789",
		Usage:        "Salt for webhook config",
	}
	webhookTargetURL = &types.CmdParameter[string]{
		Name:         "target-url",
		Env:          "TARGET_URL",
		DefaultValue: "https://hook.pipelinesascode.com/EyFYTakxEgEy",
		Usage:        "Target URL for webhook",
	}
)

var reportPortalWebhookCmd = &cobra.Command{
	Use: "report-portal",
	PreRunE: func(cmd *cobra.Command, args []string) error {
		var err error
		openshiftJobSpec, err = prow.ParseJobSpec(jobSpec.Value)
		if err != nil {
			return fmt.Errorf("error parsing openshift job spec: %+v", err)
		}
		return nil
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		pullNumber := ""
		if openshiftJobSpec.Type == "periodic" {
			openshiftJobSpec.Refs.RepoLink = "https://github.com/redhat-appstudio/infra-deployments"
			pullNumber = "periodic"
		} else if (openshiftJobSpec.Refs.Repo == "e2e-tests" || openshiftJobSpec.Refs.Repo == "infra-deployments") && len(openshiftJobSpec.Refs.Pulls) > 0 {
			pullNumber = strconv.Itoa(openshiftJobSpec.Refs.Pulls[0].Number)
		} else {
			klog.Infof("sending webhook for jobType %s, jobName %s is not supported", openshiftJobSpec.Type, openshiftJobSpec.Job)
			return nil
		}

		path, err := os.Executable()
		if err != nil {
			return fmt.Errorf("error when sending webhook: %+v", err)
		}

		wh := webhook.Webhook{
			Path: path,
			Repository: webhook.Repository{
				FullName:   fmt.Sprintf("%s/%s", openshiftJobSpec.Refs.Organization, openshiftJobSpec.Refs.Repo),
				PullNumber: pullNumber,
			},
			RepositoryURL: openshiftJobSpec.Refs.RepoLink,
		}

		resp, err := wh.CreateAndSend(saltSecret.Value, webhookTargetURL.Value)
		if err != nil {
			return fmt.Errorf("error sending webhook: %+v", err)
		}
		klog.Infof("webhook response: %+v", resp)

		return nil
	},
}

func init() {
	for _, parameter := range parameters {
		reportPortalWebhookCmd.Flags().StringVar(&parameter.Value, parameter.Name, parameter.DefaultValue, parameter.Usage)
		_ = viper.BindEnv(parameter.Name, parameter.Env)
		viper.SetDefault(parameter.Name, parameter.DefaultValue)
		parameter.Value = viper.GetString(parameter.Name)
	}
}
