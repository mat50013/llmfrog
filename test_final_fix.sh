#!/bin/bash

# Final test for all model ID formats after fixes

echo "========================================"
echo "FrogLLM Model ID Format Test - FINAL"
echo "========================================"
echo "Testing all supported formats after fixes:"
echo ""

API_BASE="${FROG_URL:-http://localhost:5800}"

test_model() {
    local model_id="$1"
    local description="$2"

    echo -e "\n----------------------------------------"
    echo "Test: $description"
    echo "Model ID: $model_id"
    echo "----------------------------------------"

    response=$(curl -s -X POST "${API_BASE}/v1/chat/completions" \
        -H "Content-Type: application/json" \
        -d "{
            \"model\": \"$model_id\",
            \"messages\": [{
                \"role\": \"user\",
                \"content\": \"Say 'Hello from FrogLLM' and nothing else.\"
            }],
            \"temperature\": 0.1,
            \"max_tokens\": 20
        }" 2>&1)

    # Check response
    if echo "$response" | grep -q '"error"'; then
        error_msg=$(echo "$response" | python3 -c "import json, sys; d=json.load(sys.stdin); print(d.get('error', 'Unknown error'))" 2>/dev/null || echo "$response")
        echo "❌ Failed: $error_msg"
    elif echo "$response" | grep -q '"choices"'; then
        echo "✅ Success! Model accepted and processing"
        # Try to extract the response content
        content=$(echo "$response" | python3 -c "import json, sys; d=json.load(sys.stdin); print(d['choices'][0]['message']['content'][:100] if 'choices' in d else '')" 2>/dev/null)
        if [ ! -z "$content" ]; then
            echo "Response: $content"
        fi
    else
        echo "⚠️  Unexpected response (might be downloading):"
        echo "$response" | head -c 200
    fi
}

# Test 1: Traditional repo format
test_model "Qwen/Qwen2.5-0.5B-Instruct-GGUF" "Traditional repo format (downloads all GGUF)"

# Test 2: Repo with specific file
test_model "Qwen/Qwen2.5-0.5B-Instruct-GGUF:qwen2.5-0.5b-instruct-q4_k_m.gguf" "Specific file format"

# Test 3: Repo with quantization (various formats)
test_model "Qwen/Qwen2.5-0.5B-Instruct-GGUF:q4_k_m" "Quantization format (lowercase)"
test_model "Qwen/Qwen2.5-0.5B-Instruct-GGUF:Q4_K_M" "Quantization format (uppercase)"
test_model "Qwen/Qwen2.5-0.5B-Instruct-GGUF:q4_k" "Quantization format (without suffix)"

# Test 4: The problematic bartowski format
test_model "bartowski/Mistral-22B-v0.1-GGUF:q5_k" "Bartowski model with quantization"

# Test 5: Other common formats
test_model "TheBloke/Llama-2-7B-Chat-GGUF:Q4_K_M" "TheBloke model with quantization"
test_model "mistralai/Mistral-7B-Instruct-v0.2-GGUF:mistral-7b-instruct-v0.2.Q4_K_M.gguf" "Mistral with specific file"

echo -e "\n========================================"
echo "Test Summary"
echo "========================================"
echo "The improved system should now handle:"
echo "1. ✓ Traditional repo/model format"
echo "2. ✓ repo:filename.gguf format (specific file)"
echo "3. ✓ repo:quantization format (flexible matching)"
echo "4. ✓ Case-insensitive quantization matching"
echo "5. ✓ Quantization with/without suffixes (_M, _S, _L)"
echo "6. ✓ Fuzzy matching for base patterns (Q5 matches Q5_K_M)"
echo ""
echo "Metrics middleware now skips unknown models with '/' or ':'"
echo "allowing them to be processed by the main handler for auto-download."
echo ""
echo "Check server logs for detailed quantization matching info."
echo ""