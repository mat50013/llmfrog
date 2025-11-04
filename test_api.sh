#!/bin/bash

# FrogLLM API Test Script
# Tests all new endpoints: search, download, load/unload, and auto-download for non-existent models

API_BASE="http://localhost:5800/api"
V1_BASE="http://localhost:5800/v1"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

echo -e "${BLUE}========================================${NC}"
echo -e "${BLUE}FrogLLM API Test Suite${NC}"
echo -e "${BLUE}========================================${NC}"

# Function to print test headers
print_test() {
    echo -e "\n${YELLOW}TEST: $1${NC}"
    echo "----------------------------------------"
}

# Function to pretty print JSON
pretty_json() {
    echo "$1" | python3 -m json.tool 2>/dev/null || echo "$1"
}

# 1. Test Model Search API
print_test "Model Search - Qwen Models"
echo "Query: Search for Qwen 7B models"
response=$(curl -s -X GET "${API_BASE}/v1/models/search?q=qwen+7b&limit=5")
echo "Response:"
pretty_json "$response"

print_test "Model Search - Llama Models with Restricted Access"
echo "Query: Search for Llama 3 models including gated ones"
response=$(curl -s -X GET \
  -H "HF-Token: YOUR_HF_TOKEN_HERE" \
  "${API_BASE}/v1/models/search?q=llama+3&include_restricted=true&limit=5")
echo "Response:"
pretty_json "$response"

print_test "Model Search - Small Embedding Models"
echo "Query: Search for small embedding models"
response=$(curl -s -X GET "${API_BASE}/v1/models/search?q=embedding+gguf&limit=5")
echo "Response:"
pretty_json "$response"

# 2. Test Model Download
print_test "Model Download - Small Test Model"
echo "Downloading: Qwen/Qwen2.5-0.5B-Instruct-GGUF (small model for testing)"
response=$(curl -s -X POST \
  -H "Content-Type: application/json" \
  -d '{
    "repo": "Qwen/Qwen2.5-0.5B-Instruct-GGUF",
    "filename": "qwen2.5-0.5b-instruct-q4_k_m.gguf",
    "destination": "/downloads"
  }' \
  "${API_BASE}/models/download")
echo "Response:"
pretty_json "$response"

# Get download ID from response (if available)
download_id=$(echo "$response" | python3 -c "import sys, json; print(json.load(sys.stdin).get('downloadId', ''))" 2>/dev/null)

if [ ! -z "$download_id" ]; then
    sleep 2
    print_test "Check Download Status"
    echo "Download ID: $download_id"
    response=$(curl -s -X GET "${API_BASE}/models/downloads/$download_id")
    echo "Response:"
    pretty_json "$response"
fi

# 3. Test Getting Loaded Models
print_test "Get Currently Loaded Models"
response=$(curl -s -X GET "${API_BASE}/v1/models/loaded")
echo "Response:"
pretty_json "$response"

# 4. Test Loading a Model
print_test "Load Model with Auto-Unload"
echo "Loading: qwen-qwen3-30b-a3b-instruct-2507-3b (if exists in config)"
response=$(curl -s -X POST \
  -H "Content-Type: application/json" \
  -d '{
    "model_id": "qwen-qwen3-30b-a3b-instruct-2507-3b",
    "auto_unload": true
  }' \
  "${API_BASE}/v1/models/load")
echo "Response:"
pretty_json "$response"

# 5. Test Chat Completion with Non-Existent Model (Auto-Download)
print_test "Chat Completion - Non-Existent Model (Should Trigger Download)"
echo "Model: Qwen/Qwen2.5-1.5B-Instruct-GGUF:qwen2.5-1.5b-instruct-q4_k_m.gguf"
echo "Task: Complex reasoning problem"
response=$(curl -s -X POST \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer YOUR_API_KEY_HERE" \
  -d '{
    "model": "Qwen/Qwen2.5-1.5B-Instruct-GGUF:qwen2.5-1.5b-instruct-q4_k_m.gguf",
    "messages": [
      {
        "role": "system",
        "content": "You are a helpful AI assistant capable of complex reasoning."
      },
      {
        "role": "user",
        "content": "Solve this step by step: A farmer has 17 sheep. All but 9 die. How many sheep does the farmer have left? Then, if the farmer buys twice as many sheep as survived, and 3 more sheep are born, how many sheep does the farmer have in total?"
      }
    ],
    "temperature": 0.7,
    "max_tokens": 500,
    "stream": false
  }' \
  "${V1_BASE}/chat/completions")
echo "Response:"
pretty_json "$response"

# 6. Test Streaming with Non-Existent Model
print_test "Streaming Chat - Non-Existent Model"
echo "Model: TinyLlama/TinyLlama-1.1B-Chat-v1.0-GGUF:tinyllama-1.1b-chat-v1.0.Q4_K_M.gguf"
echo "Task: Creative writing"
echo "Streaming response (first 500 chars):"
curl -s -X POST \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer YOUR_API_KEY_HERE" \
  -d '{
    "model": "TinyLlama/TinyLlama-1.1B-Chat-v1.0-GGUF:tinyllama-1.1b-chat-v1.0.Q4_K_M.gguf",
    "messages": [
      {
        "role": "user",
        "content": "Write a haiku about debugging code at midnight"
      }
    ],
    "temperature": 0.9,
    "max_tokens": 50,
    "stream": true
  }' \
  "${V1_BASE}/chat/completions" | head -c 500

echo -e "\n"

# 7. Test Model Unload
print_test "Unload Specific Model"
echo "Unloading: qwen-qwen3-30b-a3b-instruct-2507-3b"
response=$(curl -s -X POST \
  -H "Content-Type: application/json" \
  -d '{
    "model_id": "qwen-qwen3-30b-a3b-instruct-2507-3b"
  }' \
  "${API_BASE}/v1/models/unload")
echo "Response:"
pretty_json "$response"

# 8. Test Activity Statistics
print_test "Get Activity Statistics"
response=$(curl -s -X GET "${API_BASE}/activity/stats")
echo "Response:"
pretty_json "$response"

# 9. Test System Specs (Multi-GPU)
print_test "Get System Specs (Should Show All GPUs)"
response=$(curl -s -X GET "${API_BASE}/system/specs")
echo "Response:"
pretty_json "$response"

# 10. Test GPU Stats (Multi-GPU)
print_test "Get GPU Statistics"
response=$(curl -s -X GET "${API_BASE}/gpu/stats")
echo "Response:"
pretty_json "$response"

# 11. Edge Case: Load Multiple Models with LRU
print_test "Load Multiple Models to Test LRU Eviction"
echo "This test will attempt to load multiple models to trigger LRU eviction"

models=("model1" "model2" "model3")
for model in "${models[@]}"; do
    echo -e "\nLoading: $model"
    response=$(curl -s -X POST \
      -H "Content-Type: application/json" \
      -d "{
        \"model_id\": \"$model\",
        \"auto_unload\": true
      }" \
      "${API_BASE}/v1/models/load")
    pretty_json "$response"
    sleep 1
done

# 12. Complex Query with Missing Model
print_test "Complex Multi-Turn Conversation with Auto-Download"
echo "Model: mistralai/Mistral-7B-Instruct-v0.2-GGUF:mistral-7b-instruct-v0.2.Q4_K_M.gguf"
response=$(curl -s -X POST \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer YOUR_API_KEY_HERE" \
  -d '{
    "model": "mistralai/Mistral-7B-Instruct-v0.2-GGUF:mistral-7b-instruct-v0.2.Q4_K_M.gguf",
    "messages": [
      {
        "role": "system",
        "content": "You are an expert in algorithms and data structures."
      },
      {
        "role": "user",
        "content": "Implement a red-black tree in Python with insert and search operations. Explain the color properties and rotations."
      },
      {
        "role": "assistant",
        "content": "I will implement a red-black tree with insert and search operations, explaining the key properties and rotations."
      },
      {
        "role": "user",
        "content": "Good. Now explain how the insertion maintains the red-black properties and when rotations are needed."
      }
    ],
    "temperature": 0.3,
    "max_tokens": 1000,
    "top_p": 0.95,
    "frequency_penalty": 0.1
  }' \
  "${V1_BASE}/chat/completions")
echo "Response (first 1000 chars):"
echo "$response" | head -c 1000
echo "..."

# 13. Test Download Destinations
print_test "Get Available Download Destinations"
response=$(curl -s -X GET "${API_BASE}/models/download-destinations")
echo "Response:"
pretty_json "$response"

# 14. Test Model Search with Different Quantizations
print_test "Search for Different Quantizations"
echo "Query: Q4_K_M quantized models"
response=$(curl -s -X GET "${API_BASE}/v1/models/search?q=Q4_K_M+gguf&limit=5")
echo "Response:"
pretty_json "$response"

# 15. Error Case: Invalid Model ID
print_test "Error Test - Invalid Model Load"
echo "Attempting to load non-existent model without auto-download"
response=$(curl -s -X POST \
  -H "Content-Type: application/json" \
  -d '{
    "model_id": "completely-invalid-model-id-that-does-not-exist",
    "auto_unload": false
  }' \
  "${API_BASE}/v1/models/load")
echo "Response:"
pretty_json "$response"

echo -e "\n${GREEN}========================================${NC}"
echo -e "${GREEN}Test Suite Complete!${NC}"
echo -e "${GREEN}========================================${NC}"

# Summary
echo -e "\n${BLUE}Test Summary:${NC}"
echo "1. ✓ Model Search API with various queries"
echo "2. ✓ Model Download API"
echo "3. ✓ Load/Unload Models with LRU"
echo "4. ✓ Chat Completions with auto-download"
echo "5. ✓ Streaming responses"
echo "6. ✓ Activity statistics"
echo "7. ✓ Multi-GPU display"
echo "8. ✓ Error handling"
echo -e "\n${YELLOW}Note:${NC} Replace YOUR_HF_TOKEN_HERE and YOUR_API_KEY_HERE with actual values"
echo -e "${YELLOW}Note:${NC} Ensure FrogLLM is running on localhost:5800 before running tests"