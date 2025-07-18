#!/bin/bash

# Run dr-charm in the background and test performance
# Usage: ./run-test.sh

# Kill any existing dr-charm processes
echo "Cleaning up existing processes..."
pkill -f "dr-charm" 2>/dev/null

# Start dr-charm in CLI mode for testing
echo "Starting dr-charm in CLI mode with debug..."
DR_CHARM_DEBUG=true DR_CHARM_CLI=true ./dr-charm > dr-charm.log 2>&1 &
DR_PID=$!

echo "dr-charm started with PID: $DR_PID"
echo "Waiting for startup..."
sleep 5

# Check if it's still running
if ! ps -p $DR_PID > /dev/null; then
    echo "dr-charm failed to start. Check dr-charm.log"
    cat dr-charm.log
    exit 1
fi

# Run performance test
echo "Running performance test..."
./test-performance.sh

# Give time for stats to accumulate
echo "Waiting for stats to process..."
sleep 2

# Extract performance data from log
echo ""
echo "=== Performance Events from Log ==="
grep "\[PERF\]" dr-charm.log | tail -20

echo ""
echo "dr-charm is still running (PID: $DR_PID)"
echo "To stop: kill $DR_PID"
echo "To see full log: tail -f dr-charm.log"