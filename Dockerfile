# syntax=docker/dockerfile:1

FROM golang:1.25 AS build
ENV CGO_ENABLED=0 GOTOOLCHAIN=auto
WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/go/pkg/mod \
    go build -trimpath -ldflags="-s -w" -o /bin/linkwatch ./cmd/linkwatch

FROM gcr.io/distroless/static-debian12
COPY --from=build /bin/linkwatch /linkwatch
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/linkwatch"]
