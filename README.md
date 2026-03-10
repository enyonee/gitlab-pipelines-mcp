# GitLab Pipelines MCP

MCP server for GitLab CI/CD pipelines. Written in Go, using [mcp-go](https://github.com/mark3labs/mcp-go).

> **For self-hosted GitLab without built-in MCP support.** GitLab 17.x+ has native MCP - use that instead. This server is for older GitLab versions (< 17) that don't have MCP integration.

## Tools

| Tool | Description |
|------|-------------|
| `mr_pipelines` | Get all pipelines for a merge request. Failed pipelines auto-expand with failed jobs. |
| `pipeline_jobs` | Get all jobs for a pipeline, grouped by stage. |
| `job_log` | Get job log (last 150 lines by default). |
| `retry_pipeline` | Retry all failed jobs in a pipeline. |
| `retry_job` | Retry a single job. |
| `cancel_pipeline` | Cancel a running pipeline. |

## Docker (recommended)

```bash
docker run -d --name gitlab-pipelines-mcp \
  -e GITLAB_PERSONAL_ACCESS_TOKEN=glpat-your-token \
  -e GITLAB_API_URL=https://gitlab.example.com/api/v4 \
  -e GITLAB_DEFAULT_PROJECT_ID=123 \
  -p 9883:8000 \
  ghcr.io/enyonee/gitlab-pipelines-mcp:latest
```

Then add to `.mcp.json`:

```json
{
  "mcpServers": {
    "gitlab-pipelines": {
      "type": "http",
      "url": "http://localhost:9883/mcp"
    }
  }
}
```

### Docker Compose

```yaml
gitlab-pipelines:
  image: ghcr.io/enyonee/gitlab-pipelines-mcp:latest
  restart: unless-stopped
  ports:
    - "9883:8000"
  environment:
    - GITLAB_PERSONAL_ACCESS_TOKEN=${GITLAB_PERSONAL_ACCESS_TOKEN}
    - GITLAB_API_URL=https://gitlab.example.com/api/v4
    - GITLAB_DEFAULT_PROJECT_ID=123
```

## stdio

```bash
claude mcp add gitlab-pipelines \
  -e GITLAB_PERSONAL_ACCESS_TOKEN=glpat-your-token \
  -e GITLAB_API_URL=https://gitlab.example.com/api/v4 \
  -e GITLAB_DEFAULT_PROJECT_ID=123 \
  -- /path/to/gitlab-pipelines-mcp
```

## Build

```bash
go build -o gitlab-pipelines-mcp .
```

## Environment variables

| Variable | Required | Description |
|----------|----------|-------------|
| `GITLAB_PERSONAL_ACCESS_TOKEN` | yes | GitLab PAT with `api` scope |
| `GITLAB_API_URL` | no | GitLab API URL (default: `https://gitlab.com/api/v4`) |
| `GITLAB_DEFAULT_PROJECT_ID` | no | Default project ID (can pass per call) |
| `MCP_TRANSPORT` | no | Transport: `stdio` (default) or `streamable-http` |
| `MCP_PORT` | no | Bind port (default: `8000`) |

## Usage flow

```
mr_pipelines(mr_iid=975)
  -> see failed pipeline #48073 with failed job run-tests

job_log(job_id=284589)
  -> read last 150 lines of the failed job

retry_pipeline(pipeline_id=48073)
  -> retry all failed jobs
```
