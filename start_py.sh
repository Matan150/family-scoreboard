#!/usr/bin/env bash

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

BACKEND_PID=""
FRONTEND_PID=""

cleanup() {
  echo ""
  echo "Shutting down..."

  # Kill the processes we started
  [ -n "$BACKEND_PID" ]  && kill "$BACKEND_PID"  2>/dev/null
  [ -n "$FRONTEND_PID" ] && kill "$FRONTEND_PID" 2>/dev/null

  # Also kill any leftover processes on our ports
  for port in 8080 5173 5174 5175 5176 5177 5178 5179; do
    pid=$(netstat -ano 2>/dev/null | grep ":$port " | grep LISTENING | awk '{print $5}' | head -1)
    [ -n "$pid" ] && taskkill //F //PID "$pid" 2>/dev/null
  done

  wait "$BACKEND_PID" "$FRONTEND_PID" 2>/dev/null
  exit 0
}

trap cleanup INT TERM EXIT

# Kill leftover processes from previous runs before starting
echo "Cleaning up old processes..."
for port in 8080 5173 5174 5175 5176 5177 5178 5179; do
  pid=$(netstat -ano 2>/dev/null | grep ":$port " | grep LISTENING | awk '{print $5}' | head -1)
  [ -n "$pid" ] && taskkill //F //PID "$pid" 2>/dev/null && echo "  Killed leftover on port $port"
done

echo "Starting backend (Python)..."
cd "$SCRIPT_DIR/listener"

if [ -f "$SCRIPT_DIR/.venv/Scripts/activate" ]; then
  source "$SCRIPT_DIR/.venv/Scripts/activate"
elif [ -f "$SCRIPT_DIR/.venv/bin/activate" ]; then
  source "$SCRIPT_DIR/.venv/bin/activate"
fi

python main.py &
BACKEND_PID=$!

echo "Starting frontend..."
cd "$SCRIPT_DIR/scoreboard"
npm run dev &
FRONTEND_PID=$!

echo "Backend PID: $BACKEND_PID | Frontend PID: $FRONTEND_PID"
echo "Press Ctrl+C to stop."

while true; do
  if ! kill -0 "$BACKEND_PID" 2>/dev/null; then
    echo "Backend exited unexpectedly."
    cleanup
  fi
  if ! kill -0 "$FRONTEND_PID" 2>/dev/null; then
    echo "Frontend exited unexpectedly."
    cleanup
  fi
  sleep 1
done