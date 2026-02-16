#!/bin/bash

# Simple script to spawn a new PicoClaw agent in a Docker container
# Usage: ./spawn-agent.sh <agent_id> "<task_message>"

AGENT_ID=$1
TASK=$2

if [ -z "$AGENT_ID" ] || [ -z "$TASK" ]; then
  echo "Usage: ./spawn-agent.sh <agent_id> \"<task_message>\""
  exit 1
fi

# Ensure workspace directory exists
mkdir -p workspaces/$AGENT_ID

# Run agent in Docker
docker run --rm \
  -v $(pwd)/workspaces/$AGENT_ID:/root/.picoclaw/workspace \
  -v $(pwd)/config/config.json:/root/.picoclaw/config.json:ro \
  -e GEMINI_API_KEY=$GEMINI_API_KEY \
  picoclaw agent -m "$TASK"
