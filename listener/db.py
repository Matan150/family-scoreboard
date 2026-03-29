import json
from datetime import datetime, timezone

from sqlalchemy import Column, DateTime, ForeignKey, Integer, String, create_engine, func
from sqlalchemy.orm import DeclarativeBase, Session, relationship, sessionmaker


class Base(DeclarativeBase):
    pass


class Member(Base):
    __tablename__ = "members"
    id = Column(Integer, primary_key=True, autoincrement=True)
    name = Column(String, unique=True, nullable=False)
    display_name = Column(String, nullable=False)
    created_at = Column(DateTime, default=lambda: datetime.now(timezone.utc))
    aliases = relationship("MemberAlias", back_populates="member", cascade="all, delete-orphan")
    events = relationship("ScoreEvent", back_populates="member", cascade="all, delete-orphan")


class MemberAlias(Base):
    __tablename__ = "member_aliases"
    id = Column(Integer, primary_key=True, autoincrement=True)
    member_id = Column(Integer, ForeignKey("members.id"), nullable=False)
    alias = Column(String, nullable=False)
    member = relationship("Member", back_populates="aliases")


class ScoreEvent(Base):
    __tablename__ = "score_events"
    id = Column(Integer, primary_key=True, autoincrement=True)
    member_id = Column(Integer, ForeignKey("members.id"), nullable=False, index=True)
    timestamp = Column(DateTime, default=lambda: datetime.now(timezone.utc))
    context_text = Column(String)
    member = relationship("Member", back_populates="events")


engine = create_engine("sqlite:///scoreboard.db", connect_args={"check_same_thread": False})
SessionLocal = sessionmaker(autocommit=False, autoflush=False, bind=engine)
Base.metadata.create_all(bind=engine)


def get_scores(db: Session) -> list:
    members = db.query(Member).all()
    result = []
    for m in members:
        count = db.query(func.count(ScoreEvent.id)).filter(ScoreEvent.member_id == m.id).scalar()
        result.append({
            "member_id": m.id,
            "name": m.name,
            "display_name": m.display_name,
            "count": count,
            "aliases": [a.alias for a in m.aliases],
        })
    return result


def get_all_names_lower(db: Session) -> dict:
    members = db.query(Member).all()
    name_map: dict[str, int] = {}
    for m in members:
        name_map[m.name.lower()] = m.id
        for a in m.aliases:
            if a.alias.strip():
                name_map[a.alias.lower()] = m.id
    return name_map


def broadcast_scores_sync(hub) -> None:
    """Called from HTTP handlers (thread pool) or the listener thread."""
    db = SessionLocal()
    try:
        scores = get_scores(db)
        msg = json.dumps({"type": "score_update", "data": scores})
        hub.broadcast_from_thread(msg)
    finally:
        db.close()
