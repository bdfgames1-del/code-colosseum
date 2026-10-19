package main

import (
	"encoding/json"
	"fmt"
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

// ĐÃ BỎ HOÀN TOÀN BIẾN LIVES (MẠNG) VÀ ISALIVE
type Player struct {
	Hub      *ArenaHub
	Conn     *websocket.Conn
	Send     chan []byte
	Username string
	Score    int  `json:"score"`
	Answered bool `json:"answered"`
}

type ArenaHub struct {
	Players         map[*Player]bool
	Register        chan *Player
	Unregister      chan *Player
	Broadcast       chan []byte
	AllAnsweredChan chan int
	Mutex           sync.Mutex
	GameRunning     bool
	MatchID         int
	Round           int
	CurrentQuestion *QuestionItem
}

var GlobalArenaHub = &ArenaHub{
	Players:         make(map[*Player]bool),
	Register:        make(chan *Player),
	Unregister:      make(chan *Player),
	Broadcast:       make(chan []byte),
	AllAnsweredChan: make(chan int, 20),
}

// BỘ ĐỀ ĐỦ 10 CÂU HỎI ĐẤU TRƯỜNG ĐUA ĐIỂM
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
		Code:     "# Không có đoạn mã kèm theo",
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
		Exp:      "Phương thức dict.get() trả về None nếu không tìm thấy key thay vì quăng lỗi KeyError.",
	},
	{
		Question: "Giá trị nào sau đây được Python đánh giá là Falsy (tương đương False)?",
		Code:     "# Không có đoạn mã kèm theo",
		Options:  []string{"[] (List rỗng)", "[0]", "'False'", "1"},
		Ans:      0,
		Exp:      "Trong Python, tập hợp rỗng [], {}, chuỗi rỗng '', số 0 và None đều là Falsy.",
	},
	{
		Question: "Khối lệnh nào luôn luôn được thực thi dù có xảy ra ngoại lệ hay không?",
		Code:     "# Không có đoạn mã kèm theo",
		Options:  []string{"else", "finally", "catch", "defer"},
		Ans:      1,
		Exp:      "Khối lệnh 'finally:' luôn luôn được thực thi cuối cùng trong khối try-except.",
	},
	{
		Question: "Kết quả của phép toán 2 ** 3 ** 2 trong Python là bao nhiêu?",
		Code:     "print(2 ** 3 ** 2)",
		Options:  []string{"64", "512", "36", "256"},
		Ans:      1,
		Exp:      "Toán tử lũy thừa '**' tính từ phải sang trái: 3 ** 2 = 9, sau đó 2 ** 9 = 512.",
	},
	{
		Question: "Phương thức nào dùng để thêm một phần tử vào cuối danh sách (List)?",
		Code:     "nums = [1, 2, 3]\nnums.____(4)",
		Options:  []string{"push(4)", "add(4)", "append(4)", "insert(4)"},
		Ans:      2,
		Exp:      "Trong Python, list.append(x) dùng để chèn phần tử x vào cuối danh sách.",
	},
	{
		Question: "Từ khóa nào dùng để khai báo một hàm vô danh (Anonymous Function)?",
		Code:     "f = ____ x: x * 2\nprint(f(5))",
		Options:  []string{"def", "func", "lambda", "arrow"},
		Ans:      2,
		Exp:      "Từ khóa 'lambda' dùng để tạo hàm vô danh ngắn gọn trên một dòng.",
	},
	{
		Question: "Kết quả in ra màn hình của đoạn mã dưới đây là gì?",
		Code:     "s = {1, 2, 2, 3, 3, 3}\nprint(len(s))",
		Options:  []string{"6", "3", "2", "Lỗi cú pháp"},
		Ans:      1,
		Exp:      "Set (tập hợp) tự động loại bỏ các phần tử trùng lặp, nên s chỉ còn {1, 2, 3} có độ dài là 3.",
	},
}

// Kiểm tra xem tất cả người chơi trong phòng đã chọn đáp án chưa
func (h *ArenaHub) allPlayersAnsweredLocked() bool {
	if len(h.Players) == 0 {
		return false
	}
	for p := range h.Players {
		if !p.Answered {
			return false
		}
	}
	return true
}

func (h *ArenaHub) Run() {
	for {
		select {
		case p := <-h.Register:
			h.Mutex.Lock()
			h.Players[p] = true
			total := len(h.Players)
			shouldStart := (total >= 2 && !h.GameRunning)
			if shouldStart {
				h.GameRunning = true
				h.MatchID++
			}
			currentMatchID := h.MatchID
			h.Mutex.Unlock()

			log.Printf("[Đấu Trường] Đấu thủ vào phòng: %s (Tổng: %d)\n", p.Username, total)
			h.broadcastLobbyState()

			if shouldStart {
				go h.runTenQuestionsMatch(currentMatchID)
			}

		case p := <-h.Unregister:
			h.Mutex.Lock()
			if _, ok := h.Players[p]; ok {
				delete(h.Players, p)
				close(p.Send)
			}
			total := len(h.Players)
			if total == 0 {
				h.GameRunning = false
				h.MatchID++
			} else if h.GameRunning && h.allPlayersAnsweredLocked() {
				select {
				case h.AllAnsweredChan <- h.Round:
				default:
				}
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
		Answered bool   `json:"answered"`
	}

	var list []PlayerInfo
	for p := range h.Players {
		list = append(list, PlayerInfo{
			Username: p.Username,
			Score:    p.Score,
			Answered: p.Answered,
		})
	}

	payload, _ := json.Marshal(map[string]interface{}{
		"action": "lobby_state",
		"data": map[string]interface{}{
			"totalPlayers": len(list),
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

// CHẠY XUYÊN SUỐT ĐỦ 10 CÂU HỎI - KHÔNG BAO GIỜ DỪNG Ở CÂU 3 DÙ SAI HAY ĐÚNG
func (h *ArenaHub) runTenQuestionsMatch(matchID int) {
	for i := 3; i >= 1; i-- {
		h.Mutex.Lock()
		if !h.GameRunning || h.MatchID != matchID {
			h.Mutex.Unlock()
			return
		}
		h.Mutex.Unlock()

		h.broadcastJSON("countdown_start", map[string]interface{}{"seconds": i})
		time.Sleep(1 * time.Second)
	}

	// Đặt lại điểm = 0 khi bắt đầu Câu 1
	h.Mutex.Lock()
	if !h.GameRunning || h.MatchID != matchID {
		h.Mutex.Unlock()
		return
	}
	for p := range h.Players {
		p.Score = 0
		p.Answered = false
	}
	h.Mutex.Unlock()
	h.broadcastLobbyState()

	totalQuestions := len(ArenaQuestions) // 10 câu

	// VÒNG LẶP CỐ ĐỊNH TỪ CÂU 1 ĐẾN CÂU 10
	for round := 1; round <= totalQuestions; round++ {
		h.Mutex.Lock()
		if !h.GameRunning || h.MatchID != matchID || len(h.Players) == 0 {
			h.GameRunning = false
			h.Mutex.Unlock()
			return
		}

		// Xóa tín hiệu cũ còn tồn đọng
		for len(h.AllAnsweredChan) > 0 {
			<-h.AllAnsweredChan
		}

		for p := range h.Players {
			p.Answered = false
		}

		h.Round = round
		q := ArenaQuestions[round-1]
		h.CurrentQuestion = &q
		timeLimitSeconds := 20
		h.Mutex.Unlock()

		log.Printf("🔥 [Trận #%d] Đang chạy Câu %d / %d\n", matchID, round, totalQuestions)

		h.broadcastLobbyState()
		h.broadcastJSON("new_round_question", map[string]interface{}{
			"round":       round,
			"totalRounds": totalQuestions,
			"timeLimit":   timeLimitSeconds,
			"question": map[string]interface{}{
				"question": q.Question,
				"code":     q.Code,
				"options":  q.Options,
				"ans":      q.Ans,
				"exp":      q.Exp,
			},
		})

		// Đợi tất cả người chơi chọn đáp án HOẶC hết 20 giây
		timer := time.NewTimer(time.Duration(timeLimitSeconds) * time.Second)
		waiting := true

		for waiting {
			select {
			case doneRound := <-h.AllAnsweredChan:
				if doneRound == round {
					timer.Stop()
					waiting = false
				}
			case <-timer.C:
				h.Mutex.Lock()
				if h.MatchID == matchID {
					for p := range h.Players {
						if !p.Answered {
							p.Answered = true
							p.sendJSON("answer_feedback", map[string]interface{}{
								"correct":     false,
								"scoreGained": 0,
								"totalScore":  p.Score,
								"speedMs":     timeLimitSeconds * 1000,
								"timeout":     true,
							})
						}
					}
				}
				h.Mutex.Unlock()
				waiting = false
			}
		}

		h.broadcastLobbyState()
		h.broadcastJSON("round_finished", map[string]interface{}{"round": round})

		// Nghỉ 1.2 giây để xem điểm cộng rồi chuyển qua câu tiếp theo
		time.Sleep(1200 * time.Millisecond)
	}

	// KẾT THÚC TRẬN ĐẤU SAU KHI ĐÃ ĐI QUA ĐỦ 10 CÂU
	h.Mutex.Lock()
	if h.MatchID != matchID {
		h.Mutex.Unlock()
		return
	}
	h.GameRunning = false
	h.CurrentQuestion = nil

	var topPlayer *Player
	maxScore := -1
	for p := range h.Players {
		if p.Score > maxScore {
			maxScore = p.Score
			topPlayer = p
		}
	}
	h.Mutex.Unlock()

	winMsg := "Đã hoàn thành đủ 10 câu hỏi đua điểm tốc độ!"
	if topPlayer != nil {
		AddWinScore(topPlayer.Username, topPlayer.Score)
		winMsg = fmt.Sprintf("🏆 CHIẾN THẮNG: %s dẫn đầu với %d PTS sau 10 câu!", topPlayer.Username, topPlayer.Score)
	}

	h.broadcastLobbyState()
	h.broadcastJSON("battle_royale_game_over", map[string]interface{}{
		"msg": winMsg,
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

		if err := json.Unmarshal(message, &req); err != nil {
			continue
		}

		if req.Action == "submit_answer" {
			p.Hub.Mutex.Lock()
			if p.Hub.GameRunning && !p.Answered && p.Hub.CurrentQuestion != nil {
				p.Answered = true
				isCorrect := (req.Chosen == p.Hub.CurrentQuestion.Ans)
				scoreGain := 0

				// CHỈ TÍNH ĐIỂM: Trả lời đúng + nhanh = nhiều điểm, trả lời sai = +0 điểm (không trừ mạng)
				if isCorrect {
					speedBonus := 500 - (req.SubmitMs / 40)
					if speedBonus < 50 {
						speedBonus = 50
					}
					if speedBonus > 500 {
						speedBonus = 500
					}
					scoreGain = 500 + speedBonus
					p.Score += scoreGain
				}

				p.sendJSON("answer_feedback", map[string]interface{}{
					"correct":     isCorrect,
					"scoreGained": scoreGain,
					"totalScore":  p.Score,
					"speedMs":     req.SubmitMs,
				})

				// Nếu tất cả người chơi đều đã chọn đáp án -> Qua câu tiếp theo ngay lập tức
				if p.Hub.allPlayersAnsweredLocked() {
					select {
					case p.Hub.AllAnsweredChan <- p.Hub.Round:
					default:
					}
				}
			}
			p.Hub.Mutex.Unlock()

			p.Hub.broadcastLobbyState()
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
		Answered: false,
	}

	hub.Register <- player

	go player.writePump()
	go player.handleIncomingMessages()
}