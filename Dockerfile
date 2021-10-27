FROM golang:alpine as builder

WORKDIR /go/src/github.com/maesoser/logrecv/
COPY . .
RUN go mod init && CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -ldflags '-s -w -extldflags "-static"' -o s3receiverd .

FROM scratch

COPY --from=builder /go/src/github.com/maesoser/logrecv/logrecv /app/s3receiverd

ENTRYPOINT ["/app/s3receiverd","--aggregate"]
