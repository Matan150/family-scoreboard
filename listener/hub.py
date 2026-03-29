import asyncio
from typing import Set

from fastapi import WebSocket


class Hub:
    def __init__(self):
        self.clients: Set[WebSocket] = set()
        self._lock = asyncio.Lock()
        self._loop: asyncio.AbstractEventLoop | None = None

    def set_loop(self, loop: asyncio.AbstractEventLoop):
        self._loop = loop

    async def connect(self, ws: WebSocket):
        await ws.accept()
        async with self._lock:
            self.clients.add(ws)

    async def disconnect(self, ws: WebSocket):
        async with self._lock:
            self.clients.discard(ws)

    async def broadcast(self, message: str):
        async with self._lock:
            dead: Set[WebSocket] = set()
            for ws in self.clients:
                try:
                    await ws.send_text(message)
                except Exception:
                    dead.add(ws)
            self.clients -= dead

    def broadcast_from_thread(self, message: str):
        """Thread-safe broadcast for use from the audio listener thread."""
        if self._loop and not self._loop.is_closed():
            asyncio.run_coroutine_threadsafe(self.broadcast(message), self._loop)


hub = Hub()
