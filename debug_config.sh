#!/bin/bash

# Debug script to show exactly what's in config vs what system expects

echo "========================================"
echo "🔍 Config vs Expectations Debug"
echo "========================================"
echo ""

API_BASE="${FROG_URL:-http://localhost:5800}"

# What we're testing
REQUEST_ID="bartowski/Mistral-22B-v0.1-GGUF:q5_k"
EXPECTED_CONFIG_ID="bartowski-mistral-22b-v0.1-gguf-q5_k"

echo "Request ID: $REQUEST_ID"
echo "Expected Config ID: $EXPECTED_CONFIG_ID"
echo ""

# Check what's actually in config
echo "=== Current Config Status ==="
if [ -f config.yaml ]; then
    echo "1. Models in config:"
    grep -A 1 "^  [a-zA-Z]" config.yaml | grep -v "^--$" | while read line; do
        if [[ $line =~ ^[[:space:]]{2}[^[:space:]] ]]; then
            echo "   - $line"
        fi
    done

    echo ""
    echo "2. Looking for bartowski entries:"
    if grep -q "bartowski" config.yaml; then
        echo "Found bartowski model(s):"
        grep -B2 -A15 "bartowski" config.yaml
    else
        echo "   No bartowski entries found"
    fi

    echo ""
    echo "3. Groups membership:"
    echo "   all-models group members:"
    sed -n '/all-models:/,/^[^ ]/p' config.yaml | grep -A20 "members:" | grep "    -" || echo "   No members found"

else
    echo "config.yaml not found!"
fi

echo ""
echo "=== Testing Model Resolution ==="

# Test if model can be found via API
echo "1. Checking /v1/models endpoint:"
models=$(curl -s "${API_BASE}/v1/models" 2>/dev/null)
if [ $? -eq 0 ]; then
    echo "$models" | python3 -c "
import json, sys
data = json.load(sys.stdin)
models = data.get('data', [])
print(f'   Total models available: {len(models)}')
for model in models:
    if 'bartowski' in model.get('id', '').lower():
        print(f'   - Found: {model.get(\"id\")}')
" 2>/dev/null || echo "   Error parsing models"
else
    echo "   Cannot reach API"
fi

echo ""
echo "=== What System Expects vs Reality ==="
echo ""
echo "EXPECTED:"
echo "  1. Config should have model with ID: $EXPECTED_CONFIG_ID"
echo "  2. Model should have alias: $REQUEST_ID"
echo "  3. Model should be in group: all-models"
echo "  4. Process group should contain: $EXPECTED_CONFIG_ID"
echo ""
echo "REALITY CHECK:"

# Check each expectation
CHECKS_PASSED=0
CHECKS_FAILED=0

# Check 1: Model in config with correct ID
if grep -q "$EXPECTED_CONFIG_ID:" config.yaml 2>/dev/null; then
    echo "  ✅ Model exists with correct ID"
    CHECKS_PASSED=$((CHECKS_PASSED + 1))
else
    echo "  ❌ Model NOT found with expected ID"
    echo "     Actual model IDs in config:"
    grep "^  [a-zA-Z]" config.yaml | sed 's/://' | sed 's/^/       /'
    CHECKS_FAILED=$((CHECKS_FAILED + 1))
fi

# Check 2: Has correct alias
if grep -A20 "$EXPECTED_CONFIG_ID:" config.yaml 2>/dev/null | grep -q "$REQUEST_ID"; then
    echo "  ✅ Model has correct alias"
    CHECKS_PASSED=$((CHECKS_PASSED + 1))
else
    echo "  ❌ Alias NOT found"
    echo "     Looking for aliases under $EXPECTED_CONFIG_ID:"
    grep -A20 "$EXPECTED_CONFIG_ID:" config.yaml 2>/dev/null | grep -A5 "aliases:" || echo "     No aliases section found"
    CHECKS_FAILED=$((CHECKS_FAILED + 1))
fi

# Check 3: In group
if grep -A20 "all-models:" config.yaml 2>/dev/null | grep -q "$EXPECTED_CONFIG_ID"; then
    echo "  ✅ Model in all-models group"
    CHECKS_PASSED=$((CHECKS_PASSED + 1))
else
    echo "  ❌ Model NOT in group"
    CHECKS_FAILED=$((CHECKS_FAILED + 1))
fi

echo ""
echo "=== Diagnosis ==="
if [ $CHECKS_FAILED -eq 0 ]; then
    echo "✅ Config looks correct! If there are still errors, the issue is in the code logic."
else
    echo "❌ Config has issues. The mismatch is:"
    if ! grep -q "$EXPECTED_CONFIG_ID:" config.yaml 2>/dev/null; then
        echo "  - Model is saved with wrong ID (not $EXPECTED_CONFIG_ID)"
        echo "  - Need to fix ID generation in reloadConfigForNewModel"
    fi
    if ! grep -A20 "$EXPECTED_CONFIG_ID:" config.yaml 2>/dev/null | grep -q "$REQUEST_ID"; then
        echo "  - Aliases are not being added correctly"
        echo "  - Need to fix alias generation"
    fi
    if ! grep -A20 "all-models:" config.yaml 2>/dev/null | grep -q "$EXPECTED_CONFIG_ID"; then
        echo "  - Model not being added to group"
        echo "  - Need to fix group membership addition"
    fi
fi

echo ""