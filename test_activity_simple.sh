#!/bin/bash

# Simple test to verify activity stats persist across config reloads
echo "🧪 Testing Activity Stats Persistence (Simple)"
echo "=============================================="

# Clean up previous stats
echo "Cleaning up previous activity_stats.json..."
rm -f activity_stats.json

# Start FrogLLM with existing config
echo "Starting FrogLLM..."
timeout 60 ./frogllm > /tmp/test_activity.log 2>&1 &
FROG_PID=$!

# Wait for server to start
echo "Waiting for server to start..."
sleep 8

# Check if server is running
if ! curl -s http://localhost:5800/api/models > /dev/null 2>&1; then
    echo "❌ Server failed to start"
    cat /tmp/test_activity.log
    exit 1
fi

# Download a small model for testing
echo "Downloading TinyLlama for testing..."
curl -s -X POST http://localhost:5800/api/models/download \
  -H "Content-Type: application/json" \
  -d '{
    "modelName": "QuantFactory/TinyLlama-1.1B-Chat-v1.0-GGUF",
    "quantization": "q4"
  }'

# Wait for download
echo "Waiting for download to complete..."
sleep 15

# Make a test request to generate activity
echo "Making test request to generate tokens..."
RESPONSE=$(curl -s -X POST http://localhost:5800/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "QuantFactory/TinyLlama-1.1B-Chat-v1.0-GGUF:q4",
    "messages": [{"role": "user", "content": "Count from 1 to 5"}],
    "max_tokens": 30,
    "stream": false
  }')

echo "Response received, checking for tokens..."
echo "$RESPONSE" | python3 -c "import sys, json; data=json.load(sys.stdin); print('Tokens used:', data.get('usage', {}).get('total_tokens', 'No usage data'))" 2>/dev/null || echo "Could not parse response"

sleep 3

# Check activity stats API
echo ""
echo "📊 Checking activity stats BEFORE reload..."
STATS_BEFORE=$(curl -s http://localhost:5800/api/activity/stats)
if [ -n "$STATS_BEFORE" ]; then
    echo "$STATS_BEFORE" | python3 -c "
import sys, json
data = json.load(sys.stdin)
if 'global' in data and data['global']:
    print('  Global total tokens:', data['global'].get('total_tokens', 0))
    print('  Global request count:', data['global'].get('request_count', 0))
if 'stats' in data:
    for model_id, stats in data['stats'].items():
        if model_id != '_global_' and stats:
            print(f'  Model {model_id}: {stats.get(\"total_tokens\", 0)} tokens, {stats.get(\"request_count\", 0)} requests')
" 2>/dev/null || echo "  Could not parse stats"
fi

# Check if activity_stats.json exists
echo ""
if [ -f activity_stats.json ]; then
    echo "✅ activity_stats.json file exists BEFORE reload"
    TOKENS_BEFORE=$(cat activity_stats.json | python3 -c "import sys, json; data=json.load(sys.stdin); print(data.get('global_stats', {}).get('total_tokens', 0))" 2>/dev/null || echo "0")
    echo "  File shows $TOKENS_BEFORE total tokens"
else
    echo "⚠️  activity_stats.json not found yet"
fi

# Trigger config reload by modifying the config file
echo ""
echo "🔄 Triggering config reload..."
# Just touch the config to trigger reload
touch config.yaml

# Wait for reload to complete
sleep 8

# Make another request after reload
echo ""
echo "Making another request AFTER reload..."
RESPONSE2=$(curl -s -X POST http://localhost:5800/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "QuantFactory/TinyLlama-1.1B-Chat-v1.0-GGUF:q4",
    "messages": [{"role": "user", "content": "What is 2+2?"}],
    "max_tokens": 20,
    "stream": false
  }')

echo "$RESPONSE2" | python3 -c "import sys, json; data=json.load(sys.stdin); print('Tokens used:', data.get('usage', {}).get('total_tokens', 'No usage data'))" 2>/dev/null || echo "Could not parse response"

sleep 3

# Check activity stats again
echo ""
echo "📊 Checking activity stats AFTER reload..."
STATS_AFTER=$(curl -s http://localhost:5800/api/activity/stats)
if [ -n "$STATS_AFTER" ]; then
    echo "$STATS_AFTER" | python3 -c "
import sys, json
data = json.load(sys.stdin)
if 'global' in data and data['global']:
    print('  Global total tokens:', data['global'].get('total_tokens', 0))
    print('  Global request count:', data['global'].get('request_count', 0))
if 'stats' in data:
    for model_id, stats in data['stats'].items():
        if model_id != '_global_' and stats:
            print(f'  Model {model_id}: {stats.get(\"total_tokens\", 0)} tokens, {stats.get(\"request_count\", 0)} requests')
" 2>/dev/null || echo "  Could not parse stats"
fi

# Final check of activity_stats.json
echo ""
if [ -f activity_stats.json ]; then
    echo "✅ activity_stats.json file exists AFTER reload"
    TOKENS_AFTER=$(cat activity_stats.json | python3 -c "import sys, json; data=json.load(sys.stdin); print(data.get('global_stats', {}).get('total_tokens', 0))" 2>/dev/null || echo "0")
    echo "  File shows $TOKENS_AFTER total tokens"

    # Check if stats persisted
    if [ "$TOKENS_AFTER" -gt "0" ]; then
        echo ""
        echo "🎉 SUCCESS: Activity stats persisted across config reload!"
    else
        echo ""
        echo "⚠️  Warning: Stats file exists but shows 0 tokens"
    fi
else
    echo "❌ activity_stats.json file not found after test"
fi

# Clean up
echo ""
echo "Stopping FrogLLM..."
kill $FROG_PID 2>/dev/null
wait $FROG_PID 2>/dev/null

echo ""
echo "=============================================="
echo "Test complete. Detailed logs: /tmp/test_activity.log"