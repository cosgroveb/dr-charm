#!/bin/bash

# DragonRealms navigation script
# Usage: ./navigate.sh "direction1" "direction2" "direction3" ...

# Default delay between moves (in seconds)
DELAY=${MOVE_DELAY:-2}

# Check if drcli exists
if [ ! -f "./drcli" ]; then
    echo "Error: drcli not found in current directory"
    exit 1
fi

# Check if any directions provided
if [ $# -eq 0 ]; then
    echo "Usage: $0 \"direction1\" \"direction2\" \"direction3\" ..."
    echo "Example: $0 \"north\" \"go gate\" \"east\" \"east\""
    exit 1
fi

echo "Starting navigation with $# moves..."
echo "Delay between moves: ${DELAY}s"
echo "------------------------"

# Counter for moves
count=1

# Navigate through each direction
for direction in "$@"; do
    echo "[$count/$#] Moving: $direction"
    ./drcli -c "$direction"
    
    # Check if it's the last move
    if [ $count -ne $# ]; then
        sleep $DELAY
    fi
    
    ((count++))
done

echo "------------------------"
echo "Navigation complete!"
echo "Checking current location..."
./drcli -c "look"