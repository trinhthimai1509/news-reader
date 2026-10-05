FROM golang:1.27-alpine3.24 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download && go mod verify
COPY . .
RUN go vet ./... && go test ./... && CGO_ENABLED=0 go build -buildvcs=false -trimpath -ldflags='-s -w' -o /server ./cmd/server

FROM alpine:3.24
RUN apk add --no-cache ca-certificates tzdata && adduser -D -H -u 10001 app
WORKDIR /app
COPY --from=build /server /app/server
COPY web /app/web
COPY migrations /app/migrations
USER 10001:10001
ENV LISTEN_ADDR=:8080
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=3s --start-period=20s CMD wget -q -O /dev/null http://127.0.0.1:8080/healthz || exit 1
CMD ["/app/server"]
