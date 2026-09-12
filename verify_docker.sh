#!/usr/bin/env bash

set -euo pipefail

IMAGE_NAME="ghosthaze-thinker-verify"
MAX_SIZE_MB=50
PASS=0
FAIL=0
RESULTS=()

pass() {
  RESULTS+=("✅ PASS: $1")
  PASS=$((PASS + 1))
}

fail() {
  RESULTS+=("❌ FAIL: $1")
  FAIL=$((FAIL + 1))
}

# 1. docker build
echo "═══ Check 1: Docker build ═══"
if docker build -t "$IMAGE_NAME" . ; then
  pass "docker build completed successfully"
else
  fail "docker build failed"
  echo ""
  echo "Build failed — remaining checks may be unreliable."
fi

# 2. binary exists and no source in final image
echo ""
echo "═══ Check 2: Binary exists and no source in final image ═══"
if docker run --rm "$IMAGE_NAME" sh -c '[ -x /app/ghosthaze-thinker ] && [ ! -d /app/cmd ] && [ ! -d /app/pkg ] && [ ! -d /app/internal ]'; then
  pass "compiled binary is present and source code is excluded from final image"
else
  fail "binary missing or source code leaked into final image"
fi

# 3. image size under threshold
echo ""
echo "═══ Check 3: Image size < ${MAX_SIZE_MB} MB ═══"
SIZE_BYTES=$(docker image inspect "$IMAGE_NAME" --format='{{.Size}}' 2>/dev/null || echo "0")
SIZE_MB=$((SIZE_BYTES / 1048576))
echo "Image size: ${SIZE_MB} MB"
if [ "$SIZE_MB" -lt "$MAX_SIZE_MB" ]; then
  pass "Image size (${SIZE_MB} MB) is under ${MAX_SIZE_MB} MB"
else
  fail "Image size (${SIZE_MB} MB) exceeds ${MAX_SIZE_MB} MB limit"
fi

# 4. docker compose config validation
echo ""
echo "═══ Check 4: docker compose config validation ═══"
CREATED_DUMMY_ENV=false
if [ ! -f .env ]; then
  touch .env
  CREATED_DUMMY_ENV=true
fi

if docker compose config > /dev/null 2>&1; then
  pass "docker-compose.yml is valid"
else
  fail "docker-compose.yml is invalid"
fi

if [ "$CREATED_DUMMY_ENV" = true ]; then
  rm .env
fi

# 5. github actions workflow yaml validity
echo ""
echo "═══ Check 5: GitHub Actions workflow YAML validity ═══"
WORKFLOW_FILE=".github/workflows/docker-publish.yml"
if [ ! -f "$WORKFLOW_FILE" ]; then
  fail "Workflow file $WORKFLOW_FILE does not exist"
else
  if command -v python3 > /dev/null 2>&1; then
    if python3 -c "import yaml; yaml.safe_load(open('$WORKFLOW_FILE'))" 2>/dev/null; then
      pass "Workflow YAML ($WORKFLOW_FILE) is syntactically valid"
    else
      fail "Workflow YAML ($WORKFLOW_FILE) has syntax errors"
    fi
  elif command -v ruby > /dev/null 2>&1; then
    if ruby -ryaml -e "YAML.safe_load(File.read('$WORKFLOW_FILE'))" 2>/dev/null; then
      pass "Workflow YAML ($WORKFLOW_FILE) is syntactically valid"
    else
      fail "Workflow YAML ($WORKFLOW_FILE) has syntax errors"
    fi
  else
    if [ -s "$WORKFLOW_FILE" ] && head -1 "$WORKFLOW_FILE" | grep -qE '^(name:|on:|\-\-\-)'; then
      pass "Workflow YAML ($WORKFLOW_FILE) exists and appears valid"
    else
      fail "Workflow YAML ($WORKFLOW_FILE) appears invalid or empty"
    fi
  fi
fi

# cleanup verification image
echo ""
echo "Cleaning up verification image..."
docker rmi "$IMAGE_NAME" > /dev/null 2>&1 || true

echo ""
echo "╔══════════════════════════════════════╗"
echo "║       VERIFICATION SUMMARY           ║"
echo "╠══════════════════════════════════════╣"
for result in "${RESULTS[@]}"; do
  echo "║  $result"
done
echo "╠══════════════════════════════════════╣"
printf "║  Total: %d passed, %d failed\n" "$PASS" "$FAIL"
echo "╚══════════════════════════════════════╝"

if [ "$FAIL" -gt 0 ]; then
  exit 1
fi
