#!/bin/bash
# Generates docs/COMMANDS.md from the command definitions in defs.go.
set -e
mkdir -p docs
{
echo "# hex-music-bot — Command Reference"
echo ""
echo "Auto-generated from \`internal/commands/defs.go\`. Do not edit manually."
echo ""
echo "| Command | Description |"
echo "|---------|-------------|"
grep -oP 'Name:\s+"\K[^"]+' internal/commands/defs.go | while read -r name; do
    desc=$(grep -A1 "Name:.*\"${name}\"" internal/commands/defs.go | grep -oP 'Description:\s+"\K[^"]+' | head -1)
    echo "| \`/${name}\` | ${desc} |"
done
echo ""
echo "---"
echo "_Last generated: $(date -u +"%Y-%m-%dT%H:%M:%SZ")_"
} > docs/COMMANDS.md
echo "Generated docs/COMMANDS.md with $(grep -c '| `/' docs/COMMANDS.md) commands"
