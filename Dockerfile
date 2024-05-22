FROM ubuntu:jammy as go-builder

SHELL ["bash", "-c"]
ENV TERM xterm
ENV GO_VERSION 1.22.1
ENV NODE_VERSION stable

RUN apt-get update && apt-get install -y git curl bsdmainutils make bison gcc

WORKDIR /portal

COPY . .
COPY ./.git ./.git

RUN git submodule update --init --recursive

COPY build.sh .
RUN chmod +x build.sh && source build.sh && deps && make

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