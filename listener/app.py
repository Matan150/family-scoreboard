import asyncio
import threading
from contextlib import asynccontextmanager

from fastapi import FastAPI
from fastapi.middleware.cors import CORSMiddleware

from hub import hub


@asynccontextmanager
async def lifespan(app: FastAPI):
    from audio import run_listener
    hub.set_loop(asyncio.get_running_loop())
    threading.Thread(target=run_listener, daemon=True).start()
    yield


app = FastAPI(lifespan=lifespan)
app.add_middleware(
    CORSMiddleware,
    allow_origins=["*"],
    allow_methods=["*"],
    allow_headers=["*"],
)

from api.scores import router as scores_router
from api.members import router as members_router
from api.ws import router as ws_router

app.include_router(scores_router)
app.include_router(members_router)
app.include_router(ws_router)
