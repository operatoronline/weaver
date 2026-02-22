#!/bin/bash
# Refresh Vertex AI access token for Weaver
TOKEN=$(gcloud auth print-access-token)
CONFIG="/root/.openclaw/workspace/projects/weaver/config/config.json"

python3 -c "
import json
with open('$CONFIG', 'r') as f:
    cfg = json.load(f)
cfg['providers']['gemini']['api_key'] = '''$TOKEN'''
with open('$CONFIG', 'w') as f:
    json.dump(cfg, f, indent=2)
"

# Restart weaver to pick up new token
systemctl restart weaver
