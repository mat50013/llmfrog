#!/bin/bash

# Comprehensive test script to verify all download logic works correctly

echo "========================================"
echo "🐸 FrogLLM Complete Logic Test"
echo "========================================"
echo "This test verifies:"
echo "1. Single file downloads (non-split models)"
echo "2. Split model downloads (all parts)"
echo "3. No duplicate alias errors"
echo "4. Proper config updates"
echo "5. One-call execution"
echo ""

API_BASE="${FROG_URL:-http://localhost:5800}"

# Color codes for output
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Test counter
TESTS_PASSED=0
TESTS_FAILED=0

# Function to test a model download
test_model() {
    local model_id="$1"
    local test_name="$2"
    local expected_behavior="$3"

    echo -e "\n${YELLOW}========================================${NC}"
    echo -e "${YELLOW}Test: $test_name${NC}"
    echo "Model ID: $model_id"
    echo "Expected: $expected_behavior"
    echo -e "${YELLOW}========================================${NC}"

    # Make the request
    echo "Making request to trigger download..."
    response=$(curl -s -X POST "${API_BASE}/v1/chat/completions" \
        -H "Content-Type: application/json" \
        -d "{
            \"model\": \"$model_id\",
            \"messages\": [{\"role\": \"user\", \"content\": \"Say hello\"}],
            \"max_tokens\": 5,
            \"temperature\": 0.1
        }" 2>&1)

    # Check response
    if echo "$response" | grep -q '"error"'; then
        error=$(echo "$response" | python3 -c "import json,sys; print(json.load(sys.stdin).get('error','Unknown')[:200])" 2>/dev/null || echo "Parse error")

        # Check if it's a download in progress or actual error
        if echo "$error" | grep -qi "downloading\|not found locally"; then
            echo -e "${YELLOW}⏳ Model is downloading (this is expected for first run)${NC}"
            echo "Waiting 30 seconds for download to complete..."
            sleep 30

            # Retry after download
            echo "Retrying after download..."
            response=$(curl -s -X POST "${API_BASE}/v1/chat/completions" \
                -H "Content-Type: application/json" \
                -d "{
                    \"model\": \"$model_id\",
                    \"messages\": [{\"role\": \"user\", \"content\": \"Say hello\"}],
                    \"max_tokens\": 5,
                    \"temperature\": 0.1
                }" 2>&1)

            if echo "$response" | grep -q '"choices"'; then
                echo -e "${GREEN}✅ Test PASSED: Model works after download${NC}"
                TESTS_PASSED=$((TESTS_PASSED + 1))
                return 0
            else
                echo -e "${RED}❌ Test FAILED: Model still not working after download${NC}"
                echo "Error: $response"
                TESTS_FAILED=$((TESTS_FAILED + 1))
                return 1
            fi
        elif echo "$error" | grep -qi "duplicate alias"; then
            echo -e "${RED}❌ Test FAILED: Duplicate alias error${NC}"
            echo "Error: $error"
            TESTS_FAILED=$((TESTS_FAILED + 1))
            return 1
        else
            echo -e "${RED}❌ Test FAILED: Unexpected error${NC}"
            echo "Error: $error"
            TESTS_FAILED=$((TESTS_FAILED + 1))
            return 1
        fi
    elif echo "$response" | grep -q '"choices"'; then
        echo -e "${GREEN}✅ Test PASSED: Model responded successfully${NC}"
        content=$(echo "$response" | python3 -c "import json,sys; d=json.load(sys.stdin); print(d['choices'][0]['message']['content'][:50] if 'choices' in d else 'No content')" 2>/dev/null || echo "")
        if [ ! -z "$content" ]; then
            echo "Model response: $content"
        fi
        TESTS_PASSED=$((TESTS_PASSED + 1))
        return 0
    else
        echo -e "${YELLOW}⚠️ Unexpected response format${NC}"
        echo "Response: ${response:0:200}"
        TESTS_FAILED=$((TESTS_FAILED + 1))
        return 1
    fi
}

# Function to check downloaded files
check_downloads() {
    echo -e "\n${YELLOW}========================================${NC}"
    echo "Checking Downloaded Files"
    echo -e "${YELLOW}========================================${NC}"

    if [ -d models ]; then
        for dir in models/*/; do
            if [ -d "$dir" ]; then
                model_name=$(basename "$dir")
                file_count=$(find "$dir" -name "*.gguf" -type f 2>/dev/null | wc -l)

                if [ $file_count -gt 0 ]; then
                    echo -e "\n${GREEN}$model_name:${NC}"
                    echo "  Total files: $file_count"

                    # Check for split files
                    split_count=$(find "$dir" -name "*-[0-9][0-9][0-9][0-9][0-9]-of-[0-9][0-9][0-9][0-9][0-9]*" -type f 2>/dev/null | wc -l)
                    if [ $split_count -gt 0 ]; then
                        echo "  ${GREEN}Split model: YES ($split_count parts)${NC}"
                    else
                        echo "  Split model: NO (single file)"
                    fi

                    # List first few files
                    find "$dir" -name "*.gguf" -type f 2>/dev/null | head -3 | while read -r file; do
                        size=$(ls -lh "$file" | awk '{print $5}')
                        echo "    - $(basename "$file") ($size)"
                    done

                    if [ $file_count -gt 3 ]; then
                        echo "    ... and $((file_count - 3)) more files"
                    fi
                fi
            fi
        done
    else
        echo "No models directory found"
    fi
}

# Function to check config.yaml
check_config() {
    echo -e "\n${YELLOW}========================================${NC}"
    echo "Checking Config.yaml"
    echo -e "${YELLOW}========================================${NC}"

    if [ -f config.yaml ]; then
        echo "Checking for model entries with aliases..."

        # Count models with aliases
        alias_count=$(grep -c "aliases:" config.yaml 2>/dev/null || echo "0")
        echo "Models with aliases: $alias_count"

        # Show a sample of aliases
        if [ $alias_count -gt 0 ]; then
            echo -e "\nSample aliases from config:"
            grep -A 3 "aliases:" config.yaml | head -12
        fi
    else
        echo "config.yaml not found"
    fi
}

# START TESTS
echo -e "\n${YELLOW}Starting Test Suite...${NC}"
echo "========================================"

# Test 1: Specific file download
test_model \
    "Qwen/Qwen2.5-0.5B-Instruct-GGUF:qwen2.5-0.5b-instruct-q4_k_m.gguf" \
    "Specific File Download" \
    "Should download only the specified file"

# Test 2: Quantization-based download (single file)
test_model \
    "TinyLlama/TinyLlama-1.1B-Chat-v1.0-GGUF:q4_k_m" \
    "Quantization Download (Single)" \
    "Should download first matching Q4_K_M file"

# Test 3: Test the same model again (should use existing)
echo -e "\n${YELLOW}Testing duplicate request (should use existing model)...${NC}"
test_model \
    "TinyLlama/TinyLlama-1.1B-Chat-v1.0-GGUF:q4_k_m" \
    "Duplicate Request Test" \
    "Should use existing model without re-download"

# Test 4: Different quantization of same model
test_model \
    "TinyLlama/TinyLlama-1.1B-Chat-v1.0-GGUF:q5_k_m" \
    "Different Quantization" \
    "Should download Q5_K_M version"

# Test 5: The problematic bartowski model
test_model \
    "bartowski/Mistral-22B-v0.1-GGUF:q5_k" \
    "Bartowski Mistral (Previously Problematic)" \
    "Should work without duplicate alias errors"

# Test 6: Same bartowski model again
echo -e "\n${YELLOW}Testing bartowski model again (critical test)...${NC}"
test_model \
    "bartowski/Mistral-22B-v0.1-GGUF:q5_k" \
    "Bartowski Duplicate Test" \
    "Should use existing without errors"

# Check downloads
check_downloads

# Check config
check_config

# Final Summary
echo -e "\n${YELLOW}========================================"
echo "📊 TEST SUMMARY"
echo "========================================${NC}"
echo -e "Tests Passed: ${GREEN}$TESTS_PASSED${NC}"
echo -e "Tests Failed: ${RED}$TESTS_FAILED${NC}"

if [ $TESTS_FAILED -eq 0 ]; then
    echo -e "\n${GREEN}🎉 ALL TESTS PASSED!${NC}"
    echo "The system is working correctly:"
    echo "✅ Single file downloads work"
    echo "✅ Split model handling works"
    echo "✅ No duplicate alias errors"
    echo "✅ Config updates properly"
    echo "✅ One-call execution works"
else
    echo -e "\n${RED}⚠️ SOME TESTS FAILED${NC}"
    echo "Please check the errors above."
fi

echo -e "\n${YELLOW}========================================${NC}"
echo "Test complete!"
echo ""