#!/bin/bash
# Rebrand Reasonix → Wenhao
set -e

SRC=/home/wen/projects/reasonix-source

# Step 1: Replace in all source files
echo "=== Step 1: Text replacements ==="
find "$SRC" -type f \
  \( -name "*.go" -o -name "*.mod" -o -name "*.sum" -o -name "*.toml" \
     -o -name "*.md" -o -name "*.json" -o -name "*.html" \
     -o -name "*.ts" -o -name "*.tsx" -o -name "*.css" \
     -o -name "Makefile" -o -name "*.js" -o -name "*.mjs" \
     -o -name "*.yml" -o -name "*.yaml" \) \
  -not -path "*/.git/*" \
  -not -path "*/node_modules/*" \
  -not -path "*/dist/*" \
  -not -path "*/site/node_modules/*" \
  -not -path "*/desktop/frontend/node_modules/*" \
  -not -path "*/desktop/frontend/dist/*" \
  | while read f; do
    sed -i \
      -e 's|reasonix|wenhao|g' \
      -e 's|Reasonix|Wenhao|g' \
      -e 's|REASONIX|WENHAO|g' \
      "$f"
  done

# Step 2: Rename directories
echo "=== Step 2: Directory renames ==="
if [ -d "$SRC/cmd/reasonix" ]; then
  mv "$SRC/cmd/reasonix" "$SRC/cmd/wenhao"
fi
if [ -d "$SRC/cmd/reasonix-plugin-example" ]; then
  mv "$SRC/cmd/reasonix-plugin-example" "$SRC/cmd/wenhao-plugin-example"
fi

# Step 3: Rename files
echo "=== Step 3: File renames ==="
if [ -f "$SRC/reasonix.example.toml" ]; then
  mv "$SRC/reasonix.example.toml" "$SRC/wenhao.example.toml"
fi
if [ -f "$SRC/REASONIX.md" ]; then
  mv "$SRC/REASONIX.md" "$SRC/WENHAO.md"
fi
if [ -f "$SRC/npm/reasonix/package.json" ]; then
  mv "$SRC/npm/reasonix" "$SRC/npm/wenhao"
fi

# Step 4: Fix the go.mod module path specifically (sed may have broken it)
echo "=== Step 4: Fix go.mod ==="
sed -i 's|^module wenhao$|module wenhao|' "$SRC/go.mod"

# Step 5: Fix desktop go.mod
if [ -f "$SRC/desktop/go.mod" ]; then
  sed -i 's|wenhao v|wenhao v|' "$SRC/desktop/go.mod"
fi

echo "=== Done ==="
echo "Don't forget to run: go mod tidy"
