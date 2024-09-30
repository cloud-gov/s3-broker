FROM golang
LABEL org.opencontainers.image.authors="ape factory GmbH"

# Set environment variables
ENV CGO_ENABLED 0
ENV GOARCH      amd64
ENV GOARM       5
ENV GOOS        linux

# Build BOSH Registry
RUN go get -a -installsuffix cgo -ldflags '-s' github.com/cloud-gov/s3-broker

# Add files
ADD Dockerfile.final /go/bin/Dockerfile
ADD config-sample.yml /go/bin/config.yml

# Command to run
CMD docker build -t apefactory/s3-broker /go/bin
