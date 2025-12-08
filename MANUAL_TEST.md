#!/bin/bash

# Simple manual test - start each service one by one

echo "1. Start queue service..."
echo "   ./bin/queueservice"
echo
echo "2. In another terminal, start a collector:"
echo "   export QUEUE_SERVICE_URL=http://localhost:8080"
echo "   ./bin/collector -instance-id=collector-1"
echo
echo "3. In another terminal, start another collector:"
echo "   export QUEUE_SERVICE_URL=http://localhost:8080"
echo "   ./bin/collector -instance-id=collector-2"
echo
echo "4. In another terminal, publish some messages:"
echo "   export QUEUE_SERVICE_URL=http://localhost:8080"
echo "   ./bin/streamer -csv-path=test_sample.csv -instance-id=streamer-1 -loop=false"
echo
echo "5. Check queue stats:"
echo "   curl http://localhost:8080/api/v1/queue/stats | jq ."
echo
echo "6. Check health:"
echo "   curl http://localhost:8080/health | jq ."
echo
echo "Expected behavior:"
echo "- Messages should be distributed across collectors (competing consumers)"
echo "- Each message processed by EXACTLY ONE collector"
echo "- No duplicate processing"
