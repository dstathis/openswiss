#!/bin/bash
# Test workflow for tournament management

PORT=8888
ADMIN_PASS="admin123"

# Start server
timeout 30 ./openswiss -port $PORT -admin-password $ADMIN_PASS > /tmp/test-server.log 2>&1 &
SERVER_PID=$!
sleep 2

echo "=== Testing Full Tournament Workflow ==="
echo ""

# Get admin session
echo "1. Getting admin session..."
ADMIN_COOKIE=$(curl -s -c /tmp/cookies.txt -X POST http://localhost:$PORT/login -d "password=$ADMIN_PASS" -i | grep -i "set-cookie" | awk '{print $2}' | cut -d';' -f1)
if [ -z "$ADMIN_COOKIE" ]; then
    ADMIN_COOKIE="session=$(grep session /tmp/cookies.txt | awk '{print $7}')"
fi
echo "   Admin cookie: $ADMIN_COOKIE"

# Register players
echo ""
echo "2. Registering players..."
for player in Alice Bob Charlie; do
    STATUS=$(curl -s -w "%{http_code}" -X POST http://localhost:$PORT/register -d "name=$player" -o /dev/null)
    echo "   Registered $player: $STATUS"
done
sleep 1

# Check pending players
echo ""
echo "3. Checking pending players..."
PENDING=$(curl -s http://localhost:$PORT/admin/dashboard -H "Cookie: $ADMIN_COOKIE" | grep -o 'TestPlayer\|Alice\|Bob\|Charlie' | head -5)
echo "   Found pending: $PENDING"

# Accept players
echo ""
echo "4. Accepting players..."
for player in Alice Bob Charlie; do
    STATUS=$(curl -s -w "%{http_code}" -X POST http://localhost:$PORT/admin/accept -d "name=$player" -H "Cookie: $ADMIN_COOKIE" -o /dev/null -L)
    echo "   Accepted $player: $STATUS"
    sleep 0.5
done

# Start tournament
echo ""
echo "5. Starting tournament..."
STATUS=$(curl -s -w "%{http_code}" -X POST http://localhost:$PORT/admin/start -H "Cookie: $ADMIN_COOKIE" -o /dev/null -L)
echo "   Start tournament: $STATUS"

# Create pairings
echo ""
echo "6. Creating pairings..."
STATUS=$(curl -s -w "%{http_code}" -X POST http://localhost:$PORT/admin/pair -d "allow_repair=false" -H "Cookie: $ADMIN_COOKIE" -o /dev/null -L)
echo "   Create pairings: $STATUS"

# Check pairings
echo ""
echo "7. Checking pairings..."
PAIRINGS=$(curl -s http://localhost:$PORT/pairings | grep -o "Alice\|Bob\|Charlie\|Bye" | head -5)
echo "   Found in pairings: $PAIRINGS"

# Kill server
kill $SERVER_PID 2>/dev/null
wait $SERVER_PID 2>/dev/null || true

echo ""
echo "=== Test Complete ==="



