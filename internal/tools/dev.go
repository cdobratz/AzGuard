package tools

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/agent/agent/internal/llm"
)

type CodeGenerator struct {
	provider llm.Provider
}

func NewCodeGenerator(provider llm.Provider) *CodeGenerator {
	return &CodeGenerator{provider: provider}
}

type GenerateRequest struct {
	Language string
	Task     string
	FileName string
	Path     string
}

func (g *CodeGenerator) Generate(req GenerateRequest) (string, error) {
	prompt := fmt.Sprintf(`Generate %s code for the following task:
%s

Only output the code, no explanations.`, req.Language, req.Task)

	messages := []llm.Message{
		{Role: "user", Content: prompt},
	}

	code, err := g.provider.Chat(messages)
	if err != nil {
		return "", err
	}

	if req.Path != "" {
		dir := filepath.Dir(req.Path)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return "", err
		}

		codeContent := extractCode(code, req.Language)
		if err := os.WriteFile(req.Path, []byte(codeContent), 0644); err != nil {
			return "", err
		}
		fmt.Printf("Code written to %s\n", req.Path)
	}

	return code, nil
}

func extractCode(response, language string) string {
	lines := strings.Split(response, "\n")
	var codeLines []string
	inCode := false

	for _, line := range lines {
		if strings.HasPrefix(line, "```") {
			inCode = !inCode
			continue
		}
		if inCode {
			codeLines = append(codeLines, line)
		}
	}

	if len(codeLines) > 0 {
		return strings.Join(codeLines, "\n")
	}
	return response
}

type CodeReviewer struct {
	provider llm.Provider
}

func NewCodeReviewer(provider llm.Provider) *CodeReviewer {
	return &CodeReviewer{provider: provider}
}

type ReviewRequest struct {
	Path     string
	Language string
}

type ReviewResult struct {
	Issues     []string
	Score      int
	Summary    string
	Complexity string
}

func (r *CodeReviewer) Review(req ReviewRequest) (*ReviewResult, error) {
	content, err := os.ReadFile(req.Path)
	if err != nil {
		return nil, err
	}

	language := req.Language
	if language == "" {
		language = detectLanguage(req.Path)
	}

	prompt := fmt.Sprintf(`Review the following %s code and provide:
1. A brief summary (1-2 sentences)
2. Code quality issues (list up to 5)
3. Complexity assessment (low/medium/high)
4. Overall score (1-10)

Code:
%s`, language, string(content))

	messages := []llm.Message{
		{Role: "user", Content: prompt},
	}

	review, err := r.provider.Chat(messages)
	if err != nil {
		return nil, err
	}

	result := &ReviewResult{
		Issues:  extractIssues(review),
		Summary: extractSummary(review),
	}

	return result, nil
}

func detectLanguage(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".go":
		return "Go"
	case ".js", ".jsx":
		return "JavaScript"
	case ".ts", ".tsx":
		return "TypeScript"
	case ".py":
		return "Python"
	case ".java":
		return "Java"
	case ".cs":
		return "C#"
	case ".rb":
		return "Ruby"
	case ".rs":
		return "Rust"
	case ".cpp", ".cc":
		return "C++"
	case ".c":
		return "C"
	default:
		return "Unknown"
	}
}

func extractIssues(review string) []string {
	var issues []string
	lines := strings.Split(review, "\n")
	inIssues := false

	for _, line := range lines {
		lower := strings.ToLower(line)
		if strings.Contains(lower, "issue") || strings.Contains(lower, "problem") || strings.Contains(lower, "recommend") {
			inIssues = true
		}
		if inIssues && len(strings.TrimSpace(line)) > 0 {
			issues = append(issues, strings.TrimSpace(line))
		}
	}

	if len(issues) > 5 {
		issues = issues[:5]
	}

	return issues
}

func extractSummary(review string) string {
	lines := strings.Split(review, "\n")
	if len(lines) > 0 {
		return strings.TrimSpace(lines[0])
	}
	return review
}

type TestRunner struct{}

func NewTestRunner() *TestRunner {
	return &TestRunner{}
}

type TestResult struct {
	Passed  bool
	Output  string
	Summary string
}

func (t *TestRunner) Run(path string) (*TestResult, error) {
	ext := strings.ToLower(filepath.Ext(path))
	dir := filepath.Dir(path)

	var cmd *exec.Cmd
	switch ext {
	case ".py":
		cmd = exec.Command("python3", "-m", "pytest", "-v", path)
	case ".js", ".jsx":
		cmd = exec.Command("npm", "test", "--", "--passWithNoTests")
		if _, err := os.Stat(filepath.Join(dir, "package.json")); err == nil {
			cmd.Dir = dir
		} else {
			cmd = exec.Command("node", path)
		}
	case ".ts", ".tsx":
		cmd = exec.Command("npx", "jest", path)
	case ".go":
		cmd = exec.Command("go", "test", "-v", "./...")
		wd, _ := os.Getwd()
		cmd.Dir = wd
	case ".rs":
		cmd = exec.Command("cargo", "test")
		cmd.Dir = dir
	case ".java":
		cmd = exec.Command("mvn", "test")
		cmd.Dir = dir
	default:
		return &TestResult{
			Passed:  false,
			Output:  "",
			Summary: "Unsupported test framework for " + ext,
		}, nil
	}

	output, err := cmd.CombinedOutput()

	result := &TestResult{
		Output:  string(output),
		Passed:  err == nil,
	}

	if err != nil {
		result.Summary = fmt.Sprintf("Tests failed: %v", err)
	} else {
		result.Summary = "All tests passed"
	}

	return result, nil
}
