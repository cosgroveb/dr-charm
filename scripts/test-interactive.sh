#!/bin/bash

# Kill any existing processes
pkill -f "dr-charm" 2>/dev/null

# Run in foreground to test
echo "Starting dr-charm in normal UI mode (press Ctrl+C to stop)..."
echo "Once connected, press F5 to see performance stats"
echo ""

DR_CHARM_DEBUG=true DR_CHARM_ENHANCED=true ./dr-charm