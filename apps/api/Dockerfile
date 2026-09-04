# Multi-stage build for the CaptchaFlow API service.
FROM golang:1.25-alpine AS build
WORKDIR /src
ENV GOPROXY=https://goproxy.cn,direct
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/api ./cmd/api

FROM alpine:3.20
RUN adduser -D -u 10001 captchaflow
USER captchaflow
COPY --from=build /out/api /usr/local/bin/api
EXPOSE 8000
ENTRYPOINT ["api"]
