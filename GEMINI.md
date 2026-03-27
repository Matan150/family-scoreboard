# 🎙️ Family Scoreboard - Project Specification

> [!IMPORTANT]
> **Collaboration Protocol:** Before updating or refactoring any existing code, the agent MUST present the proposed changes, explaining which lines will be modified and why and wait for approval via action required box. No code is to be overwritten without explicit approval from the user.

## 🎯 Goal

A "funny" multi-device home automation app that counts how many times family members are directly addressed in Hebrew (e.g., "אמא, אפשר...") while ignoring general mentions (e.g., "אמא אמרה לא").

## 🛠 Tech Stack

- **Engine:** **Go (Golang)** - High-efficiency, low-memory 24/7 background execution.
- **Speech-to-Text:** **Whisper.cpp** (Local-first) - Uses GGML models for offline Hebrew transcription with zero cloud latency.
- **Audio Capture:** **pvrecorder** - Professional-grade 16kHz microphone access.
- **Database:** **Turso (libSQL)** - SQLite-compatible with "Embedded Replicas" for local-first syncing and cloud backup.
- **Frontend:** **React** - A real-time web dashboard to display live scores.

## 🏗 Component Structure

### 1. Listener (`/listener`)

- **Voice Module:** - Captures 16-bit PCM via `pvrecorder`.
  - Normalizes audio to `float32` (dividing by 32768.0).
  - Manages a **3-second sliding window** to provide Whisper enough context for accurate Hebrew recognition.
- **Inference Module:** Uses `whisper.Context` to scan segments for target keywords: **"אמא"** (Mom) and **"איתמר"** (Itamar).
- **Data Module:** Writes "Score Events" to the local Turso replica.

### 2. Scoreboard (`/scoreboard`)

- **UI:** React-based dashboard with high-visibility counters.
- **Data:** Pulls aggregate data from Turso Cloud to show counts from all household devices.

## 🔄 Workflow & Logic

1.  **Audio Processing:** Raw mic input is converted from `int16` to `float32`.
2.  **Transcription:** Every 3 seconds (or via a sliding step), the buffer is passed to Whisper.
3.  **Keyword Detection:** The string output is parsed for specific family member names.
4.  **De-duplication:** Events are logged with a `timestamp` rounded to the second. A composite Unique Key on `(timestamp, member_id, household_id)` ensures that if multiple listeners hear the same shout, only one point is recorded.

## 🚀 Execution Phases

- [x] **Phase 1:** Basic Go script recognizing Hebrew keywords via Whisper.cpp.
- [ ] **Phase 2:** Implement a **Circular Buffer** logic to allow continuous listening without losing audio between processing chunks.
- [ ] **Phase 3:** Integrate **Turso (libSQL)** for local score persistence and cloud syncing.
- [ ] **Phase 4:** Build the **React Dashboard** for live visualization.
- [ ] **Phase 5:** **Optimization:** Implement Voice Activity Detection (VAD) to trigger Whisper only when someone is actually talking (CPU optimization).
- [ ] **Phase 6:** Deployment to Raspberry Pi (ARM64).

---

## ⚠️ Known Implementation Details

- **Sample Rate:** Must be 16,000Hz.
- **Model:** Currently using `ggml-base.bin` for a balance of speed and Hebrew accuracy.
- **CGO:** Requires `CGO_ENABLED=1` and proper linking to `libwhisper.a`.
