#!/bin/bash
# Test scenarios for the notification system
API="http://localhost:8080"

echo "=== 1. Health Check ==="
curl -s $API/health | jq .

echo ""
echo "=== 2. Single Notification ==="
RESP=$(curl -s -X POST $API/api/v1/notifications \
  -H "Content-Type: application/json" \
  -d '{"recipient": "+111", "channel": "sms", "content": "Single test", "priority": "high"}')
echo $RESP | jq .
NOTIF_ID=$(echo $RESP | jq -r '.id')
echo "Notification ID: $NOTIF_ID"

echo ""
echo "=== 3. Get Notification Status ==="
sleep 2
curl -s "$API/api/v1/notifications/$NOTIF_ID" | jq '{id, status, channel, priority}'

echo ""
echo "=== 4. Cancel a Notification ==="
CANCEL_RESP=$(curl -s -X POST $API/api/v1/notifications \
  -H "Content-Type: application/json" \
  -d '{"recipient": "+222", "channel": "email", "content": "Will be cancelled", "priority": "low"}')
CANCEL_ID=$(echo $CANCEL_RESP | jq -r '.id')
curl -s -X PATCH "$API/api/v1/notifications/$CANCEL_ID/cancel" | jq '{id, status}'

echo ""
echo "=== 5. Idempotency Test ==="
echo "First request:"
curl -s -X POST $API/api/v1/notifications \
  -H "Content-Type: application/json" \
  -H "Idempotency-Key: idem-test-$(date +%s)" \
  -d '{"recipient": "+333", "channel": "push", "content": "Idempotent msg"}' | jq '{id, status}'

echo "Same key again (should return same ID):"
curl -s -X POST $API/api/v1/notifications \
  -H "Content-Type: application/json" \
  -H "Idempotency-Key: idem-test-$(date +%s)" \
  -d '{"recipient": "+333", "channel": "push", "content": "Idempotent msg"}' | jq '{id, status}'

echo ""
echo "=== 6. Batch — 10 notifications ==="
BATCH_BODY='{"notifications": ['
for i in $(seq 1 10); do
  BATCH_BODY+="$(printf '{"recipient": "+%03d", "channel": "sms", "content": "Batch msg %d", "priority": "normal"}' $i $i)"
  if [ $i -lt 10 ]; then BATCH_BODY+=','; fi
done
BATCH_BODY+=']}'

BATCH_RESP=$(curl -s -X POST $API/api/v1/notifications/batch \
  -H "Content-Type: application/json" \
  -d "$BATCH_BODY")
echo $BATCH_RESP | jq '{batch_id, count: (.notification_ids | length)}'
BATCH_ID=$(echo $BATCH_RESP | jq -r '.batch_id')

echo ""
echo "=== 7. Get Batch Status ==="
sleep 3
curl -s "$API/api/v1/notifications/batch/$BATCH_ID" | jq '{batch_id, count, statuses: [.data[].status] | group_by(.) | map({status: .[0], count: length})}'

echo ""
echo "=== 8. Batch — 100 notifications (stress test) ==="
BATCH100='{"notifications": ['
for i in $(seq 1 100); do
  CHANNELS=("sms" "email" "push")
  PRIORITIES=("high" "normal" "low")
  CH=${CHANNELS[$((i % 3))]}
  PR=${PRIORITIES[$((i % 3))]}
  BATCH100+="$(printf '{"recipient": "user%d@test.com", "channel": "%s", "content": "Stress test msg %d", "priority": "%s"}' $i $CH $i $PR)"
  if [ $i -lt 100 ]; then BATCH100+=','; fi
done
BATCH100+=']}'

echo "Sending 100 notifications..."
time BATCH100_RESP=$(curl -s -X POST $API/api/v1/notifications/batch \
  -H "Content-Type: application/json" \
  -d "$BATCH100")
BATCH100_ID=$(echo $BATCH100_RESP | jq -r '.batch_id')
echo "Batch ID: $BATCH100_ID"
echo "Count: $(echo $BATCH100_RESP | jq '.notification_ids | length')"

echo ""
echo "=== 9. List with Filters ==="
curl -s "$API/api/v1/notifications?channel=sms&status=pending&page_size=5" | jq '{count: (.data | length), next_cursor}'

echo ""
echo "=== 10. Cursor Pagination ==="
echo "Page 1:"
PAGE1=$(curl -s "$API/api/v1/notifications?page_size=3")
echo $PAGE1 | jq '{count: (.data | length), next_cursor}'
CURSOR=$(echo $PAGE1 | jq -r '.next_cursor')

if [ "$CURSOR" != "null" ]; then
  echo "Page 2:"
  curl -s "$API/api/v1/notifications?page_size=3&cursor=$CURSOR" | jq '{count: (.data | length), next_cursor}'
fi

echo ""
echo "=== 11. Template Rendering ==="
curl -s -X POST $API/api/v1/notifications \
  -H "Content-Type: application/json" \
  -d '{
    "recipient": "alice@test.com",
    "channel": "email",
    "content": "Hello {{.name}}, your code is {{.code}}",
    "priority": "high",
    "template_vars": {"name": "Alice", "code": "9876"}
  }' | jq '{id, status, content}'

echo ""
echo "=== 12. Scheduled Notification ==="
FUTURE=$(date -u -v+1H +"%Y-%m-%dT%H:%M:%SZ" 2>/dev/null || date -u -d "+1 hour" +"%Y-%m-%dT%H:%M:%SZ")
curl -s -X POST $API/api/v1/notifications \
  -H "Content-Type: application/json" \
  -d "{
    \"recipient\": \"+444\",
    \"channel\": \"sms\",
    \"content\": \"Scheduled message\",
    \"priority\": \"normal\",
    \"scheduled_at\": \"$FUTURE\"
  }" | jq '{id, status, scheduled_at}'

echo ""
echo "=== 13. Metrics ==="
sleep 5
curl -s $API/api/v1/metrics | jq .

echo ""
echo "=== 14. Validation Errors ==="
echo "Invalid channel:"
curl -s -X POST $API/api/v1/notifications \
  -H "Content-Type: application/json" \
  -d '{"recipient": "+111", "channel": "telegram", "content": "test"}' | jq .

echo "Missing recipient:"
curl -s -X POST $API/api/v1/notifications \
  -H "Content-Type: application/json" \
  -d '{"channel": "sms", "content": "test"}' | jq .

echo "Missing content:"
curl -s -X POST $API/api/v1/notifications \
  -H "Content-Type: application/json" \
  -d '{"recipient": "+111", "channel": "sms"}' | jq .

echo ""
echo "=== 15. Batch Limit (1001 — should fail) ==="
OVER='{"notifications": ['
for i in $(seq 1 1001); do
  OVER+='{"recipient": "+111", "channel": "sms", "content": "x"}'
  if [ $i -lt 1001 ]; then OVER+=','; fi
done
OVER+=']}'
curl -s -X POST $API/api/v1/notifications/batch \
  -H "Content-Type: application/json" \
  -d "$OVER" | jq .

echo ""
echo "=== DONE ==="
