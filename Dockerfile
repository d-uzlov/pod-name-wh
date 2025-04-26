FROM golang:1.24 AS builder

WORKDIR /app

COPY go.* .
RUN --mount=type=cache,target=/go/pkg/mod go mod download

COPY . .

RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build \
  CGO_ENABLED=0 go build

FROM alpine AS runtime

WORKDIR /
COPY --from=builder /app/daemonset-name-webhook /daemonset-name-webhook

RUN chmod +x /daemonset-name-webhook

CMD ["/daemonset-name-webhook"]
