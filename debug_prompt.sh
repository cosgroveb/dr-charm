#!/bin/bash

# Send a simple command that should trigger a prompt response
curl -X POST http://localhost:8080/command \
  -H "Content-Type: application/json" \
  -d '{"command": "health"}'

sleep 2

# Get recent output to see what XML we received
curl -s http://localhost:8080/output | jq -r '.lines[-20:][].text'