#!/bin/bash

# Test script for bartowski model ID issue

echo "========================================"
echo "Testing Bartowski Model ID Issue"
echo "========================================"

API_BASE="http://localhost:5800"

# First, search for the model to see what IDs are returned
echo -e "\n1. Searching for Bartowski Mistral models:"
echo "----------------------------------------"
search_result=$(curl -s "${API_BASE}/api/v1/models/search?q=bartowski+Mistral-22B")

echo "$search_result" | python3 -c "
import json, sys
data = json.load(sys.stdin)
if 'models' in data:
    for model in data['models'][:5]:
        print(f\"ID: {model.get('id', 'N/A')}\")
        print(f\"  Name: {model.get('name', 'N/A')}\")
        print(f\"  Quantization: {model.get('quantization', 'N/A')}\")
        print(f\"  File: {model.get('file', 'N/A')}\")
        print(f\"  Repo: {model.get('repo', 'N/A')}\")
        print()
"

# Test different model ID formats
echo -e "\n2. Testing different model ID formats:"
echo "----------------------------------------"

test_model() {
    local model_id="$1"
    echo -e "\nTesting: $model_id"

    response=$(curl -s -X POST "${API_BASE}/v1/chat/completions" \
        -H "Content-Type: application/json" \
        -d "{
            \"model\": \"$model_id\",
            \"messages\": [{\"role\": \"user\", \"content\": \"Say hello\"}],
            \"max_tokens\": 10
        }" 2>&1)

    # Check response
    if echo "$response" | grep -q '"error"'; then
        echo "❌ Failed:"
        echo "$response" | python3 -m json.tool 2>/dev/null | grep -A2 '"error"' | head -3
    elif echo "$response" | grep -q '"choices"'; then
        echo "✅ Success! Model loaded/downloading"
    else
        echo "⚠️  Unexpected response:"
        echo "$response" | head -c 200
    fi
}

# Test various formats
test_model "bartowski/Mistral-22B-v0.1-GGUF:q5_k"
test_model "bartowski/Mistral-22B-v0.1-GGUF:Q5_K"
test_model "bartowski/Mistral-22B-v0.1-GGUF:Q5_K_M"
test_model "bartowski/Mistral-22B-v0.1-GGUF"

# Check what the actual search API returns for this specific model
echo -e "\n3. Detailed search for bartowski/Mistral-22B-v0.1-GGUF:"
echo "----------------------------------------"
curl -s "${API_BASE}/api/models/search/huggingface?modelId=bartowski/Mistral-22B-v0.1-GGUF&includeGated=false" | python3 -c "
import json, sys
try:
    data = json.load(sys.stdin)
    if 'ggufFiles' in data:
        print(f\"Found {len(data['ggufFiles'])} GGUF files:\")
        for file in data['ggufFiles'][:5]:
            print(f\"  - {file.get('filename', 'N/A')} (quant: {file.get('quantization', 'N/A')})\")
            if 'suggestedModelID' in file:
                print(f\"    Suggested ID: {file.get('suggestedModelID')}\")
except:
    print('Failed to parse response')
" 2>/dev/null

echo -e "\n========================================"
echo "Debugging Tips:"
echo "========================================"
echo "1. Check server logs for quantization matching details"
echo "2. The improved code now tries multiple quantization formats:"
echo "   - Exact match (Q5_K)"
echo "   - With suffixes (Q5_K_M, Q5_K_S, Q5_K_L)"
echo "   - Base pattern match (Q5 in any quantization)"
echo "3. If still failing, the model might not exist or use different naming"
echo ""