# Use the official Golang image as the base image for the builder stage
FROM ghcr.io/lumeweb/xportal as portal-builder

# Override the default shell to use bash
SHELL ["bash", "-c"]

# Set the working directory inside the container
WORKDIR /portal

# Copy the entire current directory to the container
COPY . .

# Copy the .git directory to enable Git operations
COPY ./.git ./.git

# Set default values for build arguments
ARG PLUGINS=""
ARG DEV=false

# Set environment variables based on build arguments
ENV XPORTAL_PLUGINS=${PLUGINS}
ENV DEV=${DEV}

# Build the Go application using the bash script
RUN bash build.sh

# Use a lightweight base image for the final stage
FROM debian:latest AS main

# Update the package lists and install CA certificates
RUN apt-get update && apt-get install -y ca-certificates && apt-get clean

# Set the working directory
WORKDIR /portal

# Copy the built binary from the go-builder stage
COPY --from=portal-builder /portal/portal .

# Expose port 8080 for the application
EXPOSE 8080

# Start the application
CMD ["./portal"]