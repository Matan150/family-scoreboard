import logging
import os
import threading
import time
from datetime import datetime, timezone
from pathlib import Path

import numpy as np

from db import ScoreEvent, SessionLocal, broadcast_scores_sync, get_all_names_lower
from detection import is_direct_address
from hub import hub

# ── Constants ──────────────────────────────────────────────────────────────────

SAMPLE_RATE = 16000
BUFFER_SECONDS = 4
STEP_SECONDS = 2
VAD_THRESHOLD = 0.008

# ── Shared sequential buffer (reader → transcriber) ────────────────────────────
#
# The reader appends to _audio_samples continuously.
# The transcriber has its own read pointer (head) and processes forward in order —
# audio that arrives during transcription is queued and processed next, never lost.
#
# _trim_offset tracks how many samples have been deleted from the front of the list,
# so that head (an absolute sample count) can be converted to a list index:
#   list_index = head - _trim_offset

_audio_samples: list[float] = []
_trim_offset: int = 0
_audio_lock = threading.Lock()
_audio_event = threading.Event()   # wakes transcriber when new samples arrive


# ── Model discovery ────────────────────────────────────────────────────────────

def find_model() -> str:
    if path := os.environ.get("WHISPER_MODEL"):
        return path
    script_dir = Path(__file__).parent
    candidates = [
        "ggml-ivrit-ai-whisper-turbo.bin",
        "ivrit-ai-whisper-turbo.bin",
        "ivrit-turbo.bin",
        "ggml-ivrit-turbo.bin",
        "ggml-small-he.bin",
    ]
    for c in candidates:
        for base in (Path("."), script_dir, script_dir / "whisper.cpp" / "models"):
            p = base / c
            if p.exists():
                return str(p)
    return ""


# ── Thread A: microphone reader ────────────────────────────────────────────────

def _reader_thread():
    try:
        from pvrecorder import PvRecorder
    except ImportError:
        print("⚠️  pvrecorder not installed. Run: pip install pvrecorder")
        return

    recorder = PvRecorder(frame_length=512)
    recorder.start()
    print("🎤 Microphone is ON — listening in Hebrew")

    while True:
        try:
            pcm = recorder.read()
            samples = [s / 32768.0 for s in pcm]
            with _audio_lock:
                _audio_samples.extend(samples)
            _audio_event.set()  # wake the transcriber
        except Exception as e:
            logging.error(f"Reader error: {e}")


# ── Thread B: transcriber ──────────────────────────────────────────────────────

def _transcriber_thread(model):
    global _trim_offset

    window_size = SAMPLE_RATE * BUFFER_SECONDS   # 64 000 samples = 4s
    step_size   = SAMPLE_RATE * STEP_SECONDS     # 32 000 samples = 2s
    head = 0  # absolute sample count — next window starts here

    recent_events: dict[str, float] = {}
    last_text = ""

    while True:
        # Block until there are enough samples ahead of head
        while True:
            with _audio_lock:
                available = (_trim_offset + len(_audio_samples)) - head
            if available >= window_size:
                break
            _audio_event.wait(timeout=0.5)
            _audio_event.clear()

        # Snapshot the window
        with _audio_lock:
            start = head - _trim_offset
            window = _audio_samples[start: start + window_size]

        # VAD — skip silent windows without transcribing
        if max(abs(s) for s in window) <= VAD_THRESHOLD:
            head += step_size
            _maybe_trim(head)
            continue

        print("🧠 מעבד...", end="", flush=True)
        t0 = time.time()

        try:
            segments, _ = model.transcribe(
                np.array(window, dtype=np.float32),
                language="he",
                beam_size=3,
                vad_filter=True,
                vad_parameters=dict(min_silence_duration_ms=300),
                initial_prompt="משפחה, ניקוד, ילדים, אמא, אבא",
            )
            text = " ".join(seg.text for seg in segments).strip()
        except Exception as e:
            logging.error(f"Transcription error: {e}")
            head += step_size
            _maybe_trim(head)
            continue

        print(f" סיים ({time.time() - t0:.2f}s)")

        # Advance pointer by one step (2s overlap kept for sentence continuity)
        head += step_size
        _maybe_trim(head)

        # Skip if same as previous window (overlapping content, no new speech)
        if not text or text == last_text:
            continue
        last_text = text

        print(f">> שמעתי: {text}")

        db = SessionLocal()
        try:
            name_map = get_all_names_lower(db)
            for name, member_id in name_map.items():
                if name in text.lower():
                    if is_direct_address(text, name):
                        key = str(member_id)
                        last = recent_events.get(key)
                        if last and (time.time() - last) < 30:
                            print(f"⏭️  כפילות מדולגת עבור member {member_id}")
                            continue
                        recent_events[key] = time.time()
                        db.add(ScoreEvent(
                            member_id=member_id,
                            timestamp=datetime.now(timezone.utc),
                            context_text=text,
                        ))
                        db.commit()
                        print(f"🔥 [נקודה!] member {member_id} — {text}")
                        broadcast_scores_sync(hub)
                    else:
                        print(f"📌 אזכור (לא פנייה ישירה): {text}")
        finally:
            db.close()


def _maybe_trim(head: int):
    """Delete samples well behind the read pointer so the list stays bounded."""
    global _trim_offset
    keep_from = max(0, head - SAMPLE_RATE * 2)  # keep 2s of history behind head
    drop = keep_from - _trim_offset
    if drop <= 0:
        return
    with _audio_lock:
        del _audio_samples[:drop]
        _trim_offset += drop


# ── Entry point ────────────────────────────────────────────────────────────────

def run_listener():
    try:
        from faster_whisper import WhisperModel
    except ImportError:
        print("⚠️  faster-whisper not installed. Run: pip install faster-whisper")
        return

    print("📂 Loading Hebrew model (first run will download ~800MB, please wait)...")
    model = WhisperModel(
        "ivrit-ai/whisper-large-v3-turbo-ct2",
        device="cpu",
        compute_type="int8",
        cpu_threads=4,
        num_workers=1,
    )
    print("✅ Hebrew model loaded!")

    threading.Thread(target=_reader_thread, daemon=True).start()
    threading.Thread(target=_transcriber_thread, args=(model,), daemon=True).start()
