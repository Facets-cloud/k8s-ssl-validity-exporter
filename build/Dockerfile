# Build stage
FROM golang:1.19.3-alpine AS build
WORKDIR /app
COPY go.* ./
RUN go mod download
COPY . .
RUN GO111MODULE=on GOOS=linux go build -o main

# Runtime stage
FROM alpine:latest AS runtime
COPY --from=build /app/main /app/main
CMD ["/app/main"]