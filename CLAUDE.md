# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Family Scoreboard is a Hebrew home automation app that counts how often family members are *directly addressed* (e.g., "אמא, אפשר לאכול?"). It uses local speech-to-text to detect addresses in real time.

Two components:
- **`listener/`** — Go backend: microphone capture → Whisper.cpp (Hebrew model) → direct-address detection → SQLite → HTTP API + WebSocket
- **`scoreboard/`** — React + TypeScript frontend: live scoreboard + member/alias management (RTL Hebrew UI)

## Commands

### Frontend (`scoreboard/`)
```bash
cd scoreboard
npm install
npm run dev        # Vite dev server (HMR), connects to backend at localhost:8080
npm run build      # tsc + vite production build
npm run lint
```

### Backend (`listener/`)
```bash
cd listener
go get github.com/gorilla/websocket   # first time only — adds to go.sum
go mod tidy
go build -o listener.exe .
./listener.exe      # requires model .bin in current directory + mic access
```

The backend server runs on `:8080`. Set `WHISPER_MODEL=/path/to/model.bin` to override model path.

## Architecture

### Go Backend (`listener/main.go`)

**Database models:**
- `Member` — primary name + display name
- `MemberAlias` — additional nicknames per member (e.g., אמא → אימוש, מאמה)
- `ScoreEvent` — each direct-address detection event with context text

**HTTP API (all CORS-enabled):**
| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/scores` | Live scores for all members |
| GET | `/api/members` | Members with their aliases |
| POST | `/api/members` | Create member `{name, display_name, aliases[]}` |
| PUT | `/api/members/{id}` | Update member + replace aliases |
| DELETE | `/api/members/{id}` | Delete member and all their events |
| DELETE | `/api/members/{id}/reset` | Reset score only |
| GET | `/ws` | WebSocket — broadcasts `{type:"score_update", data:[...]}` on every score change |

**Audio pipeline:**
1. `pvrecorder` at 16kHz, frame size 512
2. VAD: skips frames with max amplitude < 0.008
3. 4-second sliding window buffer, steps 2 seconds
4. Whisper.cpp — language: `he`, model loaded from `findModel()` (checks common ivrit filenames)
5. For each transcribed text, checks all member names + aliases (case-insensitive)
6. `isDirectAddress()` decides if it's a point; deduplicates within 30-second window

**Direct-address detection heuristic (`isDirectAddress`):**
Scores each name occurrence in the transcription:
- +3 if name is at the start of utterance
- +3 if name is at the end (vocative) → immediately returns `true`
- +2 for each of the first 4 words after name that appear in `directWords` (e.g., אפשר, תן, מה, בבקשה, 2nd-person imperatives)
- -3 for each of the first 4 words that appear in `indirectWords` (3rd-person past-tense verbs: אמרה, עשה, הלך, של…)
- Returns `true` if total score > 0

**Model detection (`findModel`):**
Checks for these filenames in order: `ggml-ivrit-ai-whisper-turbo.bin`, `ivrit-ai-whisper-turbo.bin`, `ivrit-turbo.bin`, `ggml-ivrit-turbo.bin`, `ggml-small-he.bin`. Override with `WHISPER_MODEL` env var.

### React Frontend (`scoreboard/src/`)

**Views:**
- **Scoreboard** (default) — live-updating cards sorted by score; leading card highlighted; 👑 on first place; per-card reset button
- **Members** — CRUD for participants; each member has a primary name (used for detection), display name (shown on board), and any number of aliases (all detected equally)

**Real-time:** WebSocket to `ws://localhost:8080/ws` with auto-reconnect every 3 seconds. On connect, server immediately sends the current score snapshot.

**UI:** RTL Hebrew (`dir="rtl"`), dark theme, no external component library.

## Development Status
- [x] Phase 1: Basic Go keyword detection via Whisper.cpp
- [x] Phase 2: Hebrew model support (ivrit-ai turbo), RTL frontend, member alias management, HTTP API + WebSocket
- [ ] Phase 3: Turso/libSQL cloud sync
- [ ] Phase 5: VAD CPU optimization
- [ ] Phase 6: Raspberry Pi ARM64 deployment

## Key Notes
- The whisper.cpp directory is a CGo submodule — must be built before compiling the Go backend
- Sample rate is fixed at **16,000 Hz** (Whisper requirement)
- The `go.mod` has a `replace` directive pointing whisper bindings to the local `./whisper.cpp/bindings/go`
- Detection targets Hebrew speech; `directWords`/`indirectWords` lists in `main.go` drive the heuristic and can be tuned
