#!/usr/bin/env bash
# smoke-test.sh — Cloud Agent Platform 集成验证脚本
# 一键跑通全链路：编译 + 测试 + 结果摘要

set -euo pipefail

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
NC='\033[0m' # No Color

PASS=0
FAIL=0
SKIP=0
START_TIME=$(date +%s)

echo -e "${CYAN}========================================${NC}"
echo -e "${CYAN}  Cloud Agent Platform — Smoke Test${NC}"
echo -e "${CYAN}========================================${NC}"
echo ""

# Step 1: Build
echo -e "${YELLOW}[1/3] Building...${NC}"
if go build ./... 2>&1; then
    echo -e "${GREEN}✅ Build passed${NC}"
    ((PASS++))
else
    echo -e "${RED}❌ Build failed${NC}"
    ((FAIL++))
fi
echo ""

# Step 2: Vet
echo -e "${YELLOW}[2/3] Running go vet...${NC}"
if go vet ./... 2>&1; then
    echo -e "${GREEN}✅ Vet passed${NC}"
    ((PASS++))
else
    echo -e "${RED}❌ Vet failed${NC}"
    ((FAIL++))
fi
echo ""

# Step 3: Test
echo -e "${YELLOW}[3/3] Running tests (timeout 120s)...${NC}"
TEST_OUTPUT=$(go test -p 2 -count=1 -timeout 120s ./... 2>&1) || true
echo "$TEST_OUTPUT"

# Parse test results
while IFS= read -r line; do
    if echo "$line" | grep -q '^ok\s'; then
        ((PASS++))
    elif echo "$line" | grep -q 'FAIL\s'; then
        ((FAIL++))
    elif echo "$line" | grep -q 'no test files'; then
        ((SKIP++))
    fi
done <<< "$TEST_OUTPUT"
echo ""

# Summary
END_TIME=$(date +%s)
DURATION=$((END_TIME - START_TIME))

echo -e "${CYAN}========================================${NC}"
echo -e "${CYAN}  Results${NC}"
echo -e "${CYAN}========================================${NC}"
echo -e "  ✅ Passed:  ${GREEN}${PASS}${NC}"
echo -e "  ❌ Failed:  ${RED}${FAIL}${NC}"
echo -e "  ⏭️  Skipped: ${YELLOW}${SKIP}${NC}"
echo -e "  ⏱️  Time:    ${DURATION}s"
echo ""

if [ "$FAIL" -gt 0 ]; then
    echo -e "${RED}❌ SMOKE TEST FAILED${NC}"
    exit 1
else
    echo -e "${GREEN}🎉 SMOKE TEST PASSED${NC}"
    exit 0
fi
