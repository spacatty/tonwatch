# The Go API, plus import-sqlite for moving a pre-Postgres database in. Both binaries are static.
FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/fragwatch ./cmd/server \
 && CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/import-sqlite ./cmd/import-sqlite

FROM alpine:3.22
RUN apk add --no-cache ca-certificates tzdata \
 && adduser -D -u 10001 app
COPY --from=build /out/fragwatch /out/import-sqlite /app/
USER app
WORKDIR /app
# inside the container the API listens on every interface; compose decides who can reach it
ENV ADDR=0.0.0.0:8080
EXPOSE 8080
ENTRYPOINT ["/app/fragwatch"]
