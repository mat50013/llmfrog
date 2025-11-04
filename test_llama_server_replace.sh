#!/bin/bash

# Test script for --llama-server flag functionality

echo "========================================"
echo "Testing --llama-server Flag"
echo "========================================"

# Show current llama-server path in config
echo -e "\n1. Current llama-server paths in config:"
echo "----------------------------------------"
grep -n "llama-server" config.yaml | head -5

# Test the --llama-server flag
echo -e "\n2. Testing llama-server replacement:"
echo "----------------------------------------"

# Example usage (replace with actual path to your llama-server binary)
LLAMA_SERVER_PATH="/path/to/your/llama-server"

echo "Usage examples:"
echo ""
echo "# Replace llama-server path with a specific binary and rebuild:"
echo "./frogllm --llama-server /usr/local/bin/llama-server"
echo ""
echo "# Replace with a custom compiled version:"
echo "./frogllm --llama-server ~/llama.cpp/build/bin/llama-server"
echo ""
echo "# Replace with a different backend version:"
echo "./frogllm --llama-server ./binaries/llama-server-cuda12/llama-server"
echo ""

# Check if a test llama-server exists
if [ -f "binaries/llama-server/build/bin/llama-server" ]; then
    echo -e "\n3. Found existing llama-server at: binaries/llama-server/build/bin/llama-server"
    echo "You can test with this or specify your own path"
fi

echo -e "\n========================================"
echo "How It Works:"
echo "========================================"
echo "1. The --llama-server flag replaces ALL occurrences of llama-server paths in config.yaml"
echo "2. It creates a backup of your original config (config.yaml.backup.TIMESTAMP)"
echo "3. It automatically rebuilds FrogLLM with the new configuration"
echo "4. The new binary will use the updated llama-server path"
echo ""
echo "This is useful when:"
echo "- You have a custom-compiled llama-server with specific optimizations"
echo "- You want to test different versions of llama-server"
echo "- You need to use a system-wide installation instead of the bundled one"
echo "- You're developing/debugging llama-server itself"
echo ""