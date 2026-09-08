// Command cli is a small developer helper: propose a project via the
// create_project skill, show the proposal, and only on explicit user
// confirmation commit it via POST /projects. There is no code path that
// commits without that confirmation (architecture.md §2 / AGENTS.MD rule
// 1: the LLM never writes to the datastore).
package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	v4 "github.com/aws/aws-sdk-go-v2/aws/signer/v4"

	"github.com/esvarez/lucas-assist/internal/agent"
	"github.com/esvarez/lucas-assist/internal/agent/skills"
	"github.com/esvarez/lucas-assist/internal/domain"
)

func main() {
	skillsURL := flag.String("skills-url", "", "Skill Lambda Function URL to call over HTTP, SigV4-signed (e.g. the deployed SkillsFunctionUrl output). Empty runs create_project in-process instead, for local dev.")
	apiURL := flag.String("api-url", "http://localhost:8080", "Base URL of the API Lambda to POST /projects against: cmd/local's default, or the deployed ApiUrl output.")
	flag.Parse()

	if err := run(context.Background(), *skillsURL, *apiURL, flag.Args()); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, skillsURL, apiURL string, args []string) error {
	description, err := readDescription(args, os.Stdin)
	if err != nil {
		return err
	}

	result, err := propose(ctx, skillsURL, description)
	if err != nil {
		return fmt.Errorf("propose project: %w", err)
	}

	if result.Status == "needs_clarification" {
		fmt.Println("Needs clarification before this can be proposed:")
		for _, q := range result.Questions {
			fmt.Println(" -", q)
		}
		return nil
	}

	if result.Project == nil {
		return fmt.Errorf(`skill returned status %q but no project`, result.Status)
	}

	printProject(*result.Project)

	if !confirm("Commit this project? [y/N] ") {
		fmt.Println("Not committing.")
		return nil
	}

	created, err := commit(ctx, apiURL, *result.Project)
	if err != nil {
		return fmt.Errorf("commit project: %w", err)
	}

	fmt.Printf("Created project %q (id=%s)\n", created.Name, created.ID)
	return nil
}

// readDescription takes the description from args if given, otherwise
// reads it from stdin (piped input only — never blocks on an interactive
// terminal with nothing to read).
func readDescription(args []string, stdin *os.File) (string, error) {
	if len(args) > 0 {
		return strings.Join(args, " "), nil
	}

	if stat, err := stdin.Stat(); err == nil && stat.Mode()&os.ModeCharDevice == 0 {
		b, err := io.ReadAll(stdin)
		if err != nil {
			return "", fmt.Errorf("read stdin: %w", err)
		}
		if desc := strings.TrimSpace(string(b)); desc != "" {
			return desc, nil
		}
	}

	return "", fmt.Errorf("no description given: pass it as an argument or pipe it via stdin")
}

// propose calls the create_project skill, either in-process (skillsURL
// empty, for local dev) or over HTTP against a real Skill Lambda.
func propose(ctx context.Context, skillsURL, description string) (skills.CreateProjectResult, error) {
	input, err := json.Marshal(skills.CreateProjectInput{Description: description})
	if err != nil {
		return skills.CreateProjectResult{}, fmt.Errorf("marshal input: %w", err)
	}

	if skillsURL == "" {
		got, err := agent.Run(ctx, skills.CreateProjectSkill{}, input)
		if err != nil {
			return skills.CreateProjectResult{}, err
		}
		result, ok := got.(skills.CreateProjectResult)
		if !ok {
			return skills.CreateProjectResult{}, fmt.Errorf("unexpected result type %T", got)
		}
		return result, nil
	}

	return proposeRemote(ctx, skillsURL, input)
}

type skillRequestEnvelope struct {
	Skill string          `json:"skill"`
	Input json.RawMessage `json:"input"`
}

// proposeRemote calls a deployed Skill Lambda Function URL. Function URLs
// on this project use AuthType: AWS_IAM (template.yaml), so the request
// is signed with the caller's default AWS credentials — same credential
// chain internal/store.NewDynamoDBClient already relies on.
func proposeRemote(ctx context.Context, skillsURL string, input json.RawMessage) (skills.CreateProjectResult, error) {
	body, err := json.Marshal(skillRequestEnvelope{Skill: "create_project", Input: input})
	if err != nil {
		return skills.CreateProjectResult{}, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, skillsURL, bytes.NewReader(body))
	if err != nil {
		return skills.CreateProjectResult{}, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	if err := signForLambda(ctx, req, body); err != nil {
		return skills.CreateProjectResult{}, fmt.Errorf("sign request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return skills.CreateProjectResult{}, fmt.Errorf("call skills endpoint: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return skills.CreateProjectResult{}, fmt.Errorf("read response: %w", err)
	}

	return decodeSkillResponse(resp.StatusCode, respBody)
}

func decodeSkillResponse(status int, body []byte) (skills.CreateProjectResult, error) {
	if status != http.StatusOK {
		return skills.CreateProjectResult{}, fmt.Errorf("skills endpoint returned %d: %s", status, body)
	}

	var result skills.CreateProjectResult
	if err := json.Unmarshal(body, &result); err != nil {
		return skills.CreateProjectResult{}, fmt.Errorf("unmarshal response: %w", err)
	}
	return result, nil
}

// signForLambda SigV4-signs req for the "lambda" service (Function URLs),
// using the default AWS credential chain and region.
func signForLambda(ctx context.Context, req *http.Request, body []byte) error {
	cfg, err := awsconfig.LoadDefaultConfig(ctx)
	if err != nil {
		return fmt.Errorf("load AWS config: %w", err)
	}
	creds, err := cfg.Credentials.Retrieve(ctx)
	if err != nil {
		return fmt.Errorf("retrieve AWS credentials: %w", err)
	}

	hash := sha256.Sum256(body)
	payloadHash := hex.EncodeToString(hash[:])

	return v4.NewSigner().SignHTTP(ctx, creds, req, payloadHash, "lambda", cfg.Region, time.Now())
}

// createProjectRequest mirrors internal/api's createProjectRequest.
// ProposedProject doesn't carry a status, so the commit step fills in a
// sensible default.
type createProjectRequest struct {
	Name        string     `json:"name"`
	Goal        string     `json:"goal"`
	Deadline    *time.Time `json:"deadline,omitempty"`
	Constraints []string   `json:"constraints"`
	Status      string     `json:"status"`
}

const defaultProjectStatus = "active"

func buildCreateProjectRequest(p domain.ProposedProject) createProjectRequest {
	return createProjectRequest{
		Name:        p.Name,
		Goal:        p.Goal,
		Deadline:    p.Deadline,
		Constraints: p.Constraints,
		Status:      defaultProjectStatus,
	}
}

// commit POSTs to {apiURL}/projects — cmd/local's plain HTTP server or the
// deployed API Gateway HTTP API, neither of which require request signing
// (template.yaml doesn't configure auth on NudgeApi).
func commit(ctx context.Context, apiURL string, proposed domain.ProposedProject) (domain.Project, error) {
	body, err := json.Marshal(buildCreateProjectRequest(proposed))
	if err != nil {
		return domain.Project{}, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(apiURL, "/")+"/projects", bytes.NewReader(body))
	if err != nil {
		return domain.Project{}, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return domain.Project{}, fmt.Errorf("call api endpoint: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return domain.Project{}, fmt.Errorf("read response: %w", err)
	}

	return decodeCommitResponse(resp.StatusCode, respBody)
}

func decodeCommitResponse(status int, body []byte) (domain.Project, error) {
	if status != http.StatusCreated {
		return domain.Project{}, fmt.Errorf("api endpoint returned %d: %s", status, body)
	}

	var created domain.Project
	if err := json.Unmarshal(body, &created); err != nil {
		return domain.Project{}, fmt.Errorf("unmarshal response: %w", err)
	}
	return created, nil
}

func printProject(p domain.ProposedProject) {
	fmt.Println("Proposed project:")
	fmt.Println("  Name:       ", p.Name)
	fmt.Println("  Goal:       ", p.Goal)
	if p.Deadline != nil {
		fmt.Println("  Deadline:   ", p.Deadline.Format("2006-01-02"))
	} else {
		fmt.Println("  Deadline:    (none)")
	}
	if len(p.Constraints) == 0 {
		fmt.Println("  Constraints: (none)")
	} else {
		fmt.Println("  Constraints:")
		for _, c := range p.Constraints {
			fmt.Println("   -", c)
		}
	}
}

// confirm prompts on stdout and reads a y/n answer from the controlling
// terminal (/dev/tty) rather than stdin, since stdin may already have been
// consumed reading a piped description.
func confirm(prompt string) bool {
	var reader io.Reader = os.Stdin
	if tty, err := os.Open("/dev/tty"); err == nil {
		defer tty.Close()
		reader = tty
	}

	fmt.Print(prompt)
	scanner := bufio.NewScanner(reader)
	if !scanner.Scan() {
		return false
	}
	return parseConfirmAnswer(scanner.Text())
}

func parseConfirmAnswer(s string) bool {
	s = strings.ToLower(strings.TrimSpace(s))
	return s == "y" || s == "yes"
}
