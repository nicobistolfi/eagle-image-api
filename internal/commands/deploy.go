package commands

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
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
	defaultTemplateURL = "https://raw.githubusercontent.com/nicobistolfi/eagle-image-api/main/template.yml"
	dockerHubImage     = "nicobistolfi/eagle-image-api:latest"
	ecrRepoName        = "eagle-image-api"
)

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

var DeployCmd = &cobra.Command{
	Use:   "deploy",
	Short: "Deploy Eagle Image API to AWS",
	Long: `Deploy the Eagle Image API to AWS using CloudFormation.

This command fetches the CloudFormation template, sets up an ECR repository,
pushes the Docker Hub image to your ECR, and deploys the stack.

AWS credentials are read from the standard AWS credential chain
(environment variables, ~/.aws/credentials, IAM role).`,
	RunE: runDeploy,
}

func init() {
	f := DeployCmd.Flags()
	f.StringVar(&flags.stage, "stage", "dev", "Deployment stage (sets Stage parameter and stack name)")
	f.StringVar(&flags.region, "region", "us-west-1", "AWS region")
	f.StringVar(&flags.template, "template", "", "Path to local CloudFormation template (default: fetched from GitHub)")
	f.StringVar(&flags.quality, "quality", "80", "Image quality (0-100)")
	f.StringVar(&flags.fit, "fit", "outside", "Default resize fit mode")
	f.StringVar(&flags.logLevel, "log-level", "info", "Log level (error/warn/info/debug)")
	f.StringVar(&flags.originWhitelist, "origin-whitelist", "*", "Comma-separated origin whitelist")
	f.StringVar(&flags.redirectOnError, "redirect-on-error", "false", "Redirect to original image on error")
	f.StringVar(&flags.webp, "webp", "true", "Enable WebP format")
	f.StringVar(&flags.avif, "avif", "true", "Enable AVIF format")
	f.StringVar(&flags.avifMaxMp, "avif-max-mp", "2", "Maximum megapixels for AVIF output")
	f.StringVar(&flags.environment, "environment", "production", "Environment name")
	f.StringVar(&flags.apiEndpoint, "api-endpoint", "/api/v1/image", "API endpoint path")
	f.StringVar(&flags.imageTag, "image-tag", "latest", "Docker Hub image tag to deploy")
}

func runDeploy(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	// Load AWS config
	cfg, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(flags.region))
	if err != nil {
		return fmt.Errorf("loading AWS config: %w", err)
	}

	// Get CloudFormation template body
	templateBody, err := getTemplateBody(flags.template)
	if err != nil {
		return fmt.Errorf("getting template: %w", err)
	}

	// Set up ECR and push image
	fmt.Println("Setting up ECR repository...")
	imageURI, err := setupECRAndPushImage(ctx, cfg)
	if err != nil {
		return fmt.Errorf("setting up ECR image: %w", err)
	}
	fmt.Printf("Image pushed to: %s\n", imageURI)

	// Deploy CloudFormation stack
	stackName := fmt.Sprintf("eagle-image-api-%s", flags.stage)
	fmt.Printf("Deploying stack %q in %s...\n", stackName, flags.region)

	err = deployStack(ctx, cfg, stackName, templateBody, imageURI)
	if err != nil {
		return fmt.Errorf("deploying stack: %w", err)
	}

	// Print outputs
	return printStackOutputs(ctx, cfg, stackName)
}

func getTemplateBody(localPath string) (string, error) {
	if localPath != "" {
		data, err := os.ReadFile(localPath)
		if err != nil {
			return "", fmt.Errorf("reading local template %q: %w", localPath, err)
		}
		return string(data), nil
	}

	fmt.Println("Fetching CloudFormation template from GitHub...")
	resp, err := http.Get(defaultTemplateURL)
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

func setupECRAndPushImage(ctx context.Context, cfg aws.Config) (string, error) {
	ecrClient := ecr.NewFromConfig(cfg)

	// Create ECR repository if it doesn't exist
	repoURI, err := ensureECRRepo(ctx, ecrClient)
	if err != nil {
		return "", err
	}

	// Get ECR auth token
	authToken, endpoint, err := getECRAuth(ctx, ecrClient)
	if err != nil {
		return "", fmt.Errorf("getting ECR auth: %w", err)
	}

	// Docker login to ECR
	if err := dockerLogin(authToken, endpoint); err != nil {
		return "", fmt.Errorf("docker login to ECR: %w", err)
	}

	sourceImage := fmt.Sprintf("nicobistolfi/eagle-image-api:%s", flags.imageTag)
	targetImage := fmt.Sprintf("%s:%s", repoURI, flags.imageTag)

	// Pull from Docker Hub
	fmt.Printf("Pulling %s...\n", sourceImage)
	if err := runDockerCmd("pull", sourceImage); err != nil {
		return "", fmt.Errorf("pulling image: %w", err)
	}

	// Tag for ECR
	if err := runDockerCmd("tag", sourceImage, targetImage); err != nil {
		return "", fmt.Errorf("tagging image: %w", err)
	}

	// Push to ECR
	fmt.Printf("Pushing to ECR: %s...\n", targetImage)
	if err := runDockerCmd("push", targetImage); err != nil {
		return "", fmt.Errorf("pushing image to ECR: %w", err)
	}

	return targetImage, nil
}

func ensureECRRepo(ctx context.Context, client *ecr.Client) (string, error) {
	desc, err := client.DescribeRepositories(ctx, &ecr.DescribeRepositoriesInput{
		RepositoryNames: []string{ecrRepoName},
	})
	if err == nil && len(desc.Repositories) > 0 {
		return aws.ToString(desc.Repositories[0].RepositoryUri), nil
	}

	// Create the repository
	fmt.Printf("Creating ECR repository %q...\n", ecrRepoName)
	out, err := client.CreateRepository(ctx, &ecr.CreateRepositoryInput{
		RepositoryName:     aws.String(ecrRepoName),
		ImageTagMutability: ecrtypes.ImageTagMutabilityMutable,
	})
	if err != nil {
		return "", fmt.Errorf("creating ECR repository: %w", err)
	}
	return aws.ToString(out.Repository.RepositoryUri), nil
}

func getECRAuth(ctx context.Context, client *ecr.Client) (string, string, error) {
	out, err := client.GetAuthorizationToken(ctx, &ecr.GetAuthorizationTokenInput{})
	if err != nil {
		return "", "", err
	}
	if len(out.AuthorizationData) == 0 {
		return "", "", fmt.Errorf("no authorization data returned")
	}
	auth := out.AuthorizationData[0]
	return aws.ToString(auth.AuthorizationToken), aws.ToString(auth.ProxyEndpoint), nil
}

func dockerLogin(authToken, endpoint string) error {
	decoded, err := base64.StdEncoding.DecodeString(authToken)
	if err != nil {
		return fmt.Errorf("decoding auth token: %w", err)
	}
	parts := strings.SplitN(string(decoded), ":", 2)
	if len(parts) != 2 {
		return fmt.Errorf("unexpected auth token format")
	}

	cmd := exec.Command("docker", "login", "--username", parts[0], "--password-stdin", endpoint)
	cmd.Stdin = strings.NewReader(parts[1])
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func runDockerCmd(args ...string) error {
	cmd := exec.Command("docker", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func deployStack(ctx context.Context, cfg aws.Config, stackName, templateBody, imageURI string) error {
	cfClient := cloudformation.NewFromConfig(cfg)

	params := []cftypes.Parameter{
		{ParameterKey: aws.String("Stage"), ParameterValue: aws.String(flags.stage)},
		{ParameterKey: aws.String("ImageUri"), ParameterValue: aws.String(imageURI)},
		{ParameterKey: aws.String("Environment"), ParameterValue: aws.String(flags.environment)},
		{ParameterKey: aws.String("ApiEndpoint"), ParameterValue: aws.String(flags.apiEndpoint)},
		{ParameterKey: aws.String("Quality"), ParameterValue: aws.String(flags.quality)},
		{ParameterKey: aws.String("Fit"), ParameterValue: aws.String(flags.fit)},
		{ParameterKey: aws.String("LogLevel"), ParameterValue: aws.String(flags.logLevel)},
		{ParameterKey: aws.String("OriginWhitelist"), ParameterValue: aws.String(flags.originWhitelist)},
		{ParameterKey: aws.String("RedirectOnError"), ParameterValue: aws.String(flags.redirectOnError)},
		{ParameterKey: aws.String("WebP"), ParameterValue: aws.String(flags.webp)},
		{ParameterKey: aws.String("Avif"), ParameterValue: aws.String(flags.avif)},
		{ParameterKey: aws.String("AvifMaxMp"), ParameterValue: aws.String(flags.avifMaxMp)},
	}

	// Check if stack exists
	_, err := cfClient.DescribeStacks(ctx, &cloudformation.DescribeStacksInput{
		StackName: aws.String(stackName),
	})

	if err != nil {
		// Stack doesn't exist, create it
		fmt.Println("Creating new stack...")
		_, err = cfClient.CreateStack(ctx, &cloudformation.CreateStackInput{
			StackName:    aws.String(stackName),
			TemplateBody: aws.String(templateBody),
			Parameters:   params,
			Capabilities: []cftypes.Capability{cftypes.CapabilityCapabilityNamedIam},
			Tags: []cftypes.Tag{
				{Key: aws.String("Project"), Value: aws.String("eagle-image-api")},
				{Key: aws.String("Stage"), Value: aws.String(flags.stage)},
				{Key: aws.String("ManagedBy"), Value: aws.String("eagle-cli")},
			},
		})
		if err != nil {
			return fmt.Errorf("creating stack: %w", err)
		}
		fmt.Println("Waiting for stack creation to complete...")
		return waitForStack(ctx, cfClient, stackName)
	}

	// Stack exists, update it
	fmt.Println("Updating existing stack...")
	_, err = cfClient.UpdateStack(ctx, &cloudformation.UpdateStackInput{
		StackName:    aws.String(stackName),
		TemplateBody: aws.String(templateBody),
		Parameters:   params,
		Capabilities: []cftypes.Capability{cftypes.CapabilityCapabilityNamedIam},
		Tags: []cftypes.Tag{
			{Key: aws.String("Project"), Value: aws.String("eagle-image-api")},
			{Key: aws.String("Stage"), Value: aws.String(flags.stage)},
			{Key: aws.String("ManagedBy"), Value: aws.String("eagle-cli")},
		},
	})
	if err != nil {
		if strings.Contains(err.Error(), "No updates are to be performed") {
			fmt.Println("Stack is already up to date.")
			return nil
		}
		return fmt.Errorf("updating stack: %w", err)
	}
	fmt.Println("Waiting for stack update to complete...")
	return waitForStack(ctx, cfClient, stackName)
}

func waitForStack(ctx context.Context, client *cloudformation.Client, stackName string) error {
	for {
		out, err := client.DescribeStacks(ctx, &cloudformation.DescribeStacksInput{
			StackName: aws.String(stackName),
		})
		if err != nil {
			return fmt.Errorf("describing stack: %w", err)
		}
		if len(out.Stacks) == 0 {
			return fmt.Errorf("stack %q not found", stackName)
		}

		status := out.Stacks[0].StackStatus
		switch status {
		case cftypes.StackStatusCreateComplete, cftypes.StackStatusUpdateComplete:
			fmt.Printf("Stack %s completed successfully.\n", status)
			return nil
		case cftypes.StackStatusCreateFailed, cftypes.StackStatusRollbackComplete,
			cftypes.StackStatusRollbackFailed, cftypes.StackStatusUpdateRollbackComplete,
			cftypes.StackStatusUpdateRollbackFailed, cftypes.StackStatusDeleteComplete,
			cftypes.StackStatusDeleteFailed:
			reason := ""
			if out.Stacks[0].StackStatusReason != nil {
				reason = ": " + *out.Stacks[0].StackStatusReason
			}
			return fmt.Errorf("stack reached terminal status %s%s", status, reason)
		default:
			fmt.Printf("  Status: %s...\n", status)
			time.Sleep(10 * time.Second)
		}
	}
}

func printStackOutputs(ctx context.Context, cfg aws.Config, stackName string) error {
	cfClient := cloudformation.NewFromConfig(cfg)
	out, err := cfClient.DescribeStacks(ctx, &cloudformation.DescribeStacksInput{
		StackName: aws.String(stackName),
	})
	if err != nil {
		return fmt.Errorf("describing stack outputs: %w", err)
	}
	if len(out.Stacks) == 0 {
		return fmt.Errorf("stack %q not found", stackName)
	}

	fmt.Println("\n=== Deployment Outputs ===")
	for _, output := range out.Stacks[0].Outputs {
		fmt.Printf("  %s: %s\n", aws.ToString(output.OutputKey), aws.ToString(output.OutputValue))
	}

	// Print user-friendly summary
	for _, output := range out.Stacks[0].Outputs {
		key := aws.ToString(output.OutputKey)
		val := aws.ToString(output.OutputValue)
		switch key {
		case "ApiUrl":
			fmt.Printf("\nAPI Gateway URL: %s\n", val)
		case "CloudFrontUrl":
			fmt.Printf("CloudFront URL:  %s\n", val)
		}
	}

	return nil
}
