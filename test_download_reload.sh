#!/bin/bash

# Test script to verify download and config reload works correctly

echo "========================================"
echo "Model Download & Config Reload Test"
echo "========================================"
echo ""

API_BASE="${FROG_URL:-http://localhost:5800}"

# Test function
test_model_persistence() {
    local model_id="$1"
    local test_name="$2"

    echo -e "\n========================================"
    echo "Test: $test_name"
    echo "Model: $model_id"
    echo "========================================"

    # First attempt - should trigger download
    echo -e "\n1. First request (should trigger download):"
    response1=$(curl -s -X POST "${API_BASE}/v1/chat/completions" \
        -H "Content-Type: application/json" \
        -d "{
            \"model\": \"$model_id\",
            \"messages\": [{\"role\": \"user\", \"content\": \"Say 'Hello'\"}],
            \"max_tokens\": 10
        }" 2>&1)

    if echo "$response1" | grep -q '"error"'; then
        error=$(echo "$response1" | python3 -c "import json,sys; print(json.load(sys.stdin).get('error','Unknown')[:200])" 2>/dev/null)
        echo "Response: $error"

        # If it says "downloaded but not found", wait and retry
        if echo "$error" | grep -q "downloaded but still not found"; then
            echo "⚠️  Model downloaded but config not updated properly"
            echo "Waiting 5 seconds for config to settle..."
            sleep 5
        else
            echo "❌ Download/load failed: $error"
            return 1
        fi
    elif echo "$response1" | grep -q '"choices"'; then
        echo "✅ First request successful (model was already available)"
    else
        echo "⚠️  Model might be downloading..."
        sleep 10  # Give time for download
    fi

    # Second attempt - should use the downloaded model
    echo -e "\n2. Second request (should use downloaded model):"
    response2=$(curl -s -X POST "${API_BASE}/v1/chat/completions" \
        -H "Content-Type: application/json" \
        -d "{
            \"model\": \"$model_id\",
            \"messages\": [{\"role\": \"user\", \"content\": \"Count to 3\"}],
            \"max_tokens\": 20
        }" 2>&1)

    if echo "$response2" | grep -q '"error"'; then
        error=$(echo "$response2" | python3 -c "import json,sys; print(json.load(sys.stdin).get('error','Unknown'))" 2>/dev/null)
        echo "❌ Still failing: $error"
        return 1
    elif echo "$response2" | grep -q '"choices"'; then
        echo "✅ Second request successful!"
        content=$(echo "$response2" | python3 -c "import json,sys; d=json.load(sys.stdin); print(d['choices'][0]['message']['content'][:100])" 2>/dev/null)
        if [ ! -z "$content" ]; then
            echo "Model responded: $content"
        fi
        return 0
    else
        echo "⚠️  Unexpected response"
        return 1
    fi
}

# Check config before tests
echo "Checking initial config.yaml..."
if [ -f config.yaml ]; then
    model_count=$(grep -c "^  [a-zA-Z]" config.yaml | head -1)
    echo "Found approximately $model_count models in config"
else
    echo "No config.yaml found"
fi

# Test different model formats
echo -e "\n========================================"
echo "Testing Various Model Formats"
echo "========================================"

# Test 1: Small model with specific file
test_model_persistence \
    "Qwen/Qwen2.5-0.5B-Instruct-GGUF:qwen2.5-0.5b-instruct-q4_k_m.gguf" \
    "Specific file format"

# Test 2: Model with quantization
test_model_persistence \
    "TinyLlama/TinyLlama-1.1B-Chat-v1.0-GGUF:q4_k_m" \
    "Quantization format"

# Test 3: The problematic bartowski model
test_model_persistence \
    "bartowski/Mistral-22B-v0.1-GGUF:q5_k" \
    "Bartowski Mistral with Q5_K"

# Check config after tests
echo -e "\n========================================"
echo "Checking config.yaml after tests..."
echo "========================================"

if [ -f config.yaml ]; then
    echo "Recently added models (last 10 lines of models section):"
    awk '/^models:/{flag=1; next} /^[^ ]/{flag=0} flag' config.yaml | tail -20

    echo -e "\nModel count after tests:"
    model_count=$(grep -c "^  [a-zA-Z]" config.yaml | head -1)
    echo "Now have approximately $model_count models in config"
fi

# Check download directory
echo -e "\n========================================"
echo "Checking downloaded models..."
echo "========================================"

if [ -d models ]; then
    echo "Models directory contents:"
    find models -name "*.gguf" -type f 2>/dev/null | while read -r file; do
        size=$(ls -lh "$file" | awk '{print $5}')
        echo "  - $file ($size)"
    done
else
    echo "No models directory found"
fi

echo -e "\n========================================"
echo "Test Complete!"
echo "========================================"
echo ""
echo "Summary:"
echo "1. Models should download on first use"
echo "2. Config should be updated with downloaded models"
echo "3. Second request should use the already downloaded model"
echo "4. No re-download should occur for existing models"
echo ""
echo "Check server logs for detailed download and config update info."
echo ""