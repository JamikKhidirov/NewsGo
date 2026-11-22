# syntax=docker/dockerfile:1

# ---------- build ----------
FROM golang:1.26-alpine AS build

WORKDIR /src

RUN apk add --no-cache ca-certificates tzdata

COPY go.mod go.sum ./
RUN go mod download

COPY . .

ARG VERSION=dev
ARG COMMIT=none
ARG BUILD_DATE=unknown

ENV CGO_ENABLED=0
RUN go build -trimpath \
    -ldflags "-s -w \
      -X github.com/gonews/gonews/internal/version.Version=${VERSION} \
      -X github.com/gonews/gonews/internal/version.Commit=${COMMIT} \
      -X github.com/gonews/gonews/internal/version.BuildDate=${BUILD_DATE}" \
    -o /out/gonews ./cmd/server

# ---------- runtime ----------
FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=build /out/gonews /usr/local/bin/gonews
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=build /usr/share/zoneinfo /usr/share/zoneinfo

WORKDIR /data
VOLUME ["/data"]

ENV GONEWS_HTTP_ADDR=:8080 \
    GONEWS_DB_PATH=/data/gonews.db

EXPOSE 8080
USER nonroot:nonroot

ENTRYPOINT ["/usr/local/bin/gonews"]
