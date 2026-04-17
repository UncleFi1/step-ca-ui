# ── Stage 1: Build ───────────────────────────────────────────────────────────
FROM golang:1.22-alpine AS builder

WORKDIR /build

RUN apk add --no-cache git ca-certificates

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o step-ui .

# ── Stage 2: Runtime ──────────────────────────────────────────────────────────
FROM alpine:3.19

WORKDIR /opt/step-ui

RUN apk add --no-cache curl openssl ca-certificates netcat-openbsd && \
    wget -q "https://github.com/smallstep/cli/releases/download/v0.26.1/step_linux_0.26.1_amd64.tar.gz" \
         -O /tmp/step.tar.gz && \
    tar -xzf /tmp/step.tar.gz -C /tmp && \
    mv /tmp/step_0.26.1/bin/step /usr/local/bin/step && \
    rm -rf /tmp/step* && \
    chmod +x /usr/local/bin/step

COPY --from=builder /build/step-ui .
COPY templates/ templates/
COPY static/ static/

RUN mkdir -p data certs uploads ssl

COPY entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh

EXPOSE 8443

ENTRYPOINT ["/entrypoint.sh"]
