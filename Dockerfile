# Use the bufbuild/buf image for extracting the buf command binary
FROM bufbuild/buf:latest as buf
# Use the official Node.js image as the base image for building the api/account/portal
FROM node:20-alpine as nodejs-builder

# Set the working directory
WORKDIR /portal

# Clone the repository with submodules
RUN apk add --no-cache git \
    && git clone --recurse-submodules https://git.lumeweb.com/LumeWeb/portal.git -b develop .

# Set the working directory
WORKDIR /portal/api/account/app

# Build the dashboard
RUN npm ci && npm run build

# Use the official Go image as the base image for the final Go build
FROM golang:1.21.6-alpine as go-builder

# Set the working directory
WORKDIR /portal

# Build the Go application with configurable tags
ARG BUILD_TAGS
RUN apk add --no-cache git && git clone --recurse-submodules https://git.lumeweb.com/LumeWeb/portal.git -b develop .

# Copy the built dashboard from the nodejs-builder stage
COPY --from=nodejs-builder /portal/api/account/app/build/client /portal/api/account/app/build/client
# Copy buf from the buf stage
COPY --from=buf /usr/local/bin/buf /usr/local/bin/buf

# Install the necessary dependencies
RUN apk add bash gcc curl musl-dev

# Install the necessary dependencies
RUN curl -sSf https://sh.rustup.rs | bash -s -- -y

# Set the necessary environment variables
ENV PATH="/root/.cargo/bin:${PATH}"

## Build the Go application
RUN go mod download

## Build the Go application
RUN go generate ./...

## Build the Go application
RUN go build -tags "${BUILD_TAGS}" -gcflags="all=-N -l" -o portal ./cmd/portal

# Use a lightweight base image for the final stage
FROM alpine:latest

# Set the working directory
WORKDIR /portal

# Copy the built binary from the go-builder stage
COPY --from=go-builder /portal/portal .

# Expose the necessary port(s)
EXPOSE 8080

# Run the application
CMD ["./portal"]
