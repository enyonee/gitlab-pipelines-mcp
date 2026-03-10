# gitlab-pipelines-mcp

MCP server for GitLab CI/CD pipelines.

6 tools: MR pipelines, jobs, logs, retry, cancel.

> For self-hosted GitLab < 17 without native MCP support.

## Quick start

```bash
docker run -d -p 9883:8000 \
  -e GITLAB_PERSONAL_ACCESS_TOKEN=glpat-your-token \
  -e GITLAB_API_URL=https://gitlab.example.com/api/v4 \
  -e GITLAB_DEFAULT_PROJECT_ID=123 \
  ghcr.io/enyonee/gitlab-pipelines-mcp:latest
```

`GITLAB_DEFAULT_PROJECT_ID` is optional - can pass per tool call instead.

## Connect to Claude Code

```json
{
  "mcpServers": {
    "gitlab-pipelines": {
      "url": "http://localhost:9883/mcp"
    }
  }
}
```

## Tools

| Group | Tools |
|-------|-------|
| MR | `mr_pipelines` |
| Pipeline | `pipeline_jobs` `retry_pipeline` `cancel_pipeline` |
| Job | `job_log` `retry_job` |

## License

MIT
