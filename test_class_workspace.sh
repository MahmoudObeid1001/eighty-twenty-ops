#!/bin/bash
# Test script for GET /api/class-workspace endpoint
# Usage: ./test_class_workspace.sh [class_key]
# Example: ./test_class_workspace.sh "L1|Sat/Tues|10:00:00|1"

CLASS_KEY="${1:-L1|Sat/Tues|10:00:00|1}"
ENCODED_KEY=$(echo "$CLASS_KEY" | sed 's/|/%7C/g' | sed 's/\//%2F/g' | sed 's/:/%3A/g')

echo "Testing GET /api/class-workspace?class_key=$ENCODED_KEY"
echo "=========================================="
echo ""

# Test without auth (should return 401 or 403)
echo "1. Test without authentication (should fail):"
curl -s -w "\nHTTP Status: %{http_code}\n" \
  "http://localhost:3000/api/class-workspace?class_key=$ENCODED_KEY" | head -20
echo ""
echo ""

# Test with auth (requires valid session cookie)
echo "2. Test with authentication (requires valid session cookie):"
echo "   First, login and get session cookie, then:"
echo "   curl -b cookies.txt -w '\nHTTP Status: %{http_code}\n' \\"
echo "     'http://localhost:3000/api/class-workspace?class_key=$ENCODED_KEY'"
echo ""
echo "   Or use browser DevTools Network tab to copy cookie from a logged-in session"
echo ""

echo "Expected response (when authenticated):"
echo '  {'
echo '    "class": {'
echo '      "class_key": "...",'
echo '      "level": 1,'
echo '      "days": "Sat/Tues",'
echo '      "time": "10:00:00",'
echo '      "class_number": 1'
echo '    },'
echo '    "sessionsCount": 0,'
echo '    "totalSessions": 8,'
echo '    "students": [...]'
echo '  }'
echo ""
