# syntax=docker/dockerfile:1

# ---------- build ----------
FROM golang:1.26-alpine AS build

WORKDIR /src

RUN apk add --no-cache ca-certificates tzdata git

COPY go.mod go.sum ./
RUN go mod download

COPY . .

ARG VERSION=dev
ARG COMMIT=none
ARG BUILD_DATE=unknown

ENV CGO_ENABLED=0 GOOS=linux
RUN go build -trimpath \
    -ldflags "-s -w \
      -X github.com/gonews/gonews/internal/version.Version=${VERSION} \
      -X github.com/gonews/gonews/internal/version.Commit=${COMMIT} \
      -X github.com/gonews/gonews/internal/version.BuildDate=${BUILD_DATE}" \
    -o /out/gonews ./cmd/server

# ---------- runtime ----------
FROM gcr.io/distroless/static-debian12:nonroot

LABEL org.opencontainers.image.title="GoNews" \
      org.opencontainers.image.description="Russian-first news aggregation API" \
      org.opencontainers.image.source="https://github.com/JamikKhidirov/NewsGo" \
      org.opencontainers.image.licenses="MIT"

COPY --from=build /out/gonews /usr/local/bin/gonews
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=build /usr/share/zoneinfo /usr/share/zoneinfo

ENV GONEWS_HTTP_ADDR=:8080 \
    GONEWS_ENV=production \
    GONEWS_LOG_LEVEL=info

EXPOSE 8080
USER nonroot:nonroot

HEALTHCHECK --interval=15s --timeout=3s --start-period=20s --retries=5 \
    CMD ["/usr/local/bin/gonews", "healthcheck"]

ENTRYPOINT ["/usr/local/bin/gonews"]
