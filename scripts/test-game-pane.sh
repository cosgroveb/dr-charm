#!/bin/bash

echo "Game Pane Performance Test"
echo "========================="
echo ""
echo "This will test the performance of game pane updates"
echo ""

# Create a test file with many lines of game output
cat > test-game-output.txt << 'EOF'
[Via Iltesh, Peddler's Alley]
A cramped alley winds between ramshackle buildings, their walls leaning together as if sharing secrets. Tattered awnings flutter overhead, and the air is thick with the scent of spices and woodsmoke. Peddlers hawk their wares from makeshift stalls, their voices rising above the general din.
You also see a wooden barrel, a tattered merchant's tent, and a mangy alley cat.
Obvious paths: north, south, east.
A trader just arrived.
You hear a merchant shouting about fresh bread.
The alley cat meows plaintively.
Someone whispers to you, "Are you looking for something special?"
A guard strolls by, eyeing the crowd suspiciously.
You gained 5 experience in perception.
The wooden barrel creaks in the wind.
A peddler calls out, "Fine silks from the east!"
You notice a small coin on the ground.
The mangy cat stretches lazily.
A beggar holds out his hand hopefully.
You feel a slight breeze from the north.
The merchant adjusts his wares.
A child runs past, laughing.
The guard continues his patrol.
You are stunned by the variety of goods.
The tent flap rustles mysteriously.
EOF

echo "Test data created. When running dr-charm with DR_CHARM_DEBUG=true:"
echo "1. Look for [PERF] Game pane slow messages"
echo "2. Check the breakdown of time spent in:"
echo "   - filter: Processing and filtering lines"
echo "   - trigger: Regex matching for highlights"
echo "   - output: Adding to output buffer"
echo "   - layout: Updating the UI layout"
echo ""
echo "Expected improvements:"
echo "- Layout updates batched (only once per chunk)"
echo "- Trigger matching optimized with quick checks"
echo "- Total time should be under 20ms for typical chunks"