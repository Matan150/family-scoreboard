import { useState, useEffect, useRef, useCallback } from 'react'
import './App.css'

interface MemberAlias {
  id: number
  member_id: number
  alias: string
}

interface Member {
  id: number
  name: string
  display_name: string
  aliases: MemberAlias[]
  created_at: string
}

interface Score {
  member_id: number
  name: string
  display_name: string
  count: number
  aliases: string[]
}

type View = 'scoreboard' | 'members'

const API = 'http://localhost:8080'

function App() {
  const [view, setView] = useState<View>('scoreboard')
  const [scores, setScores] = useState<Score[]>([])
  const [members, setMembers] = useState<Member[]>([])
  const [connected, setConnected] = useState(false)

  // Form state
  const [showForm, setShowForm] = useState(false)
  const [editingMember, setEditingMember] = useState<Member | null>(null)
  const [formName, setFormName] = useState('')
  const [formDisplayName, setFormDisplayName] = useState('')
  const [formAliases, setFormAliases] = useState<string[]>([''])

  const wsRef = useRef<WebSocket | null>(null)

  // ── WebSocket ──────────────────────────────────────────────────────────────

  const connectWS = useCallback(() => {
    const ws = new WebSocket(`ws://localhost:8080/ws`)
    wsRef.current = ws

    ws.onopen = () => setConnected(true)
    ws.onclose = () => {
      setConnected(false)
      setTimeout(connectWS, 3000)
    }
    ws.onerror = () => ws.close()
    ws.onmessage = (e) => {
      try {
        const msg = JSON.parse(e.data)
        if (msg.type === 'score_update') {
          setScores(msg.data ?? [])
        }
      } catch { /* ignore */ }
    }
  }, [])

  useEffect(() => {
    connectWS()
    return () => wsRef.current?.close()
  }, [connectWS])

  // ── Members CRUD ───────────────────────────────────────────────────────────

  const fetchMembers = async () => {
    const res = await fetch(`${API}/api/members`)
    setMembers(await res.json())
  }

  useEffect(() => {
    if (view === 'members') fetchMembers()
  }, [view])

  const resetForm = () => {
    setFormName('')
    setFormDisplayName('')
    setFormAliases([''])
    setEditingMember(null)
    setShowForm(false)
  }

  const openNewForm = () => {
    resetForm()
    setShowForm(true)
  }

  const openEditForm = (m: Member) => {
    setEditingMember(m)
    setFormName(m.name)
    setFormDisplayName(m.display_name)
    setFormAliases([...m.aliases.map(a => a.alias), ''])
    setShowForm(true)
  }

  const handleSave = async () => {
    const aliases = formAliases.filter(a => a.trim() !== '')
    const payload = {
      name: formName.trim(),
      display_name: formDisplayName.trim() || formName.trim(),
      aliases,
    }
    if (!payload.name) return

    if (editingMember) {
      await fetch(`${API}/api/members/${editingMember.id}`, {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(payload),
      })
    } else {
      await fetch(`${API}/api/members`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(payload),
      })
    }
    resetForm()
    fetchMembers()
  }

  const handleDelete = async (id: number) => {
    if (!confirm('למחוק משתתף זה?')) return
    await fetch(`${API}/api/members/${id}`, { method: 'DELETE' })
    fetchMembers()
  }

  const handleResetScore = async (id: number) => {
    if (!confirm('לאפס את הניקוד?')) return
    await fetch(`${API}/api/members/${id}/reset`, { method: 'DELETE' })
  }

  const addAliasField = () => setFormAliases(prev => [...prev, ''])
  const updateAlias = (i: number, val: string) =>
    setFormAliases(prev => prev.map((a, idx) => (idx === i ? val : a)))
  const removeAlias = (i: number) =>
    setFormAliases(prev => prev.filter((_, idx) => idx !== i))

  // ── Render ─────────────────────────────────────────────────────────────────

  const sortedScores = [...scores].sort((a, b) => b.count - a.count)
  const maxScore = sortedScores.length > 0 ? sortedScores[0].count : 0

  return (
    <div className="app" dir="rtl">
      <header className="app-header">
        <div className="header-title">
          <h1>לוח ניקוד משפחתי</h1>
          <span className={`status-dot ${connected ? 'connected' : 'disconnected'}`}
            title={connected ? 'מחובר לשרת' : 'מנותק — מנסה להתחבר...'} />
        </div>
        <nav className="app-nav">
          <button
            className={view === 'scoreboard' ? 'nav-btn active' : 'nav-btn'}
            onClick={() => setView('scoreboard')}
          >
            🏆 לוח ניקוד
          </button>
          <button
            className={view === 'members' ? 'nav-btn active' : 'nav-btn'}
            onClick={() => setView('members')}
          >
            👥 משתתפים
          </button>
        </nav>
      </header>

      <main className="app-main">
        {/* ── Scoreboard ── */}
        {view === 'scoreboard' && (
          <div className="scoreboard-view">
            {sortedScores.length === 0 ? (
              <div className="empty-state">
                <p>אין משתתפים עדיין</p>
                <button className="btn-primary" onClick={() => setView('members')}>
                  הוסף משתתפים
                </button>
              </div>
            ) : (
              <div className="score-grid">
                {sortedScores.map((s, i) => (
                  <div
                    key={s.member_id}
                    className={`score-card${s.count === maxScore && maxScore > 0 ? ' leading' : ''}${i === 0 && maxScore > 0 ? ' first-place' : ''}`}
                  >
                    {i === 0 && maxScore > 0 && <div className="crown">👑</div>}
                    <div className="score-display-name">{s.display_name}</div>
                    <div className="score-count">{s.count}</div>
                    <div className="score-aliases">
                      {s.aliases.length > 0 ? s.aliases.join(' · ') : s.name}
                    </div>
                    <button
                      className="reset-btn"
                      onClick={() => handleResetScore(s.member_id)}
                      title="אפס ניקוד"
                    >
                      אפס
                    </button>
                  </div>
                ))}
              </div>
            )}
          </div>
        )}

        {/* ── Members management ── */}
        {view === 'members' && (
          <div className="members-view">
            <div className="members-toolbar">
              <h2>ניהול משתתפים</h2>
              <button className="btn-primary" onClick={openNewForm}>+ הוסף משתתף</button>
            </div>

            {showForm && (
              <div className="form-card">
                <h3>{editingMember ? 'עריכת משתתף' : 'משתתף חדש'}</h3>

                <label className="form-label">
                  שם ראשי
                  <span className="form-hint">זה מה שהמשפחה אומרת — נוצר לזיהוי בדיבור</span>
                  <input
                    value={formName}
                    onChange={e => setFormName(e.target.value)}
                    placeholder="לדוגמה: אמא"
                    dir="rtl"
                  />
                </label>

                <label className="form-label">
                  שם תצוגה
                  <span className="form-hint">מה שיופיע על לוח הניקוד</span>
                  <input
                    value={formDisplayName}
                    onChange={e => setFormDisplayName(e.target.value)}
                    placeholder="לדוגמה: אמא"
                    dir="rtl"
                  />
                </label>

                <div className="form-label">
                  <span>כינויים נוספים</span>
                  <span className="form-hint">שמות נוספים שהמשפחה משתמשת — כולם יספרו</span>
                  <div className="aliases-list">
                    {formAliases.map((alias, i) => (
                      <div key={i} className="alias-row">
                        <input
                          value={alias}
                          onChange={e => updateAlias(i, e.target.value)}
                          placeholder={`כינוי ${i + 1} — למשל: אימוש, מאמה`}
                          dir="rtl"
                        />
                        {formAliases.length > 1 && (
                          <button className="remove-btn" onClick={() => removeAlias(i)}>✕</button>
                        )}
                      </div>
                    ))}
                  </div>
                  <button className="btn-ghost" onClick={addAliasField}>+ הוסף כינוי</button>
                </div>

                <div className="form-actions">
                  <button className="btn-primary" onClick={handleSave}>שמור</button>
                  <button className="btn-secondary" onClick={resetForm}>ביטול</button>
                </div>
              </div>
            )}

            <div className="members-list">
              {members.length === 0 && !showForm && (
                <div className="empty-state">
                  <p>לא נוספו משתתפים עדיין</p>
                </div>
              )}
              {members.map(m => (
                <div key={m.id} className="member-card">
                  <div className="member-info">
                    <span className="member-display-name">{m.display_name}</span>
                    <span className="member-aliases-preview">
                      {[m.name, ...m.aliases.map(a => a.alias)].join(' · ')}
                    </span>
                  </div>
                  <div className="member-actions">
                    <button className="btn-secondary small" onClick={() => openEditForm(m)}>עריכה</button>
                    <button className="btn-danger small" onClick={() => handleDelete(m.id)}>מחיקה</button>
                  </div>
                </div>
              ))}
            </div>
          </div>
        )}
      </main>

      <footer className="app-footer">
        <span>מופעל על ידי Whisper.cpp · ivrit-ai · GORM</span>
      </footer>
    </div>
  )
}

export default App
