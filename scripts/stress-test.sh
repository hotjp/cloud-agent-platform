#!/bin/bash
# E2E Stress Test: 10 concurrent tasks
# 5 same repo (octocat/Hello-World), 5 different repos

set -e

TOKEN="eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiJ1c2VyLTAwMSIsImNsaWVudF9pZCI6InRlc3QtY2xpZW50IiwidGVuYW50X2lkIjoiZGVmYXVsdCIsImlhdCI6MTc3ODgyMTM1NiwiZXhwIjoxNzc4ODI0OTU2fQ.Y0rSAVvZIAiEx--WJ9_N_gKScWuGxwo3neEGlvJHrd0"
API="http://localhost:18080/api/v1/tasks"

submit() {
  local goal="$1"
  local repo="$2"
  local branch="${3:-master}"
  
  payload="{\"goal\":\"$goal\",\"priority\":5"
  if [ -n "$repo" ]; then
    payload="$payload,\"repository\":{\"url\":\"$repo\",\"branch\":\"$branch\"}"
  fi
  payload="$payload}"
  
  result=$(curl -s --noproxy localhost -X POST "$API" \
    -H "Content-Type: application/json" \
    -H "Authorization: Bearer $TOKEN" \
    -d "$payload" 2>&1)
  
  task_id=$(echo "$result" | grep -o '"taskId":"[^"]*"' | cut -d'"' -f4)
  echo "  → $task_id: $goal"
  echo "$task_id"
}

echo "============================================"
echo "  CAP Stress Test — 10 Concurrent Tasks"
echo "============================================"
echo ""

TASKS=()

echo "=== Batch 1: 5 tasks on same repo (octocat/Hello-World) ==="
for i in $(seq 1 5); do
  id=$(submit "Create file-stress-a${i}.txt with: Stress test file ${i}" "https://github.com/octocat/Hello-World.git")
  TASKS+=("$id")
done

echo ""
echo "=== Batch 2: 5 tasks on different repos ==="

# Task 6: rails/rails
id=$(submit "Create file-stress-b1.txt with: Rails repo test" "https://github.com/rails/rails.git")
TASKS+=("$id")

# Task 7: django/django
id=$(submit "Create file-stress-b2.txt with: Django repo test" "https://github.com/django/django.git")
TASKS+=("$id")

# Task 8: pallets/flask
id=$(submit "Create file-stress-b3.txt with: Flask repo test" "https://github.com/pallets/flask.git")
TASKS+=("$id")

# Task 9: expressjs/express
id=$(submit "Create file-stress-b4.txt with: Express repo test" "https://github.com/expressjs/express.git")
TASKS+=("$id")

# Task 10: no repo (empty project)
id=$(submit "Create file-stress-b5.txt with: Empty project test" "")
TASKS+=("$id")

echo ""
echo "=== All 10 submitted ==="
echo "Task IDs:"
for t in "${TASKS[@]}"; do
  echo "  $t"
done

echo ""
echo "Waiting for all tasks to complete (max 120s)..."

# Poll until all done or timeout
for attempt in $(seq 1 24); do
  sleep 5
  
  completed=0
  failed=0
  pending=0
  
  for t in "${TASKS[@]}"; do
    status=$(curl -s --noproxy localhost "$API/$t" \
      -H "Authorization: Bearer $TOKEN" 2>/dev/null | grep -o '"status":"[^"]*"' | head -1 | cut -d'"' -f4)
    
    case "$status" in
      COMPLETED) ((completed++)) ;;
      FAILED|CANCELLED) ((failed++)) ;;
      *) ((pending++)) ;;
    esac
  done
  
  echo "  [${attempt}/24] completed=$completed failed=$failed pending=$pending"
  
  if [ $((completed + failed)) -eq 10 ]; then
    echo ""
    echo "=== All tasks finished ==="
    break
  fi
done

echo ""
echo "=== Final Status ==="
for t in "${TASKS[@]}"; do
  status=$(curl -s --noproxy localhost "$API/$t" \
    -H "Authorization: Bearer $TOKEN" 2>/dev/null | grep -o '"status":"[^"]*"' | head -1 | cut -d'"' -f4)
  echo "  $t: $status"
done

echo ""
echo "=== Git Containers ==="
docker ps --filter "name=cap-git" --format "table {{.ID}}\t{{.Names}}\t{{.Ports}}" 2>&1

echo ""
echo "=== Worker Containers (leftover check) ==="
docker ps --filter "name=cap-worker" --format "table {{.ID}}\t{{.Names}}\t{{.Status}}" 2>&1

echo ""
echo "=== Docker resource usage ==="
docker stats --no-stream --format "table {{.Name}}\t{{.CPUPerc}}\t{{.MemUsage}}" 2>&1 | head -20

echo ""
echo "============================================"
echo "  Stress Test Complete"
echo "============================================"
