#!/bin/bash

# Test performance by sending commands via API
# Usage: ./test-performance.sh

API_URL="http://localhost:8080"

# Function to send command
send_command() {
    local cmd="$1"
    echo "Sending: $cmd"
    curl -s -X POST "$API_URL/command" \
        -H "Content-Type: application/json" \
        -d "{\"command\": \"$cmd\"}" | jq -r '.status' 2>/dev/null || echo "Failed"
}

# Function to get latest output
get_output() {
    curl -s "$API_URL/output?since=0" | jq -r '.lines[-5:][].text' 2>/dev/null || echo "No output"
}

# Check if API is up
echo "Checking API health..."
if ! curl -s "$API_URL/health" > /dev/null 2>&1; then
    echo "API not available. Make sure dr-charm is running."
    exit 1
fi

echo "API is up!"
echo "Starting performance test..."

# Wait for connection
sleep 2

# Send a series of commands to test performance
commands=(
    "look"
    "inventory"
    "who"
    "exp all"
    "health"
    "info"
    "time"
    "weather"
)

# Send commands with small delays
for cmd in "${commands[@]}"; do
    send_command "$cmd"
    sleep 0.5
done

echo ""
echo "Test complete. Press F5 in the UI to see performance stats."
echo "Look for slow events in the console output (DR_CHARM_DEBUG=true)."