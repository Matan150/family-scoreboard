package main

import (
	"encoding/json"
	"fmt"
	"log"
	"math"
	"net/http"
	"strings"
	"sync"
	"time"

	pvrecorder "github.com/Picovoice/pvrecorder/binding/go"
	"github.com/ggerganov/whisper.cpp/bindings/go/pkg/whisper"
	"github.com/gorilla/websocket"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

/*
#cgo CFLAGS: -I${SRCDIR}/whisper.cpp/include -I${SRCDIR}/whisper.cpp/ggml/include
#cgo LDFLAGS: -L${SRCDIR}/whisper.cpp/build_go/src -L${SRCDIR}/whisper.cpp/build_go/ggml/src -lwhisper -lggml -lggml-base -lggml-cpu -lgomp -lm -lstdc++
*/
import "C"

const (
	SampleRate    = 16000
	BufferSeconds = 4
	StepSeconds   = 2
	VADThreshold  = 0.008
	Port          = ":8080"
)

// ── Database models ────────────────────────────────────────────────────────────

type Member struct {
	ID          uint          `gorm:"primaryKey" json:"id"`
	Name        string        `gorm:"uniqueIndex" json:"name"`
	DisplayName string        `json:"display_name"`
	CreatedAt   time.Time     `json:"created_at"`
	Aliases     []MemberAlias `gorm:"foreignKey:MemberID;constraint:OnDelete:CASCADE" json:"aliases"`
}

type MemberAlias struct {
	ID       uint   `gorm:"primaryKey" json:"id"`
	MemberID uint   `json:"member_id"`
	Alias    string `json:"alias"`
}

type ScoreEvent struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	MemberID    uint      `gorm:"index" json:"member_id"`
	Timestamp   time.Time `json:"timestamp"`
	ContextText string    `json:"context_text"`
}

// ── API types ──────────────────────────────────────────────────────────────────

type ScoreData struct {
	MemberID    uint     `json:"member_id"`
	Name        string   `json:"name"`
	DisplayName string   `json:"display_name"`
	Count       int64    `json:"count"`
	Aliases     []string `json:"aliases"`
}

type MemberRequest struct {
	Name        string   `json:"name"`
	DisplayName string   `json:"display_name"`
	Aliases     []string `json:"aliases"`
}

type WSMessage struct {
	Type string      `json:"type"`
	Data interface{} `json:"data"`
}

// ── WebSocket hub ──────────────────────────────────────────────────────────────

type Hub struct {
	clients    map[*websocket.Conn]bool
	broadcast  chan []byte
	register   chan *websocket.Conn
	unregister chan *websocket.Conn
	mu         sync.Mutex
}

func newHub() *Hub {
	return &Hub{
		clients:    make(map[*websocket.Conn]bool),
		broadcast:  make(chan []byte, 256),
		register:   make(chan *websocket.Conn),
		unregister: make(chan *websocket.Conn),
	}
}

func (h *Hub) run() {
	for {
		select {
		case conn := <-h.register:
			h.mu.Lock()
			h.clients[conn] = true
			h.mu.Unlock()
		case conn := <-h.unregister:
			h.mu.Lock()
			if _, ok := h.clients[conn]; ok {
				delete(h.clients, conn)
				conn.Close()
			}
			h.mu.Unlock()
		case msg := <-h.broadcast:
			h.mu.Lock()
			for conn := range h.clients {
				if err := conn.WriteMessage(websocket.TextMessage, msg); err != nil {
					delete(h.clients, conn)
					conn.Close()
				}
			}
			h.mu.Unlock()
		}
	}
}

func (h *Hub) broadcastScores(db *gorm.DB) {
	scores := getScores(db)
	msg := WSMessage{Type: "score_update", Data: scores}
	data, _ := json.Marshal(msg)
	select {
	case h.broadcast <- data:
	default:
	}
}

// ── Database helpers ───────────────────────────────────────────────────────────

func getScores(db *gorm.DB) []ScoreData {
	var members []Member
	db.Preload("Aliases").Find(&members)

	scores := make([]ScoreData, 0, len(members))
	for _, m := range members {
		var count int64
		db.Model(&ScoreEvent{}).Where("member_id = ?", m.ID).Count(&count)

		aliases := make([]string, 0, len(m.Aliases))
		for _, a := range m.Aliases {
			aliases = append(aliases, a.Alias)
		}
		scores = append(scores, ScoreData{
			MemberID:    m.ID,
			Name:        m.Name,
			DisplayName: m.DisplayName,
			Count:       count,
			Aliases:     aliases,
		})
	}
	return scores
}

// getAllNamesLower returns a map of lowercase name/alias → member ID
func getAllNamesLower(db *gorm.DB) map[string]uint {
	var members []Member
	db.Preload("Aliases").Find(&members)

	nameMap := make(map[string]uint)
	for _, m := range members {
		nameMap[strings.ToLower(m.Name)] = m.ID
		for _, a := range m.Aliases {
			if a.Alias != "" {
				nameMap[strings.ToLower(a.Alias)] = m.ID
			}
		}
	}
	return nameMap
}

// ── Hebrew direct-address detection ───────────────────────────────────────────
//
// A name occurrence scores +points for direct-address signals and -points for
// indirect-mention signals. If the final score > 0, it counts as a direct address.
//
// Direct address: "אמא, אפשר לאכול עוגיה?"  → name at start + request word
// Indirect mention: "אמא אמרה לי שאסור"      → name at start + 3rd-person past verb

var directWords = []string{
	// permission / requests
	"אפשר", "בבקשה", "סליחה",
	// 2nd-person imperatives and modal verbs
	"תוכל", "תוכלי", "תן", "תני", "בוא", "בואי", "עזור", "עזרי",
	"תראה", "תראי", "שמע", "שמעי", "תגיד", "תגידי",
	"תביא", "תביאי", "תעשה", "תעשי", "תקח", "תקחי",
	"תשים", "תשימי", "תבוא", "תבואי", "תיתן", "תיתני",
	"תפתח", "תפתחי", "תסגור", "תסגרי", "תשב", "תשבי",
	"תלך", "תלכי", "תסתכל", "תסתכלי", "תעזור", "תעזרי",
	"תקשיב", "תקשיבי", "חכה", "חכי", "קח", "קחי",
	// question words (implying you're asking someone present)
	"מה", "למה", "איך", "מתי", "כמה", "האם", "מי", "איפה",
	// first-person fragments implying a request to the addressee
	"אני", "אנחנו",
	// common vocative particles
	"יש", "רגע", "רגעיה",
}

var indirectWords = []string{
	// 3rd-person past tense verbs (he said / she said / they said…)
	"אמר", "אמרה", "אמרו", "אמרת",
	"עשה", "עשתה", "עשו", "עשית",
	"הלך", "הלכה", "הלכו", "הלכת",
	"נתן", "נתנה", "נתנו", "נתת",
	"אסר", "אסרה", "אסרו",
	"הגיד", "הגידה", "הגידו",
	"סיפר", "סיפרה", "סיפרו",
	"כתב", "כתבה", "כתבו",
	"קנה", "קנתה", "קנו",
	"הביא", "הביאה", "הביאו",
	"ביקש", "ביקשה", "ביקשו",
	"החליט", "החליטה", "החליטו",
	"הסכים", "הסכימה", "הסכימו",
	"ראה", "ראתה", "ראו",
	"ידע", "ידעה", "ידעו",
	"חשב", "חשבה", "חשבו",
	"רצה", "רצתה", "רצו",
	"יצא", "יצאה", "יצאו",
	"בא", "באה", "באו",
	"שלח", "שלחה", "שלחו",
	"לקח", "לקחה", "לקחו",
	// possessive / referential particles
	"של", "שלה", "שלו", "שלהם",
}

func containsWord(list []string, w string) bool {
	for _, item := range list {
		if item == w {
			return true
		}
	}
	return false
}

// cleanWord strips punctuation from a word token
func cleanWord(w string) string {
	return strings.Trim(w, ".,!?;:\"'()[]")
}

func isDirectAddress(text, name string) bool {
	textLower := strings.ToLower(text)
	nameLower := strings.ToLower(name)

	idx := strings.Index(textLower, nameLower)
	if idx == -1 {
		return false
	}

	beforeName := strings.TrimSpace(textLower[:idx])
	afterRaw := strings.TrimSpace(textLower[idx+len(nameLower):])
	afterClean := strings.TrimLeft(afterRaw, " .,!?;:")
	wordsAfter := strings.Fields(afterClean)

	score := 0

	// Name at start of utterance — strong direct-address signal
	if beforeName == "" {
		score += 3
	}

	// Name at end of utterance — vocative
	if strings.TrimSpace(afterRaw) == "" ||
		strings.TrimSpace(afterRaw) == "?" ||
		strings.TrimSpace(afterRaw) == "!" {
		score += 3
		return true // definitely vocative
	}

	// Inspect up to 4 words following the name
	limit := 4
	if len(wordsAfter) < limit {
		limit = len(wordsAfter)
	}
	for i := 0; i < limit; i++ {
		w := cleanWord(wordsAfter[i])
		if containsWord(directWords, w) {
			score += 2
		}
		if containsWord(indirectWords, w) {
			score -= 3
		}
	}

	return score > 0
}

// ── HTTP handlers ──────────────────────────────────────────────────────────────

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

func cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func handleScores(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(getScores(db))
	}
}

func handleMembers(db *gorm.DB, hub *Hub) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Parse path: /api/members, /api/members/{id}, /api/members/{id}/reset
		path := strings.TrimPrefix(r.URL.Path, "/api/members")
		path = strings.TrimPrefix(path, "/")
		parts := strings.SplitN(path, "/", 2)
		id := parts[0]
		action := ""
		if len(parts) == 2 {
			action = parts[1]
		}

		w.Header().Set("Content-Type", "application/json")

		switch {
		// GET /api/members
		case r.Method == http.MethodGet && id == "":
			var members []Member
			db.Preload("Aliases").Find(&members)
			json.NewEncoder(w).Encode(members)

		// POST /api/members
		case r.Method == http.MethodPost && id == "":
			var req MemberRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			if req.DisplayName == "" {
				req.DisplayName = req.Name
			}
			member := Member{Name: req.Name, DisplayName: req.DisplayName}
			if err := db.Create(&member).Error; err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			for _, alias := range req.Aliases {
				if strings.TrimSpace(alias) != "" {
					db.Create(&MemberAlias{MemberID: member.ID, Alias: strings.TrimSpace(alias)})
				}
			}
			db.Preload("Aliases").First(&member, member.ID)
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(member)
			hub.broadcastScores(db)

		// PUT /api/members/{id}
		case r.Method == http.MethodPut && id != "" && action == "":
			var req MemberRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			if req.DisplayName == "" {
				req.DisplayName = req.Name
			}
			var member Member
			if err := db.First(&member, id).Error; err != nil {
				http.Error(w, "not found", http.StatusNotFound)
				return
			}
			member.Name = req.Name
			member.DisplayName = req.DisplayName
			db.Save(&member)
			db.Where("member_id = ?", member.ID).Delete(&MemberAlias{})
			for _, alias := range req.Aliases {
				if strings.TrimSpace(alias) != "" {
					db.Create(&MemberAlias{MemberID: member.ID, Alias: strings.TrimSpace(alias)})
				}
			}
			db.Preload("Aliases").First(&member, member.ID)
			json.NewEncoder(w).Encode(member)
			hub.broadcastScores(db)

		// DELETE /api/members/{id}/reset  — reset score only
		case r.Method == http.MethodDelete && id != "" && action == "reset":
			db.Where("member_id = ?", id).Delete(&ScoreEvent{})
			w.WriteHeader(http.StatusNoContent)
			hub.broadcastScores(db)

		// DELETE /api/members/{id}  — delete member entirely
		case r.Method == http.MethodDelete && id != "" && action == "":
			db.Where("member_id = ?", id).Delete(&MemberAlias{})
			db.Where("member_id = ?", id).Delete(&ScoreEvent{})
			db.Delete(&Member{}, id)
			w.WriteHeader(http.StatusNoContent)
			hub.broadcastScores(db)

		default:
			http.Error(w, "not found", http.StatusNotFound)
		}
	}
}

func handleWS(hub *Hub, db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Println("WS upgrade error:", err)
			return
		}
		hub.register <- conn

		// Send current scores immediately on connect
		scores := getScores(db)
		msg := WSMessage{Type: "score_update", Data: scores}
		data, _ := json.Marshal(msg)
		conn.WriteMessage(websocket.TextMessage, data)

		// Drain incoming messages (keep-alive / handle close)
		go func() {
			defer func() { hub.unregister <- conn }()
			for {
				if _, _, err := conn.ReadMessage(); err != nil {
					return
				}
			}
		}()
	}
}

// ── Audio listener ─────────────────────────────────────────────────────────────

func isLoudEnough(samples []float32) bool {
	max := float32(0)
	for _, s := range samples {
		if abs := float32(math.Abs(float64(s))); abs > max {
			max = abs
		}
	}
	return max > VADThreshold
}

func runListener(db *gorm.DB, hub *Hub, modelPath string) {
	fmt.Printf("📂 Loading Hebrew model: %s\n", modelPath)
	model, err := whisper.New(modelPath)
	if err != nil {
		log.Fatalf("Failed to load Whisper model: %v", err)
	}
	defer model.Close()

	ctx, err := model.NewContext()
	if err != nil {
		log.Fatalf("Failed to create Whisper context: %v", err)
	}
	ctx.SetLanguage("he")
	ctx.SetTemperature(0.0)
	ctx.SetThreads(4)
	ctx.SetInitialPrompt("משפחה, ניקוד, ילדים, אמא, אבא")
	fmt.Println("✅ Hebrew model loaded!")

	recorder := pvrecorder.NewPvRecorder(512)
	if err := recorder.Init(); err != nil {
		log.Fatalf("Mic init failed: %v", err)
	}
	defer recorder.Delete()

	audioChan := make(chan []float32, 500)
	go func() {
		recorder.Start()
		fmt.Println("🎤 Microphone is ON — listening in Hebrew")
		for {
			pcm, err := recorder.Read()
			if err != nil {
				continue
			}
			f32 := make([]float32, len(pcm))
			for i, s := range pcm {
				f32[i] = float32(s) / 32768.0
			}
			audioChan <- f32
		}
	}()

	samplesPerBuffer := SampleRate * BufferSeconds
	fullBuffer := make([]float32, 0, samplesPerBuffer*4)
	recentEvents := make(map[string]time.Time) // dedup key → last event time

	for pcmFrame := range audioChan {
		fullBuffer = append(fullBuffer, pcmFrame...)

		if len(fullBuffer) < samplesPerBuffer {
			continue
		}

		if isLoudEnough(fullBuffer[:samplesPerBuffer]) {
			fmt.Print("🧠 מעבד...")
			start := time.Now()

			err := ctx.Process(fullBuffer[:samplesPerBuffer], nil, nil, nil)
			fmt.Printf(" סיים (%v)\n", time.Since(start).Truncate(time.Millisecond))

			if err == nil {
				var sb strings.Builder
				for {
					seg, err := ctx.NextSegment()
					if err != nil {
						break
					}
					sb.WriteString(seg.Text)
				}
				text := strings.TrimSpace(sb.String())
				if text != "" {
					fmt.Printf(">> שמעתי: %s\n", text)

					nameMap := getAllNamesLower(db)
					for name, memberID := range nameMap {
						if strings.Contains(strings.ToLower(text), name) {
							if isDirectAddress(text, name) {
								// Dedup: max 1 point per member per 30 seconds
								dedupeKey := fmt.Sprintf("%d", memberID)
								if last, ok := recentEvents[dedupeKey]; ok && time.Since(last) < 30*time.Second {
									fmt.Printf("⏭️  כפילות מדולגת עבור member %d\n", memberID)
									continue
								}
								recentEvents[dedupeKey] = time.Now()

								db.Create(&ScoreEvent{
									MemberID:    memberID,
									Timestamp:   time.Now(),
									ContextText: text,
								})
								fmt.Printf("🔥 [נקודה!] member %d — %s\n", memberID, text)
								hub.broadcastScores(db)
							} else {
								fmt.Printf("📌 אזכור (לא פנייה ישירה): %s\n", text)
							}
						}
					}
				}
			}
		}

		// Slide buffer
		shift := SampleRate * StepSeconds
		if len(fullBuffer) > shift {
			fullBuffer = fullBuffer[shift:]
		} else {
			fullBuffer = fullBuffer[:0]
		}
	}
}

// ── Init & main ────────────────────────────────────────────────────────────────

func initDB() *gorm.DB {
	db, err := gorm.Open(sqlite.Open("scoreboard.db"), &gorm.Config{
		Logger: gormlogger.Default.LogMode(gormlogger.Warn),
	})
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	db.AutoMigrate(&Member{}, &MemberAlias{}, &ScoreEvent{})
	return db
}

// func findModel() string {
// 	if path := os.Getenv("WHISPER_MODEL"); path != "" {
// 		return path
// 	}
// 	candidates := []string{
// 		"ggml-ivrit-ai-whisper-turbo.bin",
// 		"ivrit-ai-whisper-turbo.bin",
// 		"ivrit-turbo.bin",
// 		"ggml-ivrit-turbo.bin",
// 		"ggml-small-he.bin",
// 		"ggml-medium.bin",
// 		"ggml-small.bin",
// 	}
// 	for _, c := range candidates {
// 		if _, err := os.Stat(c); err == nil {
// 			return c
// 		}
// 	}
// 	return ""
// }

func main() {
	db := initDB()
	hub := newHub()
	go hub.run()

	modelPath := `C:\Users\User\desktop\homeprojects\family-scoreboard\listener\whisper.cpp\models\ggml-ivrit-turbo.bin`
	if modelPath == "" {
		log.Println("⚠️  No Whisper model found — audio listener disabled. Set WHISPER_MODEL=/path/to/model.bin to enable.")
	} else {
		go runListener(db, hub, modelPath)
	}

	mux := http.NewServeMux()
	mux.Handle("/api/scores", cors(handleScores(db)))
	mux.Handle("/api/members", cors(handleMembers(db, hub)))
	mux.Handle("/api/members/", cors(handleMembers(db, hub)))
	mux.Handle("/ws", cors(handleWS(hub, db)))

	fmt.Printf("🚀 Server running at http://localhost%s\n", Port)
	log.Fatal(http.ListenAndServe(Port, mux))
}
