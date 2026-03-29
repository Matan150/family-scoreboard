from fastapi import APIRouter

from db import SessionLocal, get_scores

router = APIRouter()


@router.get("/api/scores")
def api_scores():
    db = SessionLocal()
    try:
        return get_scores(db)
    finally:
        db.close()
