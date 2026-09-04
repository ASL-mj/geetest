# Multi-stage build for the CaptchaFlow platform. Build context is the
# repository root: the web console is compiled first and embedded into the
# API binary, so one container serves both the SPA and the JSON API.
FROM node:22-alpine AS web
WORKDIR /web
RUN corepack enable
COPY apps/web/package.json apps/web/pnpm-lock.yaml ./
RUN pnpm install --frozen-lockfile
COPY apps/web/ ./
RUN pnpm build

FROM golang:1.25-alpine AS build
WORKDIR /src
ENV GOPROXY=https://goproxy.cn,direct
COPY apps/api/go.mod apps/api/go.sum ./
RUN go mod download
COPY apps/api/ ./
COPY --from=web /web/dist/ ./internal/httpapi/web/
RUN CGO_ENABLED=0 go build -o /out/api ./cmd/api

FROM alpine:3.20
RUN adduser -D -u 10001 captchaflow
USER captchaflow
COPY --from=build /out/api /usr/local/bin/api
EXPOSE 8000
ENTRYPOINT ["api"]
