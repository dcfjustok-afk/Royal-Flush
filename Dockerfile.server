FROM golang:1.27-alpine AS build

WORKDIR /src
COPY services/server/go.mod services/server/go.sum ./
RUN go mod download
COPY services/server/ ./
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/royal-flush ./cmd/server

FROM alpine:3.22

RUN apk add --no-cache ca-certificates tzdata \
  && addgroup -S royalflush \
  && adduser -S -G royalflush royalflush
COPY --from=build /out/royal-flush /usr/local/bin/royal-flush

USER royalflush
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/royal-flush"]
