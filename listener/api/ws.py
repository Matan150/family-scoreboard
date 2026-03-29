import json

from fastapi import APIRouter, WebSocket, WebSocketDisconnect

from db import SessionLocal, get_scores
from hub import hub

router = APIRouter()


@router.websocket("/ws")
async def ws_endpoint(websocket: WebSocket):
    await hub.connect(websocket)
    db = SessionLocal()
    try:
        scores = get_scores(db)
        await websocket.send_text(json.dumps({"type": "score_update", "data": scores}))
    finally:
        db.close()
    try:
        while True:
            await websocket.receive_text()
    except WebSocketDisconnect:
        await hub.disconnect(websocket)
