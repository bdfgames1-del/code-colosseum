package main

import (
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

type QuestionItem struct {
	Question string   `json:"question"`
	Code     string   `json:"code"`
	Options  []string `json:"options"`
	Ans      int      `json:"ans"`
	Exp      string   `json:"exp"`
}

type Player struct {
	Hub      *ArenaHub
	Conn     *websocket.Conn
	Send     chan []byte
	Username string
	Score    int  `json:"score"`
	Lives    int  `json:"lives"`
	IsAlive  bool `json:"isAlive"`
	Answered bool
}

type ArenaHub struct {
	Players         map[*Player]bool
	Register        chan *Player
	Unregister      chan *Player
	Broadcast       chan []byte
	Mutex           sync.Mutex
	GameRunning     bool
	Round           int
	CurrentQuestion *QuestionItem
	RoundStartTime  time.Time
}

var GlobalArenaHub = &ArenaHub{
	Players:    make(map[*Player]bool),
	Register:   make(chan *Player),
	Unregister: make(chan *Player),
	Broadcast:  make(chan []byte),
}

// Bộ câu hỏi cho đấu trường 1v1
var ArenaQuestions = []QuestionItem{
	{
		Question: "Kết quả của cú pháp cắt lát (List Slicing) sau là gì?",
		Code:     "arr = [10, 20, 30, 40, 50]\nprint(arr[1:4])",
		Options:  []string{"[20, 30, 40]", "[10, 20, 30]", "[20, 30, 40, 50]", "[30, 40]"},
		Ans:      0,
		Exp:      "Cắt lát arr[start:stop] lấy từ chỉ số 1 đến trước chỉ số 4, tức [20, 30, 40].",
	},
	{
		Question: "Cấu trúc dữ liệu nào sau đây là Immutable (Bất biến) trong Python?",
		Code:     "",
		Options:  []string{"List", "Tuple", "Dictionary", "Set"},
		Ans:      1,
		Exp:      "Tuple là kiểu dữ liệu bất biến (immutable), không thể chỉnh sửa giá trị sau khi tạo.",
	},
	{
		Question: "Hàm type(10 / 2) trong Python trả về kiểu dữ liệu nào?",
		Code:     "print(type(10 / 2))",
		Options:  []string{"<class 'int'>", "<class 'float'>", "<class 'double'>", "<class 'number'>"},
		Ans:      1,
		Exp:      "Toán tử '/' trong Python luôn trả về kiểu số thực float (10 / 2 = 5.0).",
	},
	{
		Question: "Phương thức nào dùng để lấy giá trị theo Key an toàn không gây lỗi KeyError?",
		Code:     "d = {'score': 100}",
		Options:  []string{"d.fetch('score')", "d.find('score')", "d.get('score')", "d.search('score')"},
		Ans:      2,
		Exp:      "Phương thức dict.get() trả về None nếu không tìm thấy key thay vì quăng lỗi ngoại lệ KeyError.",
	},
	{
		Question: "Giá trị nào sau đây được Python đánh giá là Falsy (tương đương False)?",
		Code:     "",
		Options:  []string{"[] (List rỗng)", "[0]", "'False'", "1"},
		Ans:      0,
		Exp:      "Trong Python, tập hợp rỗng [], {}, chuỗi rỗng '', số 0 và None đều là Falsy value.",
	},
	{
		Question: "Khối lệnh nào luôn luôn được thực thi dù có xảy ra ngoại lệ (exception) hay không?",
		Code:     "",
		Options:  []string{"else", "finally", "catch", "defer"},
		Ans:      1,
		Exp:      "Khối lệnh 'finally:' luôn luôn được thực hiện ở bước cuối cùng của cấu trúc try-except.",
	},
}

func (h *ArenaHub) Run() {
	for {
		select {
		case p := <-h.Register:
			h.Mutex.Lock()
			h.Players[p] = true
			total := len(h.Players)
			h.Mutex.Unlock()

			log.Printf("[Arena] Đấu thủ tham gia: %s (Tổng: %d)\n", p.Username, total)
			h.broadcastLobbyState()

			if total >= 2 && !h.GameRunning {
				h.GameRunning = true
				go h.startMatchCountdown()
			}

		case p := <-h.Unregister:
			h.Mutex.Lock()
			if _, ok := h.Players[p]; ok {
				delete(h.Players, p)
				close(p.Send)
			}
			total := len(h.Players)
			if total < 2 {
				h.GameRunning = false
			}
			h.Mutex.Unlock()
			h.broadcastLobbyState()

		case msg := <-h.Broadcast:
			h.Mutex.Lock()
			for p := range h.Players {
				select {
				case p.Send <- msg:
				default:
					close(p.Send)
					delete(h.Players, p)
				}
			}
			h.Mutex.Unlock()
		}
	}
}

func (h *ArenaHub) broadcastLobbyState() {
	h.Mutex.Lock()
	defer h.Mutex.Unlock()

	type PlayerInfo struct {
		Username string `json:"username"`
		Score    int    `json:"score"`
		Lives    int    `json:"lives"`
		IsAlive  bool   `json:"isAlive"`
	}

	var list []PlayerInfo
	for p := range h.Players {
		list = append(list, PlayerInfo{
			Username: p.Username,
			Score:    p.Score,
			Lives:    p.Lives,
			IsAlive:  p.IsAlive,
		})
	}

	payload, _ := json.Marshal(map[string]interface{}{
		"action": "lobby_state",
		"data": map[string]interface{}{
			"totalPlayers": len(h.Players),
			"players":      list,
		},
	})

	for p := range h.Players {
		select {
		case p.Send <- payload:
		default:
		}
	}
}

func (h *ArenaHub) startMatchCountdown() {
	for i := 3; i >= 1; i-- {
		h.broadcastJSON("countdown_start", map[string]interface{}{"seconds": i})
		time.Sleep(1 * time.Second)
	}

	h.Mutex.Lock()
	h.Round = 0
	for p := range h.Players {
		p.Score = 0
		p.Lives = 3
		p.IsAlive = true
		p.Answered = false
	}
	h.Mutex.Unlock()

	h.nextQuestionRound()
}

func (h *ArenaHub) nextQuestionRound() {
	h.Mutex.Lock()
	aliveCount := 0
	for p := range h.Players {
		if p.IsAlive {
			aliveCount++
		}
		p.Answered = false
	}

	if aliveCount <= 1 && len(h.Players) >= 2 {
		h.Mutex.Unlock()
		h.endMatch("Trận đấu sinh tồn đã kết thúc! Tìm ra người chiến thắng!")
		return
	}

	h.Round++
	if h.Round > len(ArenaQuestions) {
		h.Mutex.Unlock()
		h.endMatch("Đã hoàn thành toàn bộ câu hỏi trong bộ đề!")
		return
	}

	qIndex := (h.Round - 1) % len(ArenaQuestions)
	h.CurrentQuestion = &ArenaQuestions[qIndex]
	h.RoundStartTime = time.Now()

	// THỜI GIAN TRẢ LỜI MỖI CÂU: 20 GIÂY
	timeLimitSeconds := 20
	h.Mutex.Unlock()

	h.broadcastJSON("new_round_question", map[string]interface{}{
		"round":     h.Round,
		"timeLimit": timeLimitSeconds,
		"question": map[string]interface{}{
			"question": h.CurrentQuestion.Question,
			"code":     h.CurrentQuestion.Code,
			"options":  h.CurrentQuestion.Options,
		},
	})

	time.Sleep(time.Duration(timeLimitSeconds) * time.Second)

	h.Mutex.Lock()
	for p := range h.Players {
		if p.IsAlive && !p.Answered {
			p.Lives--
			if p.Lives <= 0 {
				p.IsAlive = false
				p.sendJSON("player_eliminated", map[string]interface{}{})
			}
			p.sendJSON("answer_feedback", map[string]interface{}{
				"correct":     false,
				"scoreGained": 0,
				"lives":       p.Lives,
			})
		}
	}
	h.Mutex.Unlock()

	h.broadcastLobbyState()

	time.Sleep(3 * time.Second)
	h.nextQuestionRound()
}

func (h *ArenaHub) endMatch(message string) {
	h.Mutex.Lock()
	h.GameRunning = false
	h.Mutex.Unlock()

	h.broadcastJSON("battle_royale_game_over", map[string]interface{}{
		"msg": message,
	})
}

func (h *ArenaHub) broadcastJSON(action string, data interface{}) {
	payload, _ := json.Marshal(map[string]interface{}{
		"action": action,
		"data":   data,
	})
	h.Broadcast <- payload
}

func (p *Player) sendJSON(action string, data interface{}) {
	payload, _ := json.Marshal(map[string]interface{}{
		"action": action,
		"data":   data,
	})
	select {
	case p.Send <- payload:
	default:
	}
}

func (p *Player) handleIncomingMessages() {
	defer func() {
		p.Hub.Unregister <- p
		p.Conn.Close()
	}()

	for {
		_, message, err := p.Conn.ReadMessage()
		if err != nil {
			break
		}

		var req struct {
			Action   string `json:"action"`
			Chosen   int    `json:"chosenIdx"`
			SubmitMs int    `json:"submitMs"`
		}

		if err := json.Unmarshal(message, &req); err == nil && req.Action == "submit_answer" {
			p.Hub.Mutex.Lock()
			if p.IsAlive && !p.Answered && p.Hub.CurrentQuestion != nil {
				p.Answered = true
				isCorrect := (req.Chosen == p.Hub.CurrentQuestion.Ans)
				scoreGain := 0

				if isCorrect {
					bonus := 500 - (req.SubmitMs / 50)
					if bonus < 100 {
						bonus = 100
					}
					scoreGain = 500 + bonus
					p.Score += scoreGain
				} else {
					p.Lives--
					if p.Lives <= 0 {
						p.IsAlive = false
						p.sendJSON("player_eliminated", map[string]interface{}{})
					}
				}

				p.sendJSON("answer_feedback", map[string]interface{}{
					"correct":     isCorrect,
					"scoreGained": scoreGain,
					"speedMs":     req.SubmitMs,
					"lives":       p.Lives,
				})
			}
			p.Hub.Mutex.Unlock()
		}
	}
}

func (p *Player) writePump() {
	ticker := time.NewTicker(25 * time.Second)
	defer func() {
		ticker.Stop()
		p.Conn.Close()
	}()

	for {
		select {
		case message, ok := <-p.Send:
			if !ok {
				p.Conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			p.Conn.WriteMessage(websocket.TextMessage, message)
		case <-ticker.C:
			if err := p.Conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

func ServeArenaWs(hub *ArenaHub, w http.ResponseWriter, r *http.Request, username string) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("Lỗi WebSocket Upgrade:", err)
		return
	}

	player := &Player{
		Hub:      hub,
		Conn:     conn,
		Send:     make(chan []byte, 256),
		Username: username,
		Score:    0,
		Lives:    3,
		IsAlive:  true,
		Answered: false,
	}

	hub.Register <- player

	go player.writePump()
	go player.handleIncomingMessages()
}
