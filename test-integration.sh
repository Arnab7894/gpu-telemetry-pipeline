#!/bin/bash

# Test script for queue service with multiple collectors
# This demonstrates competing consumers pattern with message locking

set -e

echo "=== GPU Telemetry Pipeline Integration Test ==="
echo

# Cleanup function
cleanup() {
    echo
    echo "Cleaning up processes..."
    kill $QUEUE_PID $COLLECTOR1_PID $COLLECTOR2_PID $COLLECTOR3_PID $STREAMER_PID 2>/dev/null || true
    wait 2>/dev/null || true
}

trap cleanup EXIT

# Start queue service
echo "Starting Queue Service..."
./bin/queueservice -port=8080 > logs/queueservice.log 2>&1 &
QUEUE_PID=$!
sleep 2

# Check if queue service is running
if ! ps -p $QUEUE_PID > /dev/null; then
    echo "❌ Queue service failed to start"
    cat logs/queueservice.log
    exit 1
fi

echo "✅ Queue service started (PID: $QUEUE_PID)"

# Check health endpoint
if curl -s http://localhost:8080/health | grep -q "healthy"; then
    echo "✅ Queue service health check passed"
else
    echo "❌ Queue service health check failed"
    exit 1
fi

# Start 3 collector instances
echo
echo "Starting 3 Collector instances..."

export QUEUE_SERVICE_URL=http://localhost:8080

./bin/collector -instance-id=collector-1 > logs/collector-1.log 2>&1 &
COLLECTOR1_PID=$!
sleep 1

./bin/collector -instance-id=collector-2 > logs/collector-2.log 2>&1 &
COLLECTOR2_PID=$!
sleep 1

./bin/collector -instance-id=collector-3 > logs/collector-3.log 2>&1 &
COLLECTOR3_PID=$!
sleep 2

echo "✅ Collectors started:"
echo "   - collector-1 (PID: $COLLECTOR1_PID)"
echo "   - collector-2 (PID: $COLLECTOR2_PID)"
echo "   - collector-3 (PID: $COLLECTOR3_PID)"

# Start streamer
echo
echo "Starting Streamer..."
./bin/streamer -csv-path=test_sample.csv -instance-id=streamer-1 -interval=100ms -loop=false > logs/streamer.log 2>&1 &
STREAMER_PID=$!

# Wait for streamer to finish
echo "Waiting for streamer to complete..."
wait $STREAMER_PID
echo "✅ Streamer completed"

# Give collectors time to process messages
sleep 5

# Check queue statistics
echo
echo "=== Queue Statistics ==="
curl -s http://localhost:8080/api/v1/queue/stats | jq '.'

# Stop collectors
echo
echo "Stopping collectors..."
kill -SIGINT $COLLECTOR1_PID $COLLECTOR2_PID $COLLECTOR3_PID

# Wait for collectors to shut down gracefully
sleep 2

# Analyze results
echo
echo "=== Test Results ==="
echo
echo "Collector 1 Statistics:"
grep "Shutdown complete" logs/collector-1.log | tail -1 | jq '.'
echo
echo "Collector 2 Statistics:"
grep "Shutdown complete" logs/collector-2.log | tail -1 | jq '.'
echo
echo "Collector 3 Statistics:"
grep "Shutdown complete" logs/collector-3.log | tail -1 | jq '.'
echo
echo "Streamer Statistics:"
grep "Shutdown complete" logs/streamer.log | tail -1 | jq '.'

# Calculate totals
COLLECTOR1_PROCESSED=$(grep "Shutdown complete" logs/collector-1.log | tail -1 | jq -r '.messages_processed // 0')
COLLECTOR2_PROCESSED=$(grep "Shutdown complete" logs/collector-2.log | tail -1 | jq -r '.messages_processed // 0')
COLLECTOR3_PROCESSED=$(grep "Shutdown complete" logs/collector-3.log | tail -1 | jq -r '.messages_processed // 0')
TOTAL_PROCESSED=$((COLLECTOR1_PROCESSED + COLLECTOR2_PROCESSED + COLLECTOR3_PROCESSED))

ROWS_SENT=$(grep "Shutdown complete" logs/streamer.log | tail -1 | jq -r '.rows_sent // 0')

echo
echo "=== Summary ==="
echo "Messages published by streamer: $ROWS_SENT"
echo "Total messages processed by collectors: $TOTAL_PROCESSED"
echo "  - Collector 1: $COLLECTOR1_PROCESSED"
echo "  - Collector 2: $COLLECTOR2_PROCESSED"
echo "  - Collector 3: $COLLECTOR3_PROCESSED"

# Verify results
if [ "$TOTAL_PROCESSED" -eq "$ROWS_SENT" ]; then
    echo
    echo "✅ SUCCESS: All messages processed exactly once (competing consumers working!)"
    
    # Check if load was distributed
    ACTIVE_COLLECTORS=0
    [ "$COLLECTOR1_PROCESSED" -gt 0 ] && ACTIVE_COLLECTORS=$((ACTIVE_COLLECTORS + 1))
    [ "$COLLECTOR2_PROCESSED" -gt 0 ] && ACTIVE_COLLECTORS=$((ACTIVE_COLLECTORS + 1))
    [ "$COLLECTOR3_PROCESSED" -gt 0 ] && ACTIVE_COLLECTORS=$((ACTIVE_COLLECTORS + 1))
    
    if [ "$ACTIVE_COLLECTORS" -ge 2 ]; then
        echo "✅ Load balancing working: $ACTIVE_COLLECTORS collectors received messages"
    else
        echo "⚠️  Load balancing issue: Only $ACTIVE_COLLECTORS collector(s) received messages"
    fi
else
    echo
    echo "❌ FAILURE: Message count mismatch!"
    echo "   Expected: $ROWS_SENT"
    echo "   Got: $TOTAL_PROCESSED"
    exit 1
fi

echo
echo "=== Test Completed Successfully ==="
