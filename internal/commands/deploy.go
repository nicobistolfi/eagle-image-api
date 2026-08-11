package commands

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/cloudformation"
	cftypes "github.com/aws/aws-sdk-go-v2/service/cloudformation/types"
	"github.com/aws/aws-sdk-go-v2/service/ecr"
	ecrtypes "github.com/aws/aws-sdk-go-v2/service/ecr/types"
	"github.com/spf13/cobra"
)

const (
	// DefaultTemplateURL is where the CloudFormation template is fetched from
	// when --template is not supplied.
	DefaultTemplateURL = "https://raw.githubusercontent.com/nicobistolfi/eagle-image-api/main/template.yml"

	// DockerHubRepo is the published image repository mirrored into the
	// user's own ECR registry before deployment.
	DockerHubRepo = "nicobistolfi/eagle-image-api"

	// ECRRepoName is the repository created in the target AWS account.
	ECRRepoName = "eagle-image-api"

	// stackPollInterval is how long to wait between stack status checks.
	stackPollInterval = 10 * time.Second

	// stagePattern constrains --stage to names that are simultaneously valid
	// as a CloudFormation stack name suffix, an IAM role name segment, an API
	// Gateway stage name and a CloudFront origin path. It must stay identical
	// to the AllowedPattern of the Stage parameter in template.yml; a test
	// guards against the two drifting apart.
	stagePattern = `^[a-z][a-z0-9-]{0,19}$`
)

// stageRegexp is the compiled form of stagePattern.
var stageRegexp = regexp.MustCompile(stagePattern)

// templateURL and templateClient are variables rather than constants so tests
// can point the fetch at a local server.
var (
	templateURL    = DefaultTemplateURL
	templateClient = &http.Client{Timeout: 30 * time.Second}
)

// ecrAPI is the subset of the ECR client the deploy command uses. Depending on
// an interface rather than *ecr.Client keeps the deployment logic testable
// without AWS credentials.
type ecrAPI interface {
	DescribeRepositories(ctx context.Context, in *ecr.DescribeRepositoriesInput, opts ...func(*ecr.Options)) (*ecr.DescribeRepositoriesOutput, error)
	CreateRepository(ctx context.Context, in *ecr.CreateRepositoryInput, opts ...func(*ecr.Options)) (*ecr.CreateRepositoryOutput, error)
	GetAuthorizationToken(ctx context.Context, in *ecr.GetAuthorizationTokenInput, opts ...func(*ecr.Options)) (*ecr.GetAuthorizationTokenOutput, error)
}

// cfnAPI is the subset of the CloudFormation client the deploy command uses.
type cfnAPI interface {
	DescribeStacks(ctx context.Context, in *cloudformation.DescribeStacksInput, opts ...func(*cloudformation.Options)) (*cloudformation.DescribeStacksOutput, error)
	CreateStack(ctx context.Context, in *cloudformation.CreateStackInput, opts ...func(*cloudformation.Options)) (*cloudformation.CreateStackOutput, error)
	UpdateStack(ctx context.Context, in *cloudformation.UpdateStackInput, opts ...func(*cloudformation.Options)) (*cloudformation.UpdateStackOutput, error)
}

// dockerRunner executes a docker subcommand. The stdin argument, when
// non-empty, is piped to the process — used to feed the ECR password to
// `docker login --password-stdin` so it never appears in the process list.
type dockerRunner func(stdin string, args ...string) error

// deployFlags mirrors the command line flags of the deploy command. Every
// value is a string because it is forwarded verbatim as a CloudFormation
// parameter, which is string-typed.
type deployFlags struct {
	stage           string
	region          string
	template        string
	quality         string
	fit             string
	logLevel        string
	originWhitelist string
	redirectOnError string
	webp            string
	avif            string
	avifMaxMp       string
	environment     string
	apiEndpoint     string
	imageTag        string
}

var flags deployFlags

// deployer carries the collaborators a deployment needs. Tests construct one
// with fakes; runDeploy builds one backed by the real AWS SDK clients.
type deployer struct {
	ecr    ecrAPI
	cfn    cfnAPI
	docker dockerRunner
	out    io.Writer
	sleep  func(time.Duration)
	flags  deployFlags
}

// DeployCmd deploys the Eagle Image API to AWS via CloudFormation.
var DeployCmd = &cobra.Command{
	Use:   "deploy",
	Short: "Deploy Eagle Image API to AWS",
	Long: `Deploy the Eagle Image API to AWS using CloudFormation.

This command fetches the CloudFormation template, sets up an ECR repository,
pushes the Docker Hub image to your ECR, and deploys the stack.

AWS credentials are read from the standard AWS credential chain
(environment variables, ~/.aws/credentials, IAM role).`,
	RunE:         runDeploy,
	SilenceUsage: true,
}

func init() {
	registerDeployFlags(DeployCmd, &flags)
}

// registerDeployFlags binds every deploy flag to dst. It is split out from
// init so tests can build an independent command with its own flag storage.
func registerDeployFlags(cmd *cobra.Command, dst *deployFlags) {
	f := cmd.Flags()
	f.StringVar(&dst.stage, "stage", "dev", "Deployment stage, 1-20 characters of lowercase letters, digits and hyphens starting with a letter (e.g. dev, prod, my-test); sets the Stage parameter and stack name")
	f.StringVar(&dst.region, "region", "us-west-1", "AWS region")
	f.StringVar(&dst.template, "template", "", "Path to local CloudFormation template (default: fetched from GitHub)")
	f.StringVar(&dst.quality, "quality", "80", "Image quality (0-100)")
	f.StringVar(&dst.fit, "fit", "outside", "Default resize fit mode")
	f.StringVar(&dst.logLevel, "log-level", "info", "Log level (error/warn/info/debug)")
	f.StringVar(&dst.originWhitelist, "origin-whitelist", "*", "Comma-separated origin whitelist")
	f.StringVar(&dst.redirectOnError, "redirect-on-error", "false", "Redirect to original image on error")
	f.StringVar(&dst.webp, "webp", "true", "Enable WebP format")
	f.StringVar(&dst.avif, "avif", "true", "Enable AVIF format")
	f.StringVar(&dst.avifMaxMp, "avif-max-mp", "2", "Maximum megapixels for AVIF output")
	f.StringVar(&dst.environment, "environment", "production", "Environment name")
	f.StringVar(&dst.apiEndpoint, "api-endpoint", "/api/v1/image", "API endpoint path")
	f.StringVar(&dst.imageTag, "image-tag", "latest", "Docker Hub image tag to deploy")
}

func runDeploy(cmd *cobra.Command, _ []string) error {
	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}

	cfg, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(flags.region))
	if err != nil {
		return fmt.Errorf("loading AWS config: %w", err)
	}

	d := &deployer{
		ecr:    ecr.NewFromConfig(cfg),
		cfn:    cloudformation.NewFromConfig(cfg),
		docker: runDockerCmd,
		out:    cmd.OutOrStdout(),
		sleep:  time.Sleep,
		flags:  flags,
	}

	return d.run(ctx)
}

// run performs the full deployment: fetch template, mirror the image into
// ECR, then create or update the CloudFormation stack.
func (d *deployer) run(ctx context.Context) error {
	// Validate first: an unusable stage name would otherwise only surface as a
	// CloudFormation error, after a full image pull and ECR push.
	if err := validateStage(d.flags.stage); err != nil {
		return err
	}

	templateBody, err := getTemplateBody(d.flags.template)
	if err != nil {
		return fmt.Errorf("getting template: %w", err)
	}

	fmt.Fprintln(d.out, "Setting up ECR repository...")
	imageURI, err := d.setupECRAndPushImage(ctx)
	if err != nil {
		return fmt.Errorf("setting up ECR image: %w", err)
	}
	fmt.Fprintf(d.out, "Image pushed to: %s\n", imageURI)

	name := stackName(d.flags.stage)
	fmt.Fprintf(d.out, "Deploying stack %q in %s...\n", name, d.flags.region)

	if err := d.deployStack(ctx, name, templateBody, imageURI); err != nil {
		return fmt.Errorf("deploying stack: %w", err)
	}

	return d.printStackOutputs(ctx, name)
}

// validateStage checks that stage can be used as a deployment namespace across
// every AWS resource the template names after it.
func validateStage(stage string) error {
	if stageRegexp.MatchString(stage) {
		return nil
	}
	return fmt.Errorf("invalid stage %q: must be 1-20 characters of lowercase letters, digits and hyphens, starting with a letter (pattern %s), for example dev, prod or my-test", stage, stagePattern)
}

// stackName derives the CloudFormation stack name for a deployment stage.
func stackName(stage string) string {
	return fmt.Sprintf("eagle-image-api-%s", stage)
}

// getTemplateBody returns the CloudFormation template, read from localPath
// when set and otherwise downloaded from the project repository.
func getTemplateBody(localPath string) (string, error) {
	if localPath != "" {
		data, err := os.ReadFile(localPath)
		if err != nil {
			return "", fmt.Errorf("reading local template %q: %w", localPath, err)
		}
		return string(data), nil
	}

	req, err := http.NewRequest(http.MethodGet, templateURL, nil)
	if err != nil {
		return "", fmt.Errorf("building template request: %w", err)
	}

	resp, err := templateClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetching template: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("fetching template: HTTP %d", resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("reading template response: %w", err)
	}
	return string(data), nil
}

// setupECRAndPushImage ensures the ECR repository exists and mirrors the
// published Docker Hub image into it, returning the pushed image URI.
func (d *deployer) setupECRAndPushImage(ctx context.Context) (string, error) {
	repoURI, err := d.ensureECRRepo(ctx)
	if err != nil {
		return "", err
	}

	authToken, endpoint, err := d.getECRAuth(ctx)
	if err != nil {
		return "", fmt.Errorf("getting ECR auth: %w", err)
	}

	username, password, err := decodeAuthToken(authToken)
	if err != nil {
		return "", fmt.Errorf("docker login to ECR: %w", err)
	}
	if err := d.docker(password, "login", "--username", username, "--password-stdin", endpoint); err != nil {
		return "", fmt.Errorf("docker login to ECR: %w", err)
	}

	sourceImage := fmt.Sprintf("%s:%s", DockerHubRepo, d.flags.imageTag)
	targetImage := fmt.Sprintf("%s:%s", repoURI, d.flags.imageTag)

	fmt.Fprintf(d.out, "Pulling %s...\n", sourceImage)
	if err := d.docker("", "pull", sourceImage); err != nil {
		return "", fmt.Errorf("pulling image: %w", err)
	}

	if err := d.docker("", "tag", sourceImage, targetImage); err != nil {
		return "", fmt.Errorf("tagging image: %w", err)
	}

	fmt.Fprintf(d.out, "Pushing to ECR: %s...\n", targetImage)
	if err := d.docker("", "push", targetImage); err != nil {
		return "", fmt.Errorf("pushing image to ECR: %w", err)
	}

	return targetImage, nil
}

// ensureECRRepo returns the URI of the ECR repository, creating it when the
// describe call reports it missing.
func (d *deployer) ensureECRRepo(ctx context.Context) (string, error) {
	desc, err := d.ecr.DescribeRepositories(ctx, &ecr.DescribeRepositoriesInput{
		RepositoryNames: []string{ECRRepoName},
	})
	if err == nil && len(desc.Repositories) > 0 {
		return aws.ToString(desc.Repositories[0].RepositoryUri), nil
	}

	fmt.Fprintf(d.out, "Creating ECR repository %q...\n", ECRRepoName)
	out, err := d.ecr.CreateRepository(ctx, &ecr.CreateRepositoryInput{
		RepositoryName:     aws.String(ECRRepoName),
		ImageTagMutability: ecrtypes.ImageTagMutabilityMutable,
	})
	if err != nil {
		return "", fmt.Errorf("creating ECR repository: %w", err)
	}
	if out.Repository == nil {
		return "", fmt.Errorf("creating ECR repository: no repository returned")
	}
	return aws.ToString(out.Repository.RepositoryUri), nil
}

// getECRAuth returns the base64 authorization token and registry endpoint.
func (d *deployer) getECRAuth(ctx context.Context) (string, string, error) {
	out, err := d.ecr.GetAuthorizationToken(ctx, &ecr.GetAuthorizationTokenInput{})
	if err != nil {
		return "", "", err
	}
	if len(out.AuthorizationData) == 0 {
		return "", "", fmt.Errorf("no authorization data returned")
	}
	auth := out.AuthorizationData[0]
	return aws.ToString(auth.AuthorizationToken), aws.ToString(auth.ProxyEndpoint), nil
}

// decodeAuthToken splits an ECR authorization token into username and
// password. The token is base64 of "user:password".
func decodeAuthToken(authToken string) (username, password string, err error) {
	decoded, err := base64.StdEncoding.DecodeString(authToken)
	if err != nil {
		return "", "", fmt.Errorf("decoding auth token: %w", err)
	}
	parts := strings.SplitN(string(decoded), ":", 2)
	if len(parts) != 2 {
		return "", "", fmt.Errorf("unexpected auth token format")
	}
	return parts[0], parts[1], nil
}

// runDockerCmd shells out to the docker binary, piping stdin when non-empty.
func runDockerCmd(stdin string, args ...string) error {
	cmd := exec.Command("docker", args...)
	if stdin != "" {
		cmd.Stdin = strings.NewReader(stdin)
	}
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// buildParameters maps the deploy flags onto the CloudFormation parameters
// declared in template.yml. Order is fixed to keep the mapping reviewable.
func buildParameters(f deployFlags, imageURI string) []cftypes.Parameter {
	pairs := []struct{ key, value string }{
		{"Stage", f.stage},
		{"ImageUri", imageURI},
		{"Environment", f.environment},
		{"ApiEndpoint", f.apiEndpoint},
		{"Quality", f.quality},
		{"Fit", f.fit},
		{"LogLevel", f.logLevel},
		{"OriginWhitelist", f.originWhitelist},
		{"RedirectOnError", f.redirectOnError},
		{"WebP", f.webp},
		{"Avif", f.avif},
		{"AvifMaxMp", f.avifMaxMp},
	}

	params := make([]cftypes.Parameter, 0, len(pairs))
	for _, p := range pairs {
		params = append(params, cftypes.Parameter{
			ParameterKey:   aws.String(p.key),
			ParameterValue: aws.String(p.value),
		})
	}
	return params
}

// buildTags returns the tags applied to the CloudFormation stack.
func buildTags(f deployFlags) []cftypes.Tag {
	return []cftypes.Tag{
		{Key: aws.String("Project"), Value: aws.String("eagle-image-api")},
		{Key: aws.String("Stage"), Value: aws.String(f.stage)},
		{Key: aws.String("ManagedBy"), Value: aws.String("eagle-cli")},
	}
}

// deployStack creates the stack, or updates it when it already exists, then
// blocks until CloudFormation reaches a terminal status.
func (d *deployer) deployStack(ctx context.Context, name, templateBody, imageURI string) error {
	params := buildParameters(d.flags, imageURI)
	tags := buildTags(d.flags)
	capabilities := []cftypes.Capability{cftypes.CapabilityCapabilityNamedIam}

	_, err := d.cfn.DescribeStacks(ctx, &cloudformation.DescribeStacksInput{
		StackName: aws.String(name),
	})

	if err != nil {
		fmt.Fprintln(d.out, "Creating new stack...")
		_, err = d.cfn.CreateStack(ctx, &cloudformation.CreateStackInput{
			StackName:    aws.String(name),
			TemplateBody: aws.String(templateBody),
			Parameters:   params,
			Capabilities: capabilities,
			Tags:         tags,
		})
		if err != nil {
			return fmt.Errorf("creating stack: %w", err)
		}
		fmt.Fprintln(d.out, "Waiting for stack creation to complete...")
		return d.waitForStack(ctx, name)
	}

	fmt.Fprintln(d.out, "Updating existing stack...")
	_, err = d.cfn.UpdateStack(ctx, &cloudformation.UpdateStackInput{
		StackName:    aws.String(name),
		TemplateBody: aws.String(templateBody),
		Parameters:   params,
		Capabilities: capabilities,
		Tags:         tags,
	})
	if err != nil {
		if strings.Contains(err.Error(), "No updates are to be performed") {
			fmt.Fprintln(d.out, "Stack is already up to date.")
			return nil
		}
		return fmt.Errorf("updating stack: %w", err)
	}
	fmt.Fprintln(d.out, "Waiting for stack update to complete...")
	return d.waitForStack(ctx, name)
}

// isTerminalFailure reports whether a stack status means the deployment ended
// without success and no further polling will help.
func isTerminalFailure(status cftypes.StackStatus) bool {
	switch status {
	case cftypes.StackStatusCreateFailed,
		cftypes.StackStatusRollbackComplete,
		cftypes.StackStatusRollbackFailed,
		cftypes.StackStatusUpdateRollbackComplete,
		cftypes.StackStatusUpdateRollbackFailed,
		cftypes.StackStatusDeleteComplete,
		cftypes.StackStatusDeleteFailed:
		return true
	default:
		return false
	}
}

// isTerminalSuccess reports whether a stack status means the deployment
// finished successfully.
func isTerminalSuccess(status cftypes.StackStatus) bool {
	return status == cftypes.StackStatusCreateComplete ||
		status == cftypes.StackStatusUpdateComplete
}

// waitForStack polls the stack until it succeeds, fails, or ctx is done.
func (d *deployer) waitForStack(ctx context.Context, name string) error {
	for {
		out, err := d.cfn.DescribeStacks(ctx, &cloudformation.DescribeStacksInput{
			StackName: aws.String(name),
		})
		if err != nil {
			return fmt.Errorf("describing stack: %w", err)
		}
		if len(out.Stacks) == 0 {
			return fmt.Errorf("stack %q not found", name)
		}

		stack := out.Stacks[0]
		status := stack.StackStatus

		switch {
		case isTerminalSuccess(status):
			fmt.Fprintf(d.out, "Stack %s completed successfully.\n", status)
			return nil
		case isTerminalFailure(status):
			reason := ""
			if stack.StackStatusReason != nil {
				reason = ": " + *stack.StackStatusReason
			}
			return fmt.Errorf("stack reached terminal status %s%s", status, reason)
		}

		fmt.Fprintf(d.out, "  Status: %s...\n", status)

		if err := ctx.Err(); err != nil {
			return err
		}
		d.sleep(stackPollInterval)
	}
}

// printStackOutputs writes the stack outputs, highlighting the two URLs a
// user needs after a successful deployment.
func (d *deployer) printStackOutputs(ctx context.Context, name string) error {
	out, err := d.cfn.DescribeStacks(ctx, &cloudformation.DescribeStacksInput{
		StackName: aws.String(name),
	})
	if err != nil {
		return fmt.Errorf("describing stack outputs: %w", err)
	}
	if len(out.Stacks) == 0 {
		return fmt.Errorf("stack %q not found", name)
	}

	fmt.Fprintln(d.out, "\n=== Deployment Outputs ===")
	for _, output := range out.Stacks[0].Outputs {
		fmt.Fprintf(d.out, "  %s: %s\n", aws.ToString(output.OutputKey), aws.ToString(output.OutputValue))
	}

	for _, output := range out.Stacks[0].Outputs {
		switch aws.ToString(output.OutputKey) {
		case "ApiUrl":
			fmt.Fprintf(d.out, "\nAPI Gateway URL: %s\n", aws.ToString(output.OutputValue))
		case "CloudFrontUrl":
			fmt.Fprintf(d.out, "CloudFront URL:  %s\n", aws.ToString(output.OutputValue))
		}
	}

	return nil
}
