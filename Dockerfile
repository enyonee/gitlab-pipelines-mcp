FROM golang:1.23-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY main.go .
RUN CGO_ENABLED=0 go build -o /gitlab-pipelines-mcp .

FROM alpine:3.20
LABEL org.opencontainers.image.source="https://github.com/enyonee/gitlab-pipelines-mcp"
COPY --from=builder /gitlab-pipelines-mcp /usr/local/bin/
ENV MCP_TRANSPORT=streamable-http
ENV MCP_HOST=0.0.0.0
ENV MCP_PORT=8000
EXPOSE 8000
HEALTHCHECK --interval=30s --timeout=3s CMD ["/usr/local/bin/gitlab-pipelines-mcp", "healthcheck"]
ENTRYPOINT ["/usr/local/bin/gitlab-pipelines-mcp"]
