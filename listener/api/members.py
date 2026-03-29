from typing import List

from fastapi import APIRouter, HTTPException
from pydantic import BaseModel

from db import Member, MemberAlias, ScoreEvent, SessionLocal, broadcast_scores_sync
from hub import hub

router = APIRouter()


class MemberRequest(BaseModel):
    name: str
    display_name: str = ""
    aliases: List[str] = []


def _member_json(m: Member) -> dict:
    return {
        "id": m.id,
        "name": m.name,
        "display_name": m.display_name,
        "created_at": m.created_at.isoformat() if m.created_at else None,
        "aliases": [{"id": a.id, "member_id": a.member_id, "alias": a.alias} for a in m.aliases],
    }


@router.get("/api/members")
def api_get_members():
    db = SessionLocal()
    try:
        members = db.query(Member).all()
        return [_member_json(m) for m in members]
    finally:
        db.close()


@router.post("/api/members", status_code=201)
def api_create_member(req: MemberRequest):
    db = SessionLocal()
    try:
        member = Member(name=req.name, display_name=req.display_name or req.name)
        db.add(member)
        db.commit()
        db.refresh(member)
        for alias in req.aliases:
            if alias.strip():
                db.add(MemberAlias(member_id=member.id, alias=alias.strip()))
        db.commit()
        db.refresh(member)
        result = _member_json(member)
        broadcast_scores_sync(hub)
        return result
    finally:
        db.close()


@router.put("/api/members/{member_id}")
def api_update_member(member_id: int, req: MemberRequest):
    db = SessionLocal()
    try:
        member = db.query(Member).filter(Member.id == member_id).first()
        if not member:
            raise HTTPException(status_code=404, detail="not found")
        member.name = req.name
        member.display_name = req.display_name or req.name
        db.query(MemberAlias).filter(MemberAlias.member_id == member_id).delete()
        db.commit()
        for alias in req.aliases:
            if alias.strip():
                db.add(MemberAlias(member_id=member.id, alias=alias.strip()))
        db.commit()
        db.refresh(member)
        result = _member_json(member)
        broadcast_scores_sync(hub)
        return result
    finally:
        db.close()


@router.delete("/api/members/{member_id}/reset", status_code=204)
def api_reset_score(member_id: int):
    db = SessionLocal()
    try:
        db.query(ScoreEvent).filter(ScoreEvent.member_id == member_id).delete()
        db.commit()
        broadcast_scores_sync(hub)
    finally:
        db.close()


@router.delete("/api/members/{member_id}", status_code=204)
def api_delete_member(member_id: int):
    db = SessionLocal()
    try:
        db.query(MemberAlias).filter(MemberAlias.member_id == member_id).delete()
        db.query(ScoreEvent).filter(ScoreEvent.member_id == member_id).delete()
        db.query(Member).filter(Member.id == member_id).delete()
        db.commit()
        broadcast_scores_sync(hub)
    finally:
        db.close()
