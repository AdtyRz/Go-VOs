ARG GO_VERSION=1.26
ARG ALPINE_VERSION=3.21

FROM golang:${GO_VERSION}-alpine AS build

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/govoting ./cmd/server

FROM alpine:${ALPINE_VERSION}

RUN apk add --no-cache ca-certificates tzdata \
    && addgroup -S app \
    && adduser -S -G app app

WORKDIR /app
COPY --from=build /out/govoting /app/govoting
RUN mkdir -p /data/uploads/candidates /data/uploads/school \
    && chown -R app:app /data

USER app
EXPOSE 8080
ENV APP_ADDR=:8080
ENV UPLOAD_DIR=/data/uploads

ENTRYPOINT ["/app/govoting"]
