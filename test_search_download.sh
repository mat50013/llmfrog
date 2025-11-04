#!/bin/bash

# Quick test script for search and download functionality
# Run this after starting FrogLLM: ./frogllm

echo "=================================="
echo "FrogLLM Search & Download Tests"
echo "=================================="

# Test 1: Search for popular models
echo -e "\n1. Search for Qwen models:"
curl -s "http://localhost:5800/api/v1/models/search?q=qwen&limit=3" | python3 -m json.tool

echo -e "\n2. Search for Llama models:"
curl -s "http://localhost:5800/api/v1/models/search?q=llama+3&limit=3" | python3 -m json.tool

echo -e "\n3. Search for embedding models:"
curl -s "http://localhost:5800/api/v1/models/search?q=embedding+gguf&limit=3" | python3 -m json.tool

# Test 2: Download a small model
echo -e "\n4. Download small test model (Qwen 0.5B):"
curl -X POST http://localhost:5800/api/models/download \
  -H "Content-Type: application/json" \
  -d '{
    "repo": "Qwen/Qwen2.5-0.5B-Instruct-GGUF",
    "filename": "qwen2.5-0.5b-instruct-q4_k_m.gguf",
    "destination": "/downloads"
  }' | python3 -m json.tool

# Test 3: Query non-existent model (should trigger download)
echo -e "\n5. Chat with non-existent model (will auto-download):"
curl -X POST http://localhost:5800/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "Qwen/Qwen2.5-0.5B-Instruct-GGUF:qwen2.5-0.5b-instruct-q4_k_m.gguf",
    "messages": [
      {
        "role": "system",
        "content": "You are a helpful assistant. Be concise."
      },
      {
        "role": "user",
        "content": "Explain quantum computing in one paragraph."
      }
    ],
    "temperature": 0.7,
    "max_tokens": 150
  }' | python3 -m json.tool

# Test 4: Get loaded models
echo -e "\n6. Get currently loaded models:"
curl -s http://localhost:5800/api/v1/models/loaded | python3 -m json.tool

# Test 5: Activity stats
echo -e "\n7. Get activity statistics:"
curl -s http://localhost:5800/api/activity/stats | python3 -m json.tool