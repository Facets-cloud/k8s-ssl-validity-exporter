FROM golang:1.19.3-alpine AS build
WORKDIR /app
COPY go.* ./
RUN go mod download
COPY . .
RUN GO111MODULE=on GOOS=linux GOARCH=amd64 go build -o main-amd64
RUN GO111MODULE=on GOOS=linux GOARCH=arm64 go build -o main-arm64

# Runtime stage
FROM alpine:latest AS runtime-amd64
COPY --from=build /app/main-amd64 /app/main
CMD ["/app/main"]

FROM alpine:latest AS runtime-arm64
COPY --from=build /app/main-arm64 /app/main
CMD ["/app/main"]
