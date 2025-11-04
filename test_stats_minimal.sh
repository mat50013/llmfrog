#!/bin/bash

# Minimal test for activity stats persistence
echo "🧪 Minimal Activity Stats Persistence Test"
echo "==========================================="

# Clean up and start fresh
rm -f activity_stats.json

# Start FrogLLM
echo "Starting FrogLLM..."
timeout 30 ./frogllm > /tmp/test_minimal.log 2>&1 &
FROG_PID=$!

# Wait for startup
sleep 5

# Check stats endpoint
echo "1. Initial stats check:"
curl -s http://localhost:5800/api/activity/stats | python3 -m json.tool 2>/dev/null || echo "No stats yet"

# Make a simple request to a model that exists (from previous tests)
echo ""
echo "2. Making a request to existing model..."
curl -s -X POST http://localhost:5800/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "quantfactory-tinyllama-1.1b-chat-v1.0-gguf-q4",
    "messages": [{"role": "user", "content": "Hi"}],
    "max_tokens": 5
  }' | python3 -c "import sys, json; d=json.load(sys.stdin); print('Response:', d.get('choices', [{}])[0].get('message', {}).get('content', 'No response')[:50])" 2>/dev/null || echo "Request failed"

sleep 3

# Check stats after request
echo ""
echo "3. Stats after request:"
STATS=$(curl -s http://localhost:5800/api/activity/stats)
echo "$STATS" | python3 -c "
import sys, json
data = json.load(sys.stdin)
print('  Global tokens:', data.get('global', {}).get('total_tokens', 0))
" 2>/dev/null || echo "  Could not parse stats"

# Check file
echo ""
if [ -f activity_stats.json ]; then
    echo "4. ✅ activity_stats.json exists"
    cat activity_stats.json | python3 -c "
import sys, json
data = json.load(sys.stdin)
print('  File contains:', data.get('global_stats', {}).get('total_tokens', 0), 'tokens')
" 2>/dev/null || echo "  Could not parse file"
else
    echo "4. ❌ activity_stats.json not found"
fi

# Trigger reload
echo ""
echo "5. Triggering config reload..."
touch config.yaml
sleep 5

# Check stats after reload
echo ""
echo "6. Stats after reload:"
STATS_AFTER=$(curl -s http://localhost:5800/api/activity/stats)
echo "$STATS_AFTER" | python3 -c "
import sys, json
data = json.load(sys.stdin)
tokens = data.get('global', {}).get('total_tokens', 0)
print('  Global tokens:', tokens)
if tokens > 0:
    print('  🎉 SUCCESS: Stats persisted!')
else:
    print('  ❌ FAILURE: Stats lost')
" 2>/dev/null || echo "  Could not parse stats"

# Cleanup
kill $FROG_PID 2>/dev/null
echo ""
echo "==========================================="
echo "Test complete. Logs: /tmp/test_minimal.log"