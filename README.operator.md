# Weaver (Operator Edition)

Managed AI agents for high-density Docker orchestration.

## Setup

1. **Environment:**
   Copy `.env.example` to `.env` and set your `GEMINI_API_KEY`.
   ```bash
   cp .env.example .env
   ```

2. **Config:**
   Config is located at `config/config.json`. The default setup is optimized for Gemini 3 Flash.

3. **Docker Spawning:**
   This project is designed to be used as a managed service where individual agents are spawned as isolated Docker containers.

   To spawn a new agent:
   ```bash
   docker run --rm \
     -v $(pwd)/workspaces/agent-1:/root/.weaver/workspace \
     -e GEMINI_API_KEY=$GEMINI_API_KEY \
     weaver agent -m "Your task"
   ```

## Development

- `make build`: Build the Go binary.
- `docker compose up weaver-gateway`: Start the master gateway for external channels.

## Architecture

- **Gateway:** Central dispatcher for Telegram/Discord.
- **Agents:** Lightweight, one-shot or persistent Go binaries.
- **Workspaces:** Isolated directory-based memory and state.
