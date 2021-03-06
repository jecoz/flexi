# SPDX-FileCopyrightText: 2020 jecoz
#
# SPDX-License-Identifier: BSD-3-Clause

FROM golang:alpine AS builder

# Set necessary environmet variables needed for our image
ENV GO111MODULE=on CGO_ENABLED=0

# Move to working directory /build
WORKDIR /build

# Copy and download dependency using go mod
COPY go.mod .
COPY go.sum .
RUN go mod download

# Copy the code into the container
COPY . .

# Build the application
RUN go build -o main examples/echo64/main.go

# Move to /dist directory as the place for resulting binary folder
WORKDIR /dist

# Copy binary from build to main folder
RUN cp /build/main .

# Build a small image
FROM scratch

COPY --from=builder /dist/main /

EXPOSE 564

# Command to run
ENTRYPOINT ["/main", "-port", "564"]
