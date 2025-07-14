#!/bin/bash

# DragonRealms API Test Script
# This demonstrates how Claude can control the game

echo "=== DragonRealms API Test ==="
echo "Make sure dr-charm is running first!"
echo

# Check health
echo "1. Checking API health..."
curl -s http://localhost:8080/health | jq .
echo

# Send a command
echo "2. Sending 'look' command..."
curl -s -X POST http://localhost:8080/command \
  -H "Content-Type: application/json" \
  -d '{"command": "look"}' | jq .
echo

# Wait a bit for output
sleep 2

# Get recent output
echo "3. Getting recent output..."
curl -s http://localhost:8080/output?since=0 | jq .
echo

# Send another command
echo "4. Sending 'health' command..."
curl -s -X POST http://localhost:8080/command \
  -H "Content-Type: application/json" \
  -d '{"command": "health"}' | jq .
echo

# Wait and get new output
sleep 2
echo "5. Getting updated output..."
curl -s http://localhost:8080/output?since=2 | jq .