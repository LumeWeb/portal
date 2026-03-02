#!/bin/bash

set -e

# Docker image for portal builder
BUILDER_IMAGE="ghcr.io/lumeweb/portal-builder:ubuntu"

# Get the absolute path of the project root
PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Check if DEV environment variable is set
if [[ -n "$DEV" ]]; then
  export XPORTAL_DEBUG=1
  echo "Running in development mode with XPORTAL_DEBUG=1"
fi

# Create dist directory if it doesn't exist
mkdir -p "${PROJECT_ROOT}/dist"

# Get current git commit hash if available
CURRENT_COMMIT=""
if git rev-parse --git-dir > /dev/null 2>&1; then
  CURRENT_COMMIT=$(git rev-parse HEAD)
  echo "Current commit: ${CURRENT_COMMIT}"
fi

# Use current commit as default portal version, or 'develop' if not available
PORTAL_VERSION="${CURRENT_COMMIT:-develop}"

# Create plugin manifest
echo "Creating portal-plugins.yaml manifest..."
if [[ -n "$XPORTAL_PLUGINS" ]]; then
  # Start with portal version
  yq -n ".portalVersion = \"${PORTAL_VERSION}\"" > "${PROJECT_ROOT}/portal-plugins.yaml"
  
  # Parse plugins and add to manifest
  IFS=',' read -ra plugins <<< "$XPORTAL_PLUGINS"
  for plugin_entry in "${plugins[@]}"; do
    # Check if plugin has @ syntax for git hash
    if [[ "$plugin_entry" == *@* ]]; then
      plugin_name="${plugin_entry%@*}"
      plugin_hash="${plugin_entry#*@}"
      yq -i ".plugins[\"$plugin_name\"] = \"$plugin_hash\"" "${PROJECT_ROOT}/portal-plugins.yaml"
    else
      # Use current commit for plugin if available
      if [[ -n "$CURRENT_COMMIT" ]]; then
        yq -i ".plugins[\"$plugin_entry\"] = \"$CURRENT_COMMIT\"" "${PROJECT_ROOT}/portal-plugins.yaml"
      else
        # No hash specified and no git repo available
        yq -i ".plugins += [\"$plugin_entry\"]" "${PROJECT_ROOT}/portal-plugins.yaml"
      fi
    fi
  done
else
  # Create manifest with portal version only (no plugins)
  yq -n ".portalVersion = \"${PORTAL_VERSION}\"" > "${PROJECT_ROOT}/portal-plugins.yaml"
fi

echo "Created portal-plugins.yaml:"
cat "${PROJECT_ROOT}/portal-plugins.yaml"

# Run build in Docker container
echo "Building portal using Docker container..."
docker run --rm \
  -v "${PROJECT_ROOT}:/workspace" \
  -v "${PROJECT_ROOT}/dist:/dist" \
  -e XPORTAL_DEBUG="${XPORTAL_DEBUG:-0}" \
  "${BUILDER_IMAGE}" \
  build-portal

echo "Build completed successfully!"
