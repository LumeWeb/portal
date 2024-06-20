#!/bin/bash

# Define the base command
command="xportal build"

# Check if the XPORTAL_PLUGINS environment variable is set
if [[ -n "$XPORTAL_PLUGINS" ]]; then
  # Split the XPORTAL_PLUGINS value into an array
  IFS=',' read -ra plugins <<< "$XPORTAL_PLUGINS"

  # Dynamically add plugins to the command
  for plugin in "${plugins[@]}"
  do
    command+=" --with $plugin"
  done

  command+=" --replace go.lumeweb.com/portal=$(readlink -f .)"
  command+=" --replace go.lumeweb.com/portal/core=$(readlink -f .)/core"
  command+=" --replace go.lumeweb.com/portal/cmd=$(readlink -f .)/cmd"
  command+=" --replace go.lumeweb.com/portal/service=$(readlink -f .)/service"
  command+=" --replace go.lumeweb.com/portal/db=$(readlink -f .)/db"
  command+=" --replace go.lumeweb.com/portal/config=$(readlink -f .)/config"
fi

# Check if DEV environment variable is set
if [[ -n "$DEV" ]]; then
  export XPORTAL_DEBUG=1
  echo "Running in development mode with XPORTAL_DEBUG=1"
fi

# Execute the dynamically created command
echo "Executing command: $command"
eval "$command"