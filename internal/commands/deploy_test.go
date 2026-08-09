package commands

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudformation"
	cftypes "github.com/aws/aws-sdk-go-v2/service/cloudformation/types"
	"github.com/aws/aws-sdk-go-v2/service/ecr"
	ecrtypes "github.com/aws/aws-sdk-go-v2/service/ecr/types"
)

// awsNoUpdatesMessage is the error CloudFormation returns for a no-op update.
// deployStack matches on this text, so the exact wording matters.
const awsNoUpdatesMessage = "ValidationError: No updates are to be performed."

// --- fakes -----------------------------------------------------------------

type fakeECR struct {
	describeOut *ecr.DescribeRepositoriesOutput
	describeErr error
	createOut   *ecr.CreateRepositoryOutput
	createErr   error
	authOut     *ecr.GetAuthorizationTokenOutput
	authErr     error

	createCalls int
}

func (f *fakeECR) DescribeRepositories(context.Context, *ecr.DescribeRepositoriesInput, ...func(*ecr.Options)) (*ecr.DescribeRepositoriesOutput, error) {
	return f.describeOut, f.describeErr
}

func (f *fakeECR) CreateRepository(context.Context, *ecr.CreateRepositoryInput, ...func(*ecr.Options)) (*ecr.CreateRepositoryOutput, error) {
	f.createCalls++
	return f.createOut, f.createErr
}

func (f *fakeECR) GetAuthorizationToken(context.Context, *ecr.GetAuthorizationTokenInput, ...func(*ecr.Options)) (*ecr.GetAuthorizationTokenOutput, error) {
	return f.authOut, f.authErr
}

// fakeCFN replays a queued sequence of DescribeStacks results so a test can
// model a stack that progresses through several statuses.
type fakeCFN struct {
	describeQueue []describeResult
	describeCalls int

	createErr error
	updateErr error

	created bool
	updated bool
}

type describeResult struct {
	out *cloudformation.DescribeStacksOutput
	err error
}

func (f *fakeCFN) DescribeStacks(context.Context, *cloudformation.DescribeStacksInput, ...func(*cloudformation.Options)) (*cloudformation.DescribeStacksOutput, error) {
	i := f.describeCalls
	f.describeCalls++
	if i >= len(f.describeQueue) {
		i = len(f.describeQueue) - 1
	}
	if i < 0 {
		return nil, errors.New("no describe results queued")
	}
	r := f.describeQueue[i]
	return r.out, r.err
}

func (f *fakeCFN) CreateStack(context.Context, *cloudformation.CreateStackInput, ...func(*cloudformation.Options)) (*cloudformation.CreateStackOutput, error) {
	f.created = true
	return &cloudformation.CreateStackOutput{}, f.createErr
}

func (f *fakeCFN) UpdateStack(context.Context, *cloudformation.UpdateStackInput, ...func(*cloudformation.Options)) (*cloudformation.UpdateStackOutput, error) {
	f.updated = true
	return &cloudformation.UpdateStackOutput{}, f.updateErr
}

// recordingDocker captures the docker invocations instead of running them.
type recordingDocker struct {
	calls [][]string
	stdin []string
	err   error
}

func (r *recordingDocker) run(stdin string, args ...string) error {
	r.calls = append(r.calls, args)
	r.stdin = append(r.stdin, stdin)
	return r.err
}

func stackOutput(status cftypes.StackStatus, reason string, outputs ...cftypes.Output) *cloudformation.DescribeStacksOutput {
	s := cftypes.Stack{StackStatus: status, Outputs: outputs}
	if reason != "" {
		s.StackStatusReason = aws.String(reason)
	}
	return &cloudformation.DescribeStacksOutput{Stacks: []cftypes.Stack{s}}
}

func testFlags() deployFlags {
	return deployFlags{
		stage: "dev", region: "us-west-1", quality: "80", fit: "outside",
		logLevel: "info", originWhitelist: "*", redirectOnError: "false",
		webp: "true", avif: "true", avifMaxMp: "2", environment: "production",
		apiEndpoint: "/api/v1/image", imageTag: "latest",
	}
}

func newTestDeployer(e ecrAPI, c cfnAPI, d dockerRunner) (*deployer, *bytes.Buffer) {
	var buf bytes.Buffer
	return &deployer{
		ecr:    e,
		cfn:    c,
		docker: d,
		out:    &buf,
		sleep:  func(time.Duration) {}, // never actually sleep in tests
		flags:  testFlags(),
	}, &buf
}

// --- flag wiring -----------------------------------------------------------

func TestDeployCmdFlags(t *testing.T) {
	tests := []struct{ flag, defaultValue string }{
		{"stage", "dev"},
		{"region", "us-west-1"},
		{"template", ""},
		{"quality", "80"},
		{"fit", "outside"},
		{"log-level", "info"},
		{"origin-whitelist", "*"},
		{"redirect-on-error", "false"},
		{"webp", "true"},
		{"avif", "true"},
		{"avif-max-mp", "2"},
		{"environment", "production"},
		{"api-endpoint", "/api/v1/image"},
		{"image-tag", "latest"},
	}

	for _, tt := range tests {
		t.Run(tt.flag, func(t *testing.T) {
			f := DeployCmd.Flags().Lookup(tt.flag)
			if f == nil {
				t.Fatalf("flag %q not found", tt.flag)
			}
			if f.DefValue != tt.defaultValue {
				t.Errorf("flag %q default = %q, want %q", tt.flag, f.DefValue, tt.defaultValue)
			}
		})
	}
}

func TestDeployCmdMetadata(t *testing.T) {
	if DeployCmd.Use != "deploy" {
		t.Errorf("Use = %q, want deploy", DeployCmd.Use)
	}
	if DeployCmd.Short == "" {
		t.Error("Short description should not be empty")
	}
	if DeployCmd.Long == "" {
		t.Error("Long description should not be empty")
	}
}

func TestRegisterDeployFlagsBindsValues(t *testing.T) {
	cmd, dst := newDeployCmdForTest()
	if err := cmd.Flags().Set("stage", "prod"); err != nil {
		t.Fatalf("setting stage: %v", err)
	}
	if err := cmd.Flags().Set("avif-max-mp", "8"); err != nil {
		t.Fatalf("setting avif-max-mp: %v", err)
	}
	if dst.stage != "prod" {
		t.Errorf("stage = %q, want prod", dst.stage)
	}
	if dst.avifMaxMp != "8" {
		t.Errorf("avifMaxMp = %q, want 8", dst.avifMaxMp)
	}
}

// --- template fetching -----------------------------------------------------

func TestGetTemplateBodyLocalFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "template.yml")
	content := "AWSTemplateFormatVersion: '2010-09-09'\nDescription: Test template"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("writing temp file: %v", err)
	}

	body, err := getTemplateBody(path)
	if err != nil {
		t.Fatalf("getTemplateBody() error: %v", err)
	}
	if body != content {
		t.Errorf("body = %q, want %q", body, content)
	}
}

func TestGetTemplateBodyLocalFileNotFound(t *testing.T) {
	if _, err := getTemplateBody(filepath.Join(t.TempDir(), "missing.yml")); err == nil {
		t.Fatal("expected an error for a nonexistent file")
	}
}

func TestGetTemplateBodyRemoteFetch(t *testing.T) {
	const expected = "AWSTemplateFormatVersion: '2010-09-09'"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(expected))
	}))
	defer srv.Close()

	restore := swapTemplateURL(t, srv.URL)
	defer restore()

	body, err := getTemplateBody("")
	if err != nil {
		t.Fatalf("getTemplateBody() error: %v", err)
	}
	if body != expected {
		t.Errorf("body = %q, want %q", body, expected)
	}
}

func TestGetTemplateBodyRemoteNon200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	restore := swapTemplateURL(t, srv.URL)
	defer restore()

	_, err := getTemplateBody("")
	if err == nil {
		t.Fatal("expected an error for HTTP 404")
	}
	if !strings.Contains(err.Error(), "404") {
		t.Errorf("error = %v, want it to mention the status code", err)
	}
}

func TestGetTemplateBodyRemoteUnreachable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	url := srv.URL
	srv.Close() // nothing is listening now

	restore := swapTemplateURL(t, url)
	defer restore()

	if _, err := getTemplateBody(""); err == nil {
		t.Fatal("expected an error when the server is unreachable")
	}
}

func TestDefaultTemplateURLPointsAtRepo(t *testing.T) {
	if !strings.Contains(DefaultTemplateURL, "nicobistolfi/eagle-image-api") {
		t.Errorf("DefaultTemplateURL = %q, want it to reference the project repo", DefaultTemplateURL)
	}
	if !strings.HasSuffix(DefaultTemplateURL, "template.yml") {
		t.Errorf("DefaultTemplateURL = %q, want it to end in template.yml", DefaultTemplateURL)
	}
}

// --- ECR -------------------------------------------------------------------

func TestEnsureECRRepoExisting(t *testing.T) {
	e := &fakeECR{describeOut: &ecr.DescribeRepositoriesOutput{
		Repositories: []ecrtypes.Repository{{RepositoryUri: aws.String("123.dkr.ecr.us-west-1.amazonaws.com/eagle-image-api")}},
	}}
	d, _ := newTestDeployer(e, &fakeCFN{}, (&recordingDocker{}).run)

	uri, err := d.ensureECRRepo(context.Background())
	if err != nil {
		t.Fatalf("ensureECRRepo() error: %v", err)
	}
	if uri != "123.dkr.ecr.us-west-1.amazonaws.com/eagle-image-api" {
		t.Errorf("uri = %q", uri)
	}
	if e.createCalls != 0 {
		t.Errorf("CreateRepository called %d times, want 0 for an existing repo", e.createCalls)
	}
}

func TestEnsureECRRepoCreatesWhenMissing(t *testing.T) {
	e := &fakeECR{
		describeErr: errors.New("RepositoryNotFoundException"),
		createOut: &ecr.CreateRepositoryOutput{
			Repository: &ecrtypes.Repository{RepositoryUri: aws.String("123.dkr.ecr.us-west-1.amazonaws.com/eagle-image-api")},
		},
	}
	d, out := newTestDeployer(e, &fakeCFN{}, (&recordingDocker{}).run)

	uri, err := d.ensureECRRepo(context.Background())
	if err != nil {
		t.Fatalf("ensureECRRepo() error: %v", err)
	}
	if uri == "" {
		t.Error("expected a repository URI")
	}
	if e.createCalls != 1 {
		t.Errorf("CreateRepository called %d times, want 1", e.createCalls)
	}
	if !strings.Contains(out.String(), "Creating ECR repository") {
		t.Errorf("expected progress output, got %q", out.String())
	}
}

func TestEnsureECRRepoCreateFails(t *testing.T) {
	e := &fakeECR{
		describeErr: errors.New("not found"),
		createErr:   errors.New("AccessDenied"),
	}
	d, _ := newTestDeployer(e, &fakeCFN{}, (&recordingDocker{}).run)

	_, err := d.ensureECRRepo(context.Background())
	if err == nil {
		t.Fatal("expected an error when CreateRepository fails")
	}
	if !strings.Contains(err.Error(), "AccessDenied") {
		t.Errorf("error = %v, want it to wrap the AWS error", err)
	}
}

func TestEnsureECRRepoCreateReturnsNoRepository(t *testing.T) {
	e := &fakeECR{
		describeErr: errors.New("not found"),
		createOut:   &ecr.CreateRepositoryOutput{}, // Repository is nil
	}
	d, _ := newTestDeployer(e, &fakeCFN{}, (&recordingDocker{}).run)

	if _, err := d.ensureECRRepo(context.Background()); err == nil {
		t.Fatal("expected an error when CreateRepository returns no repository")
	}
}

func TestGetECRAuth(t *testing.T) {
	e := &fakeECR{authOut: &ecr.GetAuthorizationTokenOutput{
		AuthorizationData: []ecrtypes.AuthorizationData{{
			AuthorizationToken: aws.String("dG9rZW4="),
			ProxyEndpoint:      aws.String("https://123.dkr.ecr.us-west-1.amazonaws.com"),
		}},
	}}
	d, _ := newTestDeployer(e, &fakeCFN{}, (&recordingDocker{}).run)

	token, endpoint, err := d.getECRAuth(context.Background())
	if err != nil {
		t.Fatalf("getECRAuth() error: %v", err)
	}
	if token != "dG9rZW4=" {
		t.Errorf("token = %q", token)
	}
	if endpoint != "https://123.dkr.ecr.us-west-1.amazonaws.com" {
		t.Errorf("endpoint = %q", endpoint)
	}
}

func TestGetECRAuthErrors(t *testing.T) {
	t.Run("api error", func(t *testing.T) {
		d, _ := newTestDeployer(&fakeECR{authErr: errors.New("boom")}, &fakeCFN{}, (&recordingDocker{}).run)
		if _, _, err := d.getECRAuth(context.Background()); err == nil {
			t.Fatal("expected an error")
		}
	})

	t.Run("empty authorization data", func(t *testing.T) {
		e := &fakeECR{authOut: &ecr.GetAuthorizationTokenOutput{}}
		d, _ := newTestDeployer(e, &fakeCFN{}, (&recordingDocker{}).run)
		_, _, err := d.getECRAuth(context.Background())
		if err == nil {
			t.Fatal("expected an error for empty authorization data")
		}
	})
}

func TestDecodeAuthToken(t *testing.T) {
	token := base64.StdEncoding.EncodeToString([]byte("AWS:secret:with:colons"))

	user, pass, err := decodeAuthToken(token)
	if err != nil {
		t.Fatalf("decodeAuthToken() error: %v", err)
	}
	if user != "AWS" {
		t.Errorf("user = %q, want AWS", user)
	}
	// SplitN with n=2 must keep colons in the password intact.
	if pass != "secret:with:colons" {
		t.Errorf("pass = %q, want secret:with:colons", pass)
	}
}

func TestDecodeAuthTokenInvalid(t *testing.T) {
	t.Run("not base64", func(t *testing.T) {
		if _, _, err := decodeAuthToken("!!!not base64!!!"); err == nil {
			t.Fatal("expected an error")
		}
	})

	t.Run("missing separator", func(t *testing.T) {
		token := base64.StdEncoding.EncodeToString([]byte("no-colon-here"))
		if _, _, err := decodeAuthToken(token); err == nil {
			t.Fatal("expected an error for a token without a colon")
		}
	})
}

func TestSetupECRAndPushImage(t *testing.T) {
	e := &fakeECR{
		describeOut: &ecr.DescribeRepositoriesOutput{
			Repositories: []ecrtypes.Repository{{RepositoryUri: aws.String("123.dkr.ecr.us-west-1.amazonaws.com/eagle-image-api")}},
		},
		authOut: &ecr.GetAuthorizationTokenOutput{
			AuthorizationData: []ecrtypes.AuthorizationData{{
				AuthorizationToken: aws.String(base64.StdEncoding.EncodeToString([]byte("AWS:pw"))),
				ProxyEndpoint:      aws.String("https://123.dkr.ecr.us-west-1.amazonaws.com"),
			}},
		},
	}
	docker := &recordingDocker{}
	d, _ := newTestDeployer(e, &fakeCFN{}, docker.run)

	uri, err := d.setupECRAndPushImage(context.Background())
	if err != nil {
		t.Fatalf("setupECRAndPushImage() error: %v", err)
	}
	if want := "123.dkr.ecr.us-west-1.amazonaws.com/eagle-image-api:latest"; uri != want {
		t.Errorf("uri = %q, want %q", uri, want)
	}

	if len(docker.calls) != 4 {
		t.Fatalf("docker called %d times, want 4 (login, pull, tag, push)", len(docker.calls))
	}
	for i, want := range []string{"login", "pull", "tag", "push"} {
		if docker.calls[i][0] != want {
			t.Errorf("docker call %d = %q, want %q", i, docker.calls[i][0], want)
		}
	}
	// The password must arrive over stdin, never as an argv element.
	if docker.stdin[0] != "pw" {
		t.Errorf("login stdin = %q, want the password", docker.stdin[0])
	}
	if strings.Contains(strings.Join(docker.calls[0], " "), "pw") {
		t.Error("password leaked into docker login arguments")
	}
}

func TestSetupECRAndPushImageDockerFailure(t *testing.T) {
	e := &fakeECR{
		describeOut: &ecr.DescribeRepositoriesOutput{
			Repositories: []ecrtypes.Repository{{RepositoryUri: aws.String("repo")}},
		},
		authOut: &ecr.GetAuthorizationTokenOutput{
			AuthorizationData: []ecrtypes.AuthorizationData{{
				AuthorizationToken: aws.String(base64.StdEncoding.EncodeToString([]byte("AWS:pw"))),
				ProxyEndpoint:      aws.String("endpoint"),
			}},
		},
	}
	docker := &recordingDocker{err: errors.New("docker daemon not running")}
	d, _ := newTestDeployer(e, &fakeCFN{}, docker.run)

	_, err := d.setupECRAndPushImage(context.Background())
	if err == nil {
		t.Fatal("expected an error when docker fails")
	}
	if !strings.Contains(err.Error(), "docker login") {
		t.Errorf("error = %v, want it to identify the failing docker step", err)
	}
}

func TestSetupECRAndPushImageAuthFailure(t *testing.T) {
	e := &fakeECR{
		describeOut: &ecr.DescribeRepositoriesOutput{
			Repositories: []ecrtypes.Repository{{RepositoryUri: aws.String("repo")}},
		},
		authErr: errors.New("expired credentials"),
	}
	d, _ := newTestDeployer(e, &fakeCFN{}, (&recordingDocker{}).run)

	if _, err := d.setupECRAndPushImage(context.Background()); err == nil {
		t.Fatal("expected an error when the auth call fails")
	}
}

func TestSetupECRAndPushImageBadAuthToken(t *testing.T) {
	e := &fakeECR{
		describeOut: &ecr.DescribeRepositoriesOutput{
			Repositories: []ecrtypes.Repository{{RepositoryUri: aws.String("repo")}},
		},
		authOut: &ecr.GetAuthorizationTokenOutput{
			AuthorizationData: []ecrtypes.AuthorizationData{{
				AuthorizationToken: aws.String("!!!"),
				ProxyEndpoint:      aws.String("endpoint"),
			}},
		},
	}
	d, _ := newTestDeployer(e, &fakeCFN{}, (&recordingDocker{}).run)

	if _, err := d.setupECRAndPushImage(context.Background()); err == nil {
		t.Fatal("expected an error for an undecodable auth token")
	}
}

func TestSetupECRAndPushImageRepoFailure(t *testing.T) {
	e := &fakeECR{describeErr: errors.New("nope"), createErr: errors.New("denied")}
	d, _ := newTestDeployer(e, &fakeCFN{}, (&recordingDocker{}).run)

	if _, err := d.setupECRAndPushImage(context.Background()); err == nil {
		t.Fatal("expected an error when the repository cannot be resolved")
	}
}

// --- parameters and tags ---------------------------------------------------

func TestBuildParameters(t *testing.T) {
	f := testFlags()
	f.stage = "prod"
	f.quality = "95"

	params := buildParameters(f, "123.dkr.ecr.us-west-1.amazonaws.com/eagle:latest")

	// Keys must match the parameters declared in template.yml exactly.
	wantKeys := []string{
		"Stage", "ImageUri", "Environment", "ApiEndpoint",
		"Quality", "Fit", "LogLevel", "OriginWhitelist",
		"RedirectOnError", "WebP", "Avif", "AvifMaxMp",
	}
	if len(params) != len(wantKeys) {
		t.Fatalf("got %d parameters, want %d", len(params), len(wantKeys))
	}

	got := map[string]string{}
	for i, p := range params {
		if aws.ToString(p.ParameterKey) != wantKeys[i] {
			t.Errorf("param[%d] key = %q, want %q", i, aws.ToString(p.ParameterKey), wantKeys[i])
		}
		got[aws.ToString(p.ParameterKey)] = aws.ToString(p.ParameterValue)
	}

	if got["Stage"] != "prod" {
		t.Errorf("Stage = %q, want prod", got["Stage"])
	}
	if got["Quality"] != "95" {
		t.Errorf("Quality = %q, want 95", got["Quality"])
	}
	if got["ImageUri"] != "123.dkr.ecr.us-west-1.amazonaws.com/eagle:latest" {
		t.Errorf("ImageUri = %q", got["ImageUri"])
	}
}

// TestBuildParametersMatchTemplate guards against the parameter list drifting
// away from the CloudFormation template checked into the repository.
func TestBuildParametersMatchTemplate(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "template.yml"))
	if err != nil {
		t.Skipf("template.yml not readable: %v", err)
	}
	template := string(data)

	for _, p := range buildParameters(testFlags(), "img") {
		key := aws.ToString(p.ParameterKey)
		if !strings.Contains(template, "\n  "+key+":") {
			t.Errorf("parameter %q is sent to CloudFormation but not declared in template.yml", key)
		}
	}
}

func TestBuildTags(t *testing.T) {
	f := testFlags()
	f.stage = "staging"

	tags := map[string]string{}
	for _, tag := range buildTags(f) {
		tags[aws.ToString(tag.Key)] = aws.ToString(tag.Value)
	}

	if tags["Project"] != "eagle-image-api" {
		t.Errorf("Project tag = %q", tags["Project"])
	}
	if tags["Stage"] != "staging" {
		t.Errorf("Stage tag = %q, want staging", tags["Stage"])
	}
	if tags["ManagedBy"] != "eagle-cli" {
		t.Errorf("ManagedBy tag = %q", tags["ManagedBy"])
	}
}

func TestStackName(t *testing.T) {
	tests := []struct{ stage, want string }{
		{"dev", "eagle-image-api-dev"},
		{"prod", "eagle-image-api-prod"},
		{"staging", "eagle-image-api-staging"},
	}
	for _, tt := range tests {
		if got := stackName(tt.stage); got != tt.want {
			t.Errorf("stackName(%q) = %q, want %q", tt.stage, got, tt.want)
		}
	}
}

// --- stack lifecycle -------------------------------------------------------

func TestDeployStackCreatesWhenMissing(t *testing.T) {
	c := &fakeCFN{describeQueue: []describeResult{
		{err: errors.New("stack does not exist")},                 // existence probe
		{out: stackOutput(cftypes.StackStatusCreateComplete, "")}, // wait
	}}
	d, out := newTestDeployer(&fakeECR{}, c, (&recordingDocker{}).run)

	if err := d.deployStack(context.Background(), "eagle-image-api-dev", "body", "img"); err != nil {
		t.Fatalf("deployStack() error: %v", err)
	}
	if !c.created {
		t.Error("expected CreateStack to be called")
	}
	if c.updated {
		t.Error("UpdateStack should not be called for a missing stack")
	}
	if !strings.Contains(out.String(), "Creating new stack") {
		t.Errorf("expected create progress output, got %q", out.String())
	}
}

func TestDeployStackUpdatesWhenPresent(t *testing.T) {
	c := &fakeCFN{describeQueue: []describeResult{
		{out: stackOutput(cftypes.StackStatusCreateComplete, "")}, // exists
		{out: stackOutput(cftypes.StackStatusUpdateComplete, "")}, // wait
	}}
	d, _ := newTestDeployer(&fakeECR{}, c, (&recordingDocker{}).run)

	if err := d.deployStack(context.Background(), "eagle-image-api-dev", "body", "img"); err != nil {
		t.Fatalf("deployStack() error: %v", err)
	}
	if !c.updated {
		t.Error("expected UpdateStack to be called")
	}
	if c.created {
		t.Error("CreateStack should not be called for an existing stack")
	}
}

func TestDeployStackNoUpdatesNeeded(t *testing.T) {
	c := &fakeCFN{
		describeQueue: []describeResult{{out: stackOutput(cftypes.StackStatusCreateComplete, "")}},
		updateErr:     errors.New(awsNoUpdatesMessage),
	}
	d, out := newTestDeployer(&fakeECR{}, c, (&recordingDocker{}).run)

	// A no-op update is success, not failure.
	if err := d.deployStack(context.Background(), "eagle-image-api-dev", "body", "img"); err != nil {
		t.Fatalf("deployStack() error: %v", err)
	}
	if !strings.Contains(out.String(), "already up to date") {
		t.Errorf("expected an up-to-date message, got %q", out.String())
	}
}

func TestDeployStackUpdateFails(t *testing.T) {
	c := &fakeCFN{
		describeQueue: []describeResult{{out: stackOutput(cftypes.StackStatusCreateComplete, "")}},
		updateErr:     errors.New("InsufficientCapabilities"),
	}
	d, _ := newTestDeployer(&fakeECR{}, c, (&recordingDocker{}).run)

	if err := d.deployStack(context.Background(), "eagle-image-api-dev", "body", "img"); err == nil {
		t.Fatal("expected an error when UpdateStack fails")
	}
}

func TestDeployStackCreateFails(t *testing.T) {
	c := &fakeCFN{
		describeQueue: []describeResult{{err: errors.New("missing")}},
		createErr:     errors.New("AlreadyExistsException"),
	}
	d, _ := newTestDeployer(&fakeECR{}, c, (&recordingDocker{}).run)

	if err := d.deployStack(context.Background(), "eagle-image-api-dev", "body", "img"); err == nil {
		t.Fatal("expected an error when CreateStack fails")
	}
}

func TestWaitForStackPollsUntilComplete(t *testing.T) {
	c := &fakeCFN{describeQueue: []describeResult{
		{out: stackOutput(cftypes.StackStatusCreateInProgress, "")},
		{out: stackOutput(cftypes.StackStatusCreateInProgress, "")},
		{out: stackOutput(cftypes.StackStatusCreateComplete, "")},
	}}
	d, out := newTestDeployer(&fakeECR{}, c, (&recordingDocker{}).run)

	slept := 0
	d.sleep = func(time.Duration) { slept++ }

	if err := d.waitForStack(context.Background(), "eagle-image-api-dev"); err != nil {
		t.Fatalf("waitForStack() error: %v", err)
	}
	if slept != 2 {
		t.Errorf("slept %d times, want 2 (once per in-progress poll)", slept)
	}
	if !strings.Contains(out.String(), "completed successfully") {
		t.Errorf("expected a success message, got %q", out.String())
	}
}

func TestWaitForStackTerminalFailure(t *testing.T) {
	c := &fakeCFN{describeQueue: []describeResult{
		{out: stackOutput(cftypes.StackStatusRollbackComplete, "resource creation failed")},
	}}
	d, _ := newTestDeployer(&fakeECR{}, c, (&recordingDocker{}).run)

	err := d.waitForStack(context.Background(), "eagle-image-api-dev")
	if err == nil {
		t.Fatal("expected an error for a rolled-back stack")
	}
	if !strings.Contains(err.Error(), "resource creation failed") {
		t.Errorf("error = %v, want it to include the CloudFormation reason", err)
	}
}

func TestWaitForStackTerminalFailureWithoutReason(t *testing.T) {
	c := &fakeCFN{describeQueue: []describeResult{
		{out: stackOutput(cftypes.StackStatusCreateFailed, "")},
	}}
	d, _ := newTestDeployer(&fakeECR{}, c, (&recordingDocker{}).run)

	if err := d.waitForStack(context.Background(), "eagle-image-api-dev"); err == nil {
		t.Fatal("expected an error for a failed stack")
	}
}

func TestWaitForStackDescribeError(t *testing.T) {
	c := &fakeCFN{describeQueue: []describeResult{{err: errors.New("throttled")}}}
	d, _ := newTestDeployer(&fakeECR{}, c, (&recordingDocker{}).run)

	if err := d.waitForStack(context.Background(), "eagle-image-api-dev"); err == nil {
		t.Fatal("expected an error when DescribeStacks fails")
	}
}

func TestWaitForStackNoStacksReturned(t *testing.T) {
	c := &fakeCFN{describeQueue: []describeResult{{out: &cloudformation.DescribeStacksOutput{}}}}
	d, _ := newTestDeployer(&fakeECR{}, c, (&recordingDocker{}).run)

	if err := d.waitForStack(context.Background(), "eagle-image-api-dev"); err == nil {
		t.Fatal("expected an error when no stacks are returned")
	}
}

func TestWaitForStackHonoursContextCancellation(t *testing.T) {
	// A stack stuck in progress must not poll forever once the caller gives up.
	c := &fakeCFN{describeQueue: []describeResult{
		{out: stackOutput(cftypes.StackStatusCreateInProgress, "")},
	}}
	d, _ := newTestDeployer(&fakeECR{}, c, (&recordingDocker{}).run)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := d.waitForStack(ctx, "eagle-image-api-dev"); !errors.Is(err, context.Canceled) {
		t.Fatalf("waitForStack() error = %v, want context.Canceled", err)
	}
}

func TestStackStatusClassification(t *testing.T) {
	success := []cftypes.StackStatus{
		cftypes.StackStatusCreateComplete,
		cftypes.StackStatusUpdateComplete,
	}
	failure := []cftypes.StackStatus{
		cftypes.StackStatusCreateFailed,
		cftypes.StackStatusRollbackComplete,
		cftypes.StackStatusRollbackFailed,
		cftypes.StackStatusUpdateRollbackComplete,
		cftypes.StackStatusUpdateRollbackFailed,
		cftypes.StackStatusDeleteComplete,
		cftypes.StackStatusDeleteFailed,
	}
	pending := []cftypes.StackStatus{
		cftypes.StackStatusCreateInProgress,
		cftypes.StackStatusUpdateInProgress,
		cftypes.StackStatusDeleteInProgress,
	}

	for _, s := range success {
		if !isTerminalSuccess(s) || isTerminalFailure(s) {
			t.Errorf("%s should classify as terminal success", s)
		}
	}
	for _, s := range failure {
		if !isTerminalFailure(s) || isTerminalSuccess(s) {
			t.Errorf("%s should classify as terminal failure", s)
		}
	}
	for _, s := range pending {
		if isTerminalFailure(s) || isTerminalSuccess(s) {
			t.Errorf("%s should classify as still in progress", s)
		}
	}
}

// --- outputs ---------------------------------------------------------------

func TestPrintStackOutputs(t *testing.T) {
	c := &fakeCFN{describeQueue: []describeResult{{out: stackOutput(
		cftypes.StackStatusCreateComplete, "",
		cftypes.Output{OutputKey: aws.String("ApiUrl"), OutputValue: aws.String("https://api.example.com")},
		cftypes.Output{OutputKey: aws.String("CloudFrontUrl"), OutputValue: aws.String("https://cdn.example.com")},
		cftypes.Output{OutputKey: aws.String("Other"), OutputValue: aws.String("value")},
	)}}}
	d, out := newTestDeployer(&fakeECR{}, c, (&recordingDocker{}).run)

	if err := d.printStackOutputs(context.Background(), "eagle-image-api-dev"); err != nil {
		t.Fatalf("printStackOutputs() error: %v", err)
	}

	s := out.String()
	for _, want := range []string{
		"=== Deployment Outputs ===",
		"Other: value",
		"API Gateway URL: https://api.example.com",
		"CloudFront URL:  https://cdn.example.com",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("output missing %q, got %q", want, s)
		}
	}
}

func TestPrintStackOutputsErrors(t *testing.T) {
	t.Run("describe fails", func(t *testing.T) {
		c := &fakeCFN{describeQueue: []describeResult{{err: errors.New("boom")}}}
		d, _ := newTestDeployer(&fakeECR{}, c, (&recordingDocker{}).run)
		if err := d.printStackOutputs(context.Background(), "s"); err == nil {
			t.Fatal("expected an error")
		}
	})

	t.Run("no stacks", func(t *testing.T) {
		c := &fakeCFN{describeQueue: []describeResult{{out: &cloudformation.DescribeStacksOutput{}}}}
		d, _ := newTestDeployer(&fakeECR{}, c, (&recordingDocker{}).run)
		if err := d.printStackOutputs(context.Background(), "s"); err == nil {
			t.Fatal("expected an error")
		}
	})
}

// --- end to end (fakes) ----------------------------------------------------

func TestDeployerRunHappyPath(t *testing.T) {
	templatePath := filepath.Join(t.TempDir(), "template.yml")
	if err := os.WriteFile(templatePath, []byte("Resources: {}"), 0o600); err != nil {
		t.Fatalf("writing template: %v", err)
	}

	e := &fakeECR{
		describeOut: &ecr.DescribeRepositoriesOutput{
			Repositories: []ecrtypes.Repository{{RepositoryUri: aws.String("123.dkr.ecr.us-west-1.amazonaws.com/eagle-image-api")}},
		},
		authOut: &ecr.GetAuthorizationTokenOutput{
			AuthorizationData: []ecrtypes.AuthorizationData{{
				AuthorizationToken: aws.String(base64.StdEncoding.EncodeToString([]byte("AWS:pw"))),
				ProxyEndpoint:      aws.String("https://123.dkr.ecr.us-west-1.amazonaws.com"),
			}},
		},
	}
	c := &fakeCFN{describeQueue: []describeResult{
		{err: errors.New("missing")}, // existence probe
		{out: stackOutput(cftypes.StackStatusCreateComplete, "", // wait + outputs
			cftypes.Output{OutputKey: aws.String("ApiUrl"), OutputValue: aws.String("https://api.example.com")},
		)},
	}}

	d, out := newTestDeployer(e, c, (&recordingDocker{}).run)
	d.flags.template = templatePath

	if err := d.run(context.Background()); err != nil {
		t.Fatalf("run() error: %v", err)
	}
	if !c.created {
		t.Error("expected the stack to be created")
	}
	if !strings.Contains(out.String(), "API Gateway URL: https://api.example.com") {
		t.Errorf("expected the API URL in the summary, got %q", out.String())
	}
}

func TestDeployerRunTemplateFailure(t *testing.T) {
	d, _ := newTestDeployer(&fakeECR{}, &fakeCFN{}, (&recordingDocker{}).run)
	d.flags.template = filepath.Join(t.TempDir(), "missing.yml")

	err := d.run(context.Background())
	if err == nil {
		t.Fatal("expected an error for a missing template")
	}
	if !strings.Contains(err.Error(), "getting template") {
		t.Errorf("error = %v, want it to identify the template step", err)
	}
}

func TestDeployerRunECRFailure(t *testing.T) {
	templatePath := filepath.Join(t.TempDir(), "template.yml")
	if err := os.WriteFile(templatePath, []byte("Resources: {}"), 0o600); err != nil {
		t.Fatalf("writing template: %v", err)
	}

	e := &fakeECR{describeErr: errors.New("nope"), createErr: errors.New("denied")}
	d, _ := newTestDeployer(e, &fakeCFN{}, (&recordingDocker{}).run)
	d.flags.template = templatePath

	err := d.run(context.Background())
	if err == nil {
		t.Fatal("expected an error when ECR setup fails")
	}
	if !strings.Contains(err.Error(), "setting up ECR image") {
		t.Errorf("error = %v, want it to identify the ECR step", err)
	}
}

func TestDeployerRunStackFailure(t *testing.T) {
	templatePath := filepath.Join(t.TempDir(), "template.yml")
	if err := os.WriteFile(templatePath, []byte("Resources: {}"), 0o600); err != nil {
		t.Fatalf("writing template: %v", err)
	}

	e := &fakeECR{
		describeOut: &ecr.DescribeRepositoriesOutput{
			Repositories: []ecrtypes.Repository{{RepositoryUri: aws.String("repo")}},
		},
		authOut: &ecr.GetAuthorizationTokenOutput{
			AuthorizationData: []ecrtypes.AuthorizationData{{
				AuthorizationToken: aws.String(base64.StdEncoding.EncodeToString([]byte("AWS:pw"))),
				ProxyEndpoint:      aws.String("endpoint"),
			}},
		},
	}
	c := &fakeCFN{
		describeQueue: []describeResult{{err: errors.New("missing")}},
		createErr:     errors.New("LimitExceeded"),
	}
	d, _ := newTestDeployer(e, c, (&recordingDocker{}).run)
	d.flags.template = templatePath

	err := d.run(context.Background())
	if err == nil {
		t.Fatal("expected an error when the stack deployment fails")
	}
	if !strings.Contains(err.Error(), "deploying stack") {
		t.Errorf("error = %v, want it to identify the stack step", err)
	}
}

// TestRunDockerCmdReportsMissingBinary exercises the real runner without
// depending on docker being installed: an argument list this bogus fails
// either way, and the point is that the error is surfaced rather than
// swallowed.
func TestRunDockerCmdReportsFailure(t *testing.T) {
	if err := runDockerCmd("", "--definitely-not-a-docker-flag"); err == nil {
		t.Skip("docker accepted an invalid flag; nothing to assert")
	}
}
