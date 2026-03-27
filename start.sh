#!/usr/bin/env bash

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

BACKEND_PID=""
FRONTEND_PID=""

cleanup() {
  echo ""
  echo "Shutting down..."
  [ -n "$BACKEND_PID" ]  && kill "$BACKEND_PID"  2>/dev/null
  [ -n "$FRONTEND_PID" ] && kill "$FRONTEND_PID" 2>/dev/null
  wait "$BACKEND_PID" "$FRONTEND_PID" 2>/dev/null
  exit 0
}

trap cleanup INT TERM

# Convert WHISPER_MODEL to an absolute Windows path for listener.exe
if [ -n "$WHISPER_MODEL" ]; then
  # Normalize backslashes to forward slashes
  MODEL="${WHISPER_MODEL//\\//}"
  # Windows drive letter (C:/...) → Unix (/c/...)
  if [[ "$MODEL" =~ ^([a-zA-Z]):/(.*) ]]; then
    MODEL="/${BASH_REMATCH[1],,}/${BASH_REMATCH[2]}"
  fi
  # Relative → absolute (relative to where the script was invoked)
  if [[ "$MODEL" != /* ]]; then
    MODEL="$(pwd)/$MODEL"
  fi
  # Convert back to Windows absolute path (required by listener.exe)
  export WHISPER_MODEL="$(cygpath -w "$MODEL")"
fi

echo "Starting backend..."
cd "$SCRIPT_DIR/listener"
./listener.exe &
BACKEND_PID=$!

echo "Starting frontend..."
cd "$SCRIPT_DIR/scoreboard"
npm run dev &
FRONTEND_PID=$!

echo "Backend PID: $BACKEND_PID | Frontend PID: $FRONTEND_PID"
echo "Press Ctrl+C to stop."

# Poll both processes; exit if either dies unexpectedly
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
