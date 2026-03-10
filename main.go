package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"

	"context"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

const defaultTailLines = 150

var (
	apiBase    string
	authToken  string
	defaultPID string
	httpClient = &http.Client{Timeout: 30 * time.Second}

	ansiRE    = regexp.MustCompile(`\x1b\[[0-9;?]*[a-zA-Z]|\x1b\].*?\x07`)
	sectionRE = regexp.MustCompile(`(?m)^section_(start|end):\d+:.*$`)
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "healthcheck" {
		port := envOr("MCP_PORT", "8000")
		resp, err := http.Get("http://localhost:" + port + "/healthz")
		if err != nil || resp.StatusCode != 200 {
			os.Exit(1)
		}
		os.Exit(0)
	}

	if os.Getenv("GITLAB_PERSONAL_ACCESS_TOKEN") == "" {
		log.Fatal("GITLAB_PERSONAL_ACCESS_TOKEN is required")
	}

	apiBase = envOr("GITLAB_API_URL", "https://gitlab.com/api/v4")
	authToken = os.Getenv("GITLAB_PERSONAL_ACCESS_TOKEN")
	defaultPID = os.Getenv("GITLAB_DEFAULT_PROJECT_ID")

	s := server.NewMCPServer("gitlab-pipelines", "1.0.0")
	registerTools(s)

	transport := envOr("MCP_TRANSPORT", "stdio")
	if transport == "stdio" {
		if err := server.ServeStdio(s); err != nil {
			log.Fatal(err)
		}
		return
	}

	port := envOr("MCP_PORT", "8000")
	streamable := server.NewStreamableHTTPServer(s)
	sseServer := server.NewSSEServer(s)

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		fmt.Fprint(w, "ok")
	})
	mux.Handle("/mcp", streamable)
	mux.Handle("/", sseServer)

	log.Printf("gitlab-pipelines-mcp listening on :%s", port)
	log.Fatal(http.ListenAndServe(":"+port, mux))
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// ── GitLab API ──────────────────────────────────────

func glRequest(method, path string) ([]byte, error) {
	req, err := http.NewRequest(method, apiBase+path, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("PRIVATE-TOKEN", authToken)

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case 401:
		return nil, fmt.Errorf("unauthorized - token is invalid or expired")
	case 403:
		return nil, fmt.Errorf("forbidden: %s - check token permissions (api scope)", path)
	case 404:
		return nil, fmt.Errorf("not found: %s", path)
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, path)
	}

	return io.ReadAll(resp.Body)
}

func glJSON(method, path string) (any, error) {
	data, err := glRequest(method, path)
	if err != nil {
		return nil, err
	}
	var result any
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("invalid JSON from %s", path)
	}
	return result, nil
}

func glText(method, path string) (string, error) {
	data, err := glRequest(method, path)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// ── Helpers ─────────────────────────────────────────

func resolveProject(projectID string) (string, error) {
	pid := projectID
	if pid == "" {
		pid = defaultPID
	}
	if pid == "" {
		return "", fmt.Errorf("project_id is required (or set GITLAB_DEFAULT_PROJECT_ID env var)")
	}
	return pid, nil
}

func formatDuration(seconds any) string {
	f, ok := seconds.(float64)
	if !ok || f == 0 {
		return "n/a"
	}
	total := int(f)
	m, s := total/60, total%60
	if m > 0 {
		return fmt.Sprintf("%dm %ds", m, s)
	}
	return fmt.Sprintf("%ds", s)
}

func cleanLog(raw string) string {
	text := ansiRE.ReplaceAllString(raw, "")
	text = sectionRE.ReplaceAllString(text, "")
	text = strings.ReplaceAll(text, "\r\n", "\n")

	var lines []string
	for _, line := range strings.Split(text, "\n") {
		if strings.Contains(line, "\r") {
			parts := strings.Split(line, "\r")
			line = parts[len(parts)-1]
		}
		if strings.TrimSpace(line) != "" {
			lines = append(lines, line)
		}
	}
	return strings.Join(lines, "\n")
}

func str(m map[string]any, key string) string { v, _ := m[key].(string); return v }
func num(m map[string]any, key string) float64 { v, _ := m[key].(float64); return v }

func pipelineSummary(p map[string]any) map[string]any {
	sha := str(p, "sha")
	if len(sha) > 8 {
		sha = sha[:8]
	}
	return map[string]any{
		"id":         num(p, "id"),
		"status":     str(p, "status"),
		"ref":        str(p, "ref"),
		"sha":        sha,
		"source":     str(p, "source"),
		"created_at": str(p, "created_at"),
		"updated_at": str(p, "updated_at"),
		"web_url":    str(p, "web_url"),
		"duration":   formatDuration(p["duration"]),
	}
}

func jobSummary(j map[string]any) map[string]any {
	runnerDesc := ""
	if r, ok := j["runner"].(map[string]any); ok {
		runnerDesc = str(r, "description")
	}
	return map[string]any{
		"id":             num(j, "id"),
		"name":           str(j, "name"),
		"stage":          str(j, "stage"),
		"status":         str(j, "status"),
		"failure_reason": j["failure_reason"],
		"duration":       formatDuration(j["duration"]),
		"started_at":     j["started_at"],
		"finished_at":    j["finished_at"],
		"runner":         runnerDesc,
		"web_url":        str(j, "web_url"),
	}
}

func getFailedJobs(pid string, pipelineID int) []map[string]any {
	raw, err := glJSON("GET", fmt.Sprintf("/projects/%s/pipelines/%d/jobs?per_page=100&scope[]=failed", pid, pipelineID))
	if err != nil {
		return nil
	}
	jobs, ok := raw.([]any)
	if !ok {
		return nil
	}
	var result []map[string]any
	for _, j := range jobs {
		if m, ok := j.(map[string]any); ok {
			result = append(result, jobSummary(m))
		}
	}
	return result
}

// ── MCP helpers ─────────────────────────────────────

func getArgs(r mcp.CallToolRequest) map[string]any {
	m, _ := r.Params.Arguments.(map[string]any)
	if m == nil {
		return map[string]any{}
	}
	return m
}

func sarg(r mcp.CallToolRequest, k string) string { v, _ := getArgs(r)[k].(string); return v }

func iarg(r mcp.CallToolRequest, k string, def int) int {
	v, ok := getArgs(r)[k].(float64)
	if !ok {
		return def
	}
	return int(v)
}

func res(v any) (*mcp.CallToolResult, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	return mcp.NewToolResultText(string(data)), nil
}

func fail(msg string) (*mcp.CallToolResult, error) {
	return mcp.NewToolResultError(msg), nil
}

// ── Tools ───────────────────────────────────────────

func registerTools(s *server.MCPServer) {

	// ── MR → Pipelines ──

	s.AddTool(mcp.NewTool("mr_pipelines",
		mcp.WithDescription("Get all pipelines for a merge request. Start here when investigating MR CI status. Returns pipelines newest-first. Only merge_request_event pipelines shown. The latest failed pipeline auto-expands its failed jobs."),
		mcp.WithNumber("mr_iid", mcp.Required(), mcp.Description("Merge request IID")),
		mcp.WithString("project_id", mcp.Description("GitLab project ID (default: GITLAB_DEFAULT_PROJECT_ID env)")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		mrIID := iarg(req, "mr_iid", 0)
		if mrIID == 0 {
			return fail("mr_iid is required")
		}
		pid, err := resolveProject(sarg(req, "project_id"))
		if err != nil {
			return fail(err.Error())
		}

		raw, err := glJSON("GET", fmt.Sprintf("/projects/%s/merge_requests/%d/pipelines", pid, mrIID))
		if err != nil {
			return fail(err.Error())
		}
		pipelines, _ := raw.([]any)

		var result []map[string]any
		firstFailedExpanded := false
		for _, p := range pipelines {
			pm, ok := p.(map[string]any)
			if !ok {
				continue
			}
			if str(pm, "source") != "merge_request_event" {
				continue
			}
			summary := pipelineSummary(pm)
			if str(pm, "status") == "failed" && !firstFailedExpanded {
				summary["failed_jobs"] = getFailedJobs(pid, int(num(pm, "id")))
				firstFailedExpanded = true
			}
			result = append(result, summary)
		}

		return res(map[string]any{
			"mr_iid":          mrIID,
			"project_id":      pid,
			"pipelines_count": len(result),
			"pipelines":       result,
		})
	})

	// ── Pipeline Jobs ──

	s.AddTool(mcp.NewTool("pipeline_jobs",
		mcp.WithDescription("Get all jobs for a pipeline, grouped by stage. Use after mr_pipelines to drill into a specific pipeline."),
		mcp.WithNumber("pipeline_id", mcp.Required(), mcp.Description("Pipeline ID")),
		mcp.WithString("project_id", mcp.Description("GitLab project ID (default: GITLAB_DEFAULT_PROJECT_ID env)")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		pipelineID := iarg(req, "pipeline_id", 0)
		if pipelineID == 0 {
			return fail("pipeline_id is required")
		}
		pid, err := resolveProject(sarg(req, "project_id"))
		if err != nil {
			return fail(err.Error())
		}

		raw, err := glJSON("GET", fmt.Sprintf("/projects/%s/pipelines/%d/jobs?per_page=100", pid, pipelineID))
		if err != nil {
			return fail(err.Error())
		}
		jobs, _ := raw.([]any)

		byStage := map[string][]map[string]any{}
		var failed []map[string]any
		for _, j := range jobs {
			jm, ok := j.(map[string]any)
			if !ok {
				continue
			}
			summary := jobSummary(jm)
			stage := str(jm, "stage")
			byStage[stage] = append(byStage[stage], summary)
			if str(jm, "status") == "failed" {
				failed = append(failed, summary)
			}
		}

		return res(map[string]any{
			"pipeline_id":  pipelineID,
			"project_id":   pid,
			"total_jobs":   len(jobs),
			"failed_count": len(failed),
			"stages":       byStage,
			"failed_jobs":  failed,
		})
	})

	// ── Job Log ──

	s.AddTool(mcp.NewTool("job_log",
		mcp.WithDescription("Get the log (trace) of a specific job. Returns the last `tail` lines (default 150). ANSI codes and CI section markers stripped. Use after pipeline_jobs to investigate a failed job."),
		mcp.WithNumber("job_id", mcp.Required(), mcp.Description("Job ID")),
		mcp.WithString("project_id", mcp.Description("GitLab project ID (default: GITLAB_DEFAULT_PROJECT_ID env)")),
		mcp.WithNumber("tail", mcp.Description("Number of lines from the end (default 150)")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		jobID := iarg(req, "job_id", 0)
		if jobID == 0 {
			return fail("job_id is required")
		}
		pid, err := resolveProject(sarg(req, "project_id"))
		if err != nil {
			return fail(err.Error())
		}
		tail := iarg(req, "tail", defaultTailLines)

		// Получаем метаданные джобы
		metaRaw, err := glJSON("GET", fmt.Sprintf("/projects/%s/jobs/%d", pid, jobID))
		if err != nil {
			return fail(err.Error())
		}
		meta, _ := metaRaw.(map[string]any)
		job := jobSummary(meta)

		// Получаем лог (может быть недоступен)
		rawLog, err := glText("GET", fmt.Sprintf("/projects/%s/jobs/%d/trace", pid, jobID))
		if err != nil {
			rawLog = ""
		}

		if strings.TrimSpace(rawLog) == "" {
			reason := "unknown"
			if r, ok := job["failure_reason"].(string); ok && r != "" {
				reason = r
			}
			return res(map[string]any{
				"job":             job,
				"log_total_lines": 0,
				"log_truncated":   false,
				"log_tail":        tail,
				"log":             fmt.Sprintf("(no output - job ended with %s)", reason),
			})
		}

		rawLines := strings.Split(rawLog, "\n")
		totalLines := len(rawLines)
		truncated := tail > 0 && totalLines > tail

		// Берем 3x tail перед чисткой
		if truncated {
			start := len(rawLines) - tail*3
			if start < 0 {
				start = 0
			}
			rawLines = rawLines[start:]
		}

		cleaned := cleanLog(strings.Join(rawLines, "\n"))
		lines := strings.Split(cleaned, "\n")
		if tail > 0 && len(lines) > tail {
			lines = lines[len(lines)-tail:]
		}

		return res(map[string]any{
			"job":             job,
			"log_total_lines": totalLines,
			"log_truncated":   truncated,
			"log_tail":        tail,
			"log":             strings.Join(lines, "\n"),
		})
	})

	// ── Retry Pipeline ──

	s.AddTool(mcp.NewTool("retry_pipeline",
		mcp.WithDescription("Retry all failed jobs in a pipeline. Use when failure_reason is runner_system_failure or other infra issues."),
		mcp.WithNumber("pipeline_id", mcp.Required(), mcp.Description("Pipeline ID")),
		mcp.WithString("project_id", mcp.Description("GitLab project ID (default: GITLAB_DEFAULT_PROJECT_ID env)")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		pipelineID := iarg(req, "pipeline_id", 0)
		if pipelineID == 0 {
			return fail("pipeline_id is required")
		}
		pid, err := resolveProject(sarg(req, "project_id"))
		if err != nil {
			return fail(err.Error())
		}

		raw, err := glJSON("POST", fmt.Sprintf("/projects/%s/pipelines/%d/retry", pid, pipelineID))
		if err != nil {
			return fail(err.Error())
		}
		p, _ := raw.(map[string]any)

		return res(map[string]any{
			"action":   "pipeline_retried",
			"pipeline": pipelineSummary(p),
		})
	})

	// ── Retry Job ──

	s.AddTool(mcp.NewTool("retry_job",
		mcp.WithDescription("Retry a single failed job. More targeted than retry_pipeline - use when only one job failed."),
		mcp.WithNumber("job_id", mcp.Required(), mcp.Description("Job ID")),
		mcp.WithString("project_id", mcp.Description("GitLab project ID (default: GITLAB_DEFAULT_PROJECT_ID env)")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		jobID := iarg(req, "job_id", 0)
		if jobID == 0 {
			return fail("job_id is required")
		}
		pid, err := resolveProject(sarg(req, "project_id"))
		if err != nil {
			return fail(err.Error())
		}

		raw, err := glJSON("POST", fmt.Sprintf("/projects/%s/jobs/%d/retry", pid, jobID))
		if err != nil {
			return fail(err.Error())
		}
		j, _ := raw.(map[string]any)

		return res(map[string]any{
			"action": "job_retried",
			"job":    jobSummary(j),
		})
	})

	// ── Cancel Pipeline ──

	s.AddTool(mcp.NewTool("cancel_pipeline",
		mcp.WithDescription("Cancel a running pipeline."),
		mcp.WithNumber("pipeline_id", mcp.Required(), mcp.Description("Pipeline ID")),
		mcp.WithString("project_id", mcp.Description("GitLab project ID (default: GITLAB_DEFAULT_PROJECT_ID env)")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		pipelineID := iarg(req, "pipeline_id", 0)
		if pipelineID == 0 {
			return fail("pipeline_id is required")
		}
		pid, err := resolveProject(sarg(req, "project_id"))
		if err != nil {
			return fail(err.Error())
		}

		raw, err := glJSON("POST", fmt.Sprintf("/projects/%s/pipelines/%d/cancel", pid, pipelineID))
		if err != nil {
			return fail(err.Error())
		}
		p, _ := raw.(map[string]any)

		return res(map[string]any{
			"action":   "pipeline_cancelled",
			"pipeline": pipelineSummary(p),
		})
	})
}
