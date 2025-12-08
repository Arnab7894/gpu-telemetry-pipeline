#!/bin/bash

# Script to demonstrate running multiple streamer instances in parallel

echo "Starting 3 streamer instances..."

# Start instance 1
INSTANCE_ID=streamer-1 STREAM_INTERVAL=150ms LOOP_MODE=false ./bin/streamer \
  -csv-path="../dcgm_metrics_20250718_134233 (1)(1).csv" \
  -instance-id=streamer-1 \
  -interval=150ms \
  -loop=false &
PID1=$!

# Start instance 2  
INSTANCE_ID=streamer-2 STREAM_INTERVAL=200ms LOOP_MODE=false ./bin/streamer \
  -csv-path="../dcgm_metrics_20250718_134233 (1)(1).csv" \
  -instance-id=streamer-2 \
  -interval=200ms \
  -loop=false &
PID2=$!

# Start instance 3
INSTANCE_ID=streamer-3 STREAM_INTERVAL=100ms LOOP_MODE=false ./bin/streamer \
  -csv-path="../dcgm_metrics_20250718_134233 (1)(1).csv" \
  -instance-id=streamer-3 \
  -interval=100ms \
  -loop=false &
PID3=$!

echo "Started 3 instances:"
echo "  - streamer-1 (PID $PID1) at 150ms interval"
echo "  - streamer-2 (PID $PID2) at 200ms interval"  
echo "  - streamer-3 (PID $PID3) at 100ms interval"
echo ""
echo "Press Ctrl+C to stop all instances"

# Wait for all processes
wait $PID1 $PID2 $PID3

echo "All streamers finished"
