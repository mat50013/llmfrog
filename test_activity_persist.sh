#!/bin/bash

# Test script to verify activity stats persist across config reloads
echo "🧪 Testing Activity Stats Persistence"
echo "======================================"

# Clean up previous test artifacts
rm -f activity_stats.json
rm -f test_config.yaml
rm -f /tmp/test_activity.log

# Create a minimal test config
cat > test_config.yaml << 'EOF'
listen_addr: :5801
log_level: debug
download_dir: downloads
metrics_max_in_memory: 100
models:
  tinyllama-1.1b-chat:
    name: TinyLlama-1.1B-Chat
    desc: TinyLlama 1.1B Chat model
    path: downloads/TinyLlama/TinyLlama-1.1B-Chat-v1.0/tinyllama-1.1b-chat-v1.0.Q4_K_M.gguf
    cmd: /home/matei/claude-test/ClaraCore-main/binaries/llama-server-linux-x64-cuda11-v3.10.1 -m downloads/TinyLlama/TinyLlama-1.1B-Chat-v1.0/tinyllama-1.1b-chat-v1.0.Q4_K_M.gguf --port ${PORT} --host 0.0.0.0 -ngl 999
process_groups:
  - name: default
    models:
      - tinyllama-1.1b-chat
EOF

# Start FrogLLM
echo "Starting FrogLLM with test config..."
./frogllm --config test_config.yaml > /tmp/test_activity.log 2>&1 &
FROG_PID=$!

# Wait for server to start
echo "Waiting for server to start..."
sleep 5

# Make a test request to generate activity
echo "Making test request to generate tokens..."
curl -s -X POST http://localhost:5801/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "tinyllama-1.1b-chat",
    "messages": [{"role": "user", "content": "Count to 5"}],
    "max_tokens": 30
  }' > /dev/null

sleep 2

# Check activity stats API
echo "Checking activity stats via API..."
STATS_BEFORE=$(curl -s http://localhost:5801/api/metrics/activity)
echo "Stats before reload: $(echo $STATS_BEFORE | python3 -c "import sys, json; data=json.load(sys.stdin); print('Total tokens:', data.get('global', {}).get('total_tokens', 0) if data.get('global') else 0)")"

# Trigger config reload by touching the config file
echo "Triggering config reload..."
touch test_config.yaml

# Wait for reload
sleep 5

# Make another request
echo "Making another request after reload..."
curl -s -X POST http://localhost:5801/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "tinyllama-1.1b-chat",
    "messages": [{"role": "user", "content": "Count to 3"}],
    "max_tokens": 20
  }' > /dev/null

sleep 2

# Check activity stats again
echo "Checking activity stats after reload..."
STATS_AFTER=$(curl -s http://localhost:5801/api/metrics/activity)
echo "Stats after reload: $(echo $STATS_AFTER | python3 -c "import sys, json; data=json.load(sys.stdin); print('Total tokens:', data.get('global', {}).get('total_tokens', 0) if data.get('global') else 0)")"

# Check if activity_stats.json exists
if [ -f activity_stats.json ]; then
    echo "✅ activity_stats.json file exists"
    echo "File contents:"
    python3 -m json.tool activity_stats.json | head -20
else
    echo "❌ activity_stats.json file not found"
fi

# Clean up
echo "Stopping FrogLLM..."
kill $FROG_PID 2>/dev/null
wait $FROG_PID 2>/dev/null

echo "======================================"
echo "Test complete. Check /tmp/test_activity.log for detailed logs"