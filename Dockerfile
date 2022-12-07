FROM --platform=linux/amd64 golang:1.19.3-alpine
RUN mkdir /app
WORKDIR /app
COPY go.* ./
RUN go mod download
COPY . .
RUN GO111MODULE=on GOOS=linux go build -o main

CMD ["/app/main"]