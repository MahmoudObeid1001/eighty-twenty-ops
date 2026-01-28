#!/bin/bash

# Configuration
API_URL="http://localhost:3000/api"
CLASS_KEY="L2|Sun/Wed|07:30:00|1"
URL_ENCODED_CLASS_KEY="L2%7CSun%2FWed%7C07%3A30%3A00%7C1"
LEAD_ID="06f47b93-1d15-47e3-817a-eb3fbb2f5fc0"
ADMIN_EMAIL="admin@eightytwenty.test"
ADMIN_PASS="admin123"
COOKIE_JAR="cookies.txt"

# Colors
GREEN='\033[0;32m'
RED='\033[0;31m'
NC='\033[0m'

echo "--- Starting Verification ---"

# 0. Kill/Build/Start Server
echo -e "\n[0] Rebuilding and Starting Server..."
pkill server || true
go build -o server ./cmd/server/main.go
if [ $? -ne 0 ]; then echo -e "${RED}Build Failed${NC}"; exit 1; fi
./server > /dev/null 2>&1 &
SERVER_PID=$!
echo "Server started (PID: $SERVER_PID)"
sleep 3 # Wait for startup

# 1. Login
echo -e "\n[1] Logging in as Admin..."
rm -f $COOKIE_JAR
# We need to capture the cookie!
# Using -c to save cookies.
HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" -c $COOKIE_JAR -X POST "$API_URL/../login" \
  -H "Content-Type: application/json" \
  -d "{\"email\":\"$ADMIN_EMAIL\", \"password\":\"$ADMIN_PASS\"}")

if [ "$HTTP_CODE" -eq 200 ] || [ "$HTTP_CODE" -eq 303 ]; then
    echo -e "${GREEN}Login Successful (HTTP $HTTP_CODE)${NC}"
else
    echo -e "${RED}Login Failed (HTTP $HTTP_CODE)${NC}"
    exit 1
fi

# 2. Reset Session & Attendance (Clean State)
echo -e "\n[2] Resetting Session 1 & Attendance..."
# We can't easily reset via API, so we'll assume we can move forward or use psql if needed.
# For now, let's just complete Session 1.

# 3. Complete Session 1 (Mentor Action)
echo -e "\n[3] Completing Session 1..."
# Determine Session ID for Session 1
SESSION_1_ID=$(psql "postgres://postgres:postgres@localhost:5432/eighty_twenty_ops?sslmode=disable" -t -c "SELECT id FROM class_sessions WHERE class_key='$CLASS_KEY' AND session_number=1;" | xargs)
echo "Session 1 ID: $SESSION_1_ID"

curl -s -b $COOKIE_JAR -X POST "$API_URL/classes/$URL_ENCODED_CLASS_KEY/sessions/1/complete" \
    -H "Content-Type: application/json"
echo -e "\nSession 1 Marked Complete"

# 4. Check Badge Count (Student Success Action)
echo -e "\n[4] Checking Badge Count..."
BADGE_COUNT=$(curl -s -b $COOKIE_JAR "$API_URL/student-success/class?class_key=$URL_ENCODED_CLASS_KEY" | grep -o '"completedSessionsCount":[0-9]*' | grep -o '[0-9]*')
echo "Completed Sessions Count: $BADGE_COUNT"
if [ "$BADGE_COUNT" -ge 1 ]; then echo -e "${GREEN}Badge updated correctly${NC}"; else echo -e "${RED}Badge failed to update${NC}"; fi

# 5. Mark Student Absent in Session 2
echo -e "\n[5] Marking Student Absent in Session 2..."
SESSION_2_ID=$(psql "postgres://postgres:postgres@localhost:5432/eighty_twenty_ops?sslmode=disable" -t -c "SELECT id FROM class_sessions WHERE class_key='$CLASS_KEY' AND session_number=2;" | xargs)
curl -s -b $COOKIE_JAR -X POST "$API_URL/attendance" \
  -H "Content-Type: application/json" \
  -d "{\"session_id\":\"$SESSION_2_ID\", \"lead_id\":\"$LEAD_ID\", \"status\":\"ABSENT\", \"class_key\":\"$CLASS_KEY\", \"notes\":\"Test Absence\"}"
echo -e "\nMarked Absent"

# 6. Verify Absence Feed
echo -e "\n[6] Verifying Absence Feed..."
FEED=$(curl -s -b $COOKIE_JAR "$API_URL/student-success/class/absence-feed?class_key=$URL_ENCODED_CLASS_KEY&filter=absent")
if echo "$FEED" | grep -q "$LEAD_ID"; then
  echo -e "${GREEN}Absence found in feed${NC}"
else
  echo -e "${RED}Absence NOT found in feed${NC}"
  echo "Feed Content: $FEED"
fi

# 7. Add Follow-up Note + Status
echo -e "\n[7] Adding Follow-up Note & Setting Status to CONTACTED..."
curl -s -b $COOKIE_JAR -X POST "$API_URL/student-success/followups" \
  -H "Content-Type: application/json" \
  -d "{\"class_key\":\"$CLASS_KEY\", \"lead_id\":\"$LEAD_ID\", \"session_number\":2, \"note\":\"Called student, verified.\", \"status\":\"contacted\"}"
echo -e "\nFollow-up Added"

# 8. Verify Follow-up in Feed
echo -e "\n[8] Verifying Follow-up in Feed..."
FEED=$(curl -s -b $COOKIE_JAR "$API_URL/student-success/class/absence-feed?class_key=$URL_ENCODED_CLASS_KEY")
if echo "$FEED" | grep -q "Called student, verified"; then
  echo -e "${GREEN}Note found in feed${NC}"
else
  echo -e "${RED}Note NOT found in feed${NC}"
fi

# Get Follow-up ID for resolution (using PSQL to save parsing complexity in bash, typically frontend has it from feed)
FOLLOWUP_ID=$(psql "postgres://postgres:postgres@localhost:5432/eighty_twenty_ops?sslmode=disable" -t -c "SELECT id FROM followups WHERE class_key='$CLASS_KEY' AND lead_id='$LEAD_ID' AND session_number=2;" | xargs)
echo "Follow-up ID: $FOLLOWUP_ID"

# 9. Resolve Follow-up
echo -e "\n[9] Resolving Follow-up..."
curl -s -b $COOKIE_JAR -X POST "$API_URL/absence-cases/$FOLLOWUP_ID/resolve" \
  -H "Content-Type: application/json" \
  -d "{}"
echo -e "\nResolved"

# 10. Verify Removed from 'Unresolved' Filter
echo -e "\n[10] Verifying removal from 'Unresolved' filter..."
FEED=$(curl -s -b $COOKIE_JAR "$API_URL/student-success/class/absence-feed?class_key=$URL_ENCODED_CLASS_KEY&filter=unresolved")
if echo "$FEED" | grep -q "$FOLLOWUP_ID"; then
  echo -e "${RED}Error: Context still found in unresolved filter${NC}"
else
  echo -e "${GREEN}Verified: Not in unresolved filter${NC}"
fi

echo -e "\n--- Verification Complete ---"
