# 🎙️ Family Scoreboard

A "funny" multi-device home automation app that counts how many times family members are directly addressed in Hebrew (e.g., "אמא, אפשר...") while ignoring general mentions (e.g., "אמא אמרה לא").

## 📁 Project Structure

- **`/listener`**: Go application that captures audio and performs keyword detection using Whisper.cpp.
- **`/scoreboard`**: React dashboard to visualize scores in real-time.
- **`/db`**: Database schema and migration files (Turso/libSQL).

## 🚀 Getting Started

### 1. Database Setup
The project uses GORM with SQLite (or Turso). The database schema is automatically created and migrated when you run the listener. No manual SQL steps are required!

### 2. Run the Listener (Go)
Requires `CGO_ENABLED=1` and a compiled `libwhisper.a`.
```bash
cd listener
go run main.go
```

### 3. Run the Scoreboard (React)
```bash
cd scoreboard
npm install
npm run dev
```

## 🛠 Tech Stack
- **Go** (Efficiency)
- **Whisper.cpp** (Local-first Speech-to-Text)
- **Turso (libSQL)** (Local-first DB)
- **React + Vite** (Real-time Dashboard)
- **pvrecorder** (Audio Capture)

## 🔄 Current Progress
See [GEMINI.md](./GEMINI.md) for detailed execution phases and progress.
