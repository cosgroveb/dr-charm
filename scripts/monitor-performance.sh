#!/bin/bash

# Monitor performance of dr-charm
# This script watches for performance events in the log while dr-charm runs

echo "DragonRealms Charm Performance Monitor"
echo "======================================"
echo ""
echo "Instructions:"
echo "1. Run dr-charm in another terminal with: DR_CHARM_DEBUG=true DR_CHARM_ENHANCED=true ./dr-charm"
echo "2. Press F5 in the UI to see performance stats"
echo "3. This script will show slow events (>50ms) in real-time"
echo ""
echo "Monitoring for performance events..."
echo ""

# Watch for performance events in real-time
tail -f ~/.dr-charm/logs/debug/raw-xml-*.log 2>/dev/null | grep --line-buffered "\[PERF\]" &
TAIL_PID=$!

# Also watch console output if running
if [ -f "dr-charm.log" ]; then
    tail -f dr-charm.log | grep --line-buffered "\[PERF\]" &
    TAIL2_PID=$!
fi

# Trap to clean up on exit
trap "kill $TAIL_PID $TAIL2_PID 2>/dev/null" EXIT

# Wait for user to stop
echo "Press Ctrl+C to stop monitoring..."
wait