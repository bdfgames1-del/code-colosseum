package main

import (
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"strings"
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

// Mỗi trận đấu là 1 Máy chủ (Room) độc lập chạy song song
type MatchRoom struct {
	ID              int
	Mode            string
	Players         map[*Player]bool
	AllAnsweredChan chan int
	CurrentQuestion *QuestionItem
	Round           int
	Active          bool
}

type Player struct {
	Hub          *ArenaHub
	Conn         *websocket.Conn
	Send         chan []byte
	Username     string
	Score        int    `json:"score"`
	SelectedMode string `json:"mode"`
	InQueue      bool   `json:"inQueue"`
	Team         string `json:"team"` // "BLUE" (Đội Xanh) hoặc "RED" (Đội Đỏ)
	Answered     bool   `json:"answered"`
	Room         *MatchRoom
}

type ArenaHub struct {
	Players     map[*Player]bool
	Rooms       map[int]*MatchRoom
	Register    chan *Player
	Unregister  chan *Player
	Mutex       sync.Mutex
	NextRoomSeq int
}

var GlobalArenaHub = &ArenaHub{
	Players:  make(map[*Player]bool),
	Rooms:    make(map[int]*MatchRoom),
	Register: make(chan *Player),
	Unregister: make(chan *Player),
}

// Số người cần thiết cho từng chế độ
var ModeRequiredPlayers = map[string]int{
	"1v1": 2,
	"2v2": 4,
	"3v3": 6,
	"5v5": 10,
}

// BỘ ĐỀ 10 CÂU HỎI ĐẤU TRƯỜNG
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

func init() {
	rand.Seed(time.Now().UnixNano())
}

func (h *ArenaHub) Run() {
	for {
		select {
		case p := <-h.Register:
			h.Mutex.Lock()
			h.Players[p] = true
			h.Mutex.Unlock()
			h.broadcastLobbyQueues()

		case p := <-h.Unregister:
			h.Mutex.Lock()
			room := p.Room
			if _, ok := h.Players[p]; ok {
				delete(h.Players, p)
				close(p.Send)
			}
			if room != nil {
				delete(room.Players, p)
				p.Room = nil
				if len(room.Players) == 0 {
					room.Active = false
					delete(h.Rooms, room.ID)
				} else if h.checkRoomAllAnsweredLocked(room) {
					select {
					case room.AllAnsweredChan <- room.Round:
					default:
					}
				}
			}
			h.Mutex.Unlock()

			if room != nil {
				h.broadcastRoomState(room)
			}
			h.broadcastLobbyQueues()
		}
	}
}

func (h *ArenaHub) checkRoomAllAnsweredLocked(room *MatchRoom) bool {
	if len(room.Players) == 0 {
		return false
	}
	for p := range room.Players {
		if !p.Answered {
			return false
		}
	}
	return true
}

// GHÉP TRẬN NGẪU NHIÊN VÀ TẠO MÁY CHỦ RIÊNG CHO TỪNG NHÓM ĐỦ NGƯỜI
func (h *ArenaHub) tryMatchmakingLocked(mode string) {
	required, ok := ModeRequiredPlayers[mode]
	if !ok {
		required = 2
	}

	var waiting []*Player
	for p := range h.Players {
		if p.Room == nil && p.InQueue && p.SelectedMode == mode {
			waiting = append(waiting, p)
		}
	}

	// Nếu đủ số người cho chế độ đó -> Trộn ngẫu nhiên và tạo Phòng/Server mới ngay!
	for len(waiting) >= required {
		rand.Shuffle(len(waiting), func(i, j int) {
			waiting[i], waiting[j] = waiting[j], waiting[i]
		})

		matchedGroup := waiting[:required]
		waiting = waiting[required:]

		h.NextRoomSeq++
		room := &MatchRoom{
			ID:              h.NextRoomSeq,
			Mode:            mode,
			Players:         make(map[*Player]bool),
			AllAnsweredChan: make(chan int, 20),
			Active:          true,
		}
		h.Rooms[room.ID] = room

		half := required / 2
		for idx, pl := range matchedGroup {
			pl.InQueue = false
			pl.Room = room
			pl.Score = 0
			pl.Answered = false
			if idx < half {
				pl.Team = "BLUE"
			} else {
				pl.Team = "RED"
			}
			room.Players[pl] = true
		}

		log.Printf("⚡ [ Tạo Máy Chủ #%d ] Chế độ %s (%d người chơi) bắt đầu!\n", room.ID, strings.ToUpper(mode), required)
		go h.runRoomMatch(room)
	}
}

// Cập nhật số lượng người đang tìm trận ở sảnh chờ
func (h *ArenaHub) broadcastLobbyQueues() {
	h.Mutex.Lock()
	defer h.Mutex.Unlock()

	queueCounts := map[string]int{
		"1v1": 0,
		"2v2": 0,
		"3v3": 0,
		"5v5": 0,
	}

	type LobbyPlayer struct {
		Username string `json:"username"`
		Mode     string `json:"mode"`
		InQueue  bool   `json:"inQueue"`
		RoomID   int    `json:"roomId"`
	}

	var onlineList []LobbyPlayer
	for p := range h.Players {
		if p.Room == nil && p.InQueue {
			queueCounts[p.SelectedMode]++
		}
		rID := 0
		if p.Room != nil {
			rID = p.Room.ID
		}
		onlineList = append(onlineList, LobbyPlayer{
			Username: p.Username,
			Mode:     p.SelectedMode,
			InQueue:  p.InQueue,
			RoomID:   rID,
		})
	}

	payload, _ := json.Marshal(map[string]interface{}{
		"action": "lobby_queues",
		"data": map[string]interface{}{
			"queues":        queueCounts,
			"activeServers": len(h.Rooms),
			"onlineCount":   len(h.Players),
			"onlinePlayers": onlineList,
		},
	})

	// Chỉ gửi thông tin sảnh chờ cho những người chưa vào trận
	for p := range h.Players {
		if p.Room == nil {
			select {
			case p.Send <- payload:
			default:
			}
		}
	}
}

// Gửi bảng điểm Đội Xanh vs Đội Đỏ trong phạm vi 1 Máy chủ (Room)
func (h *ArenaHub) broadcastRoomState(room *MatchRoom) {
	h.Mutex.Lock()
	defer h.Mutex.Unlock()

	type RoomPlayerInfo struct {
		Username string `json:"username"`
		Team     string `json:"team"`
		Score    int    `json:"score"`
		Answered bool   `json:"answered"`
	}

	var list []RoomPlayerInfo
	blueScore := 0
	redScore := 0

	for p := range room.Players {
		if p.Team == "BLUE" {
			blueScore += p.Score
		} else {
			redScore += p.Score
		}
		list = append(list, RoomPlayerInfo{
			Username: p.Username,
			Team:     p.Team,
			Score:    p.Score,
			Answered: p.Answered,
		})
	}

	payload, _ := json.Marshal(map[string]interface{}{
		"action": "room_state",
		"data": map[string]interface{}{
			"roomId":    room.ID,
			"mode":      room.Mode,
			"blueScore": blueScore,
			"redScore":  redScore,
			"players":   list,
		},
	})

	for p := range room.Players {
		select {
		case p.Send <- payload:
		default:
		}
	}
}

func (h *ArenaHub) broadcastToRoom(room *MatchRoom, action string, data interface{}) {
	payload, _ := json.Marshal(map[string]interface{}{
		"action": action,
		"data":   data,
	})
	h.Mutex.Lock()
	defer h.Mutex.Unlock()
	for p := range room.Players {
		select {
		case p.Send <- payload:
		default:
		}
	}
}

// LUỒNG CHẠY ĐỘC LẬP CỦA TỪNG MÁY CHỦ (10 CÂU HỎI)
func (h *ArenaHub) runRoomMatch(room *MatchRoom) {
	h.broadcastRoomState(room)
	h.broadcastLobbyQueues()

	// Đếm ngược 3 giây khi tìm thấy trận
	for i := 3; i >= 1; i-- {
		h.Mutex.Lock()
		if !room.Active || len(room.Players) == 0 {
			h.Mutex.Unlock()
			return
		}
		h.Mutex.Unlock()

		h.broadcastToRoom(room, "match_found_countdown", map[string]interface{}{
			"seconds": i,
			"roomId":  room.ID,
			"mode":    strings.ToUpper(room.Mode),
		})
		time.Sleep(1 * time.Second)
	}

	totalQuestions := len(ArenaQuestions) // 10 câu

	for round := 1; round <= totalQuestions; round++ {
		h.Mutex.Lock()
		if !room.Active || len(room.Players) == 0 {
			h.Mutex.Unlock()
			return
		}

		for len(room.AllAnsweredChan) > 0 {
			<-room.AllAnsweredChan
		}

		for p := range room.Players {
			p.Answered = false
		}

		room.Round = round
		q := ArenaQuestions[round-1]
		room.CurrentQuestion = &q
		timeLimitSeconds := 20
		h.Mutex.Unlock()

		h.broadcastRoomState(room)
		h.broadcastToRoom(room, "new_round_question", map[string]interface{}{
			"roomId":      room.ID,
			"mode":        strings.ToUpper(room.Mode),
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

		timer := time.NewTimer(time.Duration(timeLimitSeconds) * time.Second)
		waiting := true

		for waiting {
			select {
			case doneRound := <-room.AllAnsweredChan:
				if doneRound == round {
					timer.Stop()
					waiting = false
				}
			case <-timer.C:
				h.Mutex.Lock()
				if room.Active {
					for p := range room.Players {
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

		h.broadcastRoomState(room)
		h.broadcastToRoom(room, "round_finished", map[string]interface{}{"round": round})

		time.Sleep(1200 * time.Millisecond)
	}

	// KẾT THÚC 10 CÂU HỎI TRONG MÁY CHỦ NÀY
	h.Mutex.Lock()
	if !room.Active {
		h.Mutex.Unlock()
		return
	}
	room.Active = false

	blueTotal := 0
	redTotal := 0
	var mvpPlayer *Player
	maxScore := -1

	for p := range room.Players {
		if p.Team == "BLUE" {
			blueTotal += p.Score
		} else {
			redTotal += p.Score
		}
		if p.Score > maxScore {
			maxScore = p.Score
			mvpPlayer = p
		}
	}

	var resultTitle string
	if blueTotal > redTotal {
		resultTitle = fmt.Sprintf("🔵 ĐỘI XANH CHIẾN THẮNG! (%d - %d PTS)", blueTotal, redTotal)
	} else if redTotal > blueTotal {
		resultTitle = fmt.Sprintf("🔴 ĐỘI ĐỎ CHIẾN THẮNG! (%d - %d PTS)", redTotal, blueTotal)
	} else {
		resultTitle = fmt.Sprintf("⚖️ HAI ĐỘI HÒA NHAU! (%d - %d PTS)", blueTotal, redTotal)
	}

	if mvpPlayer != nil {
		AddWinScore(mvpPlayer.Username, mvpPlayer.Score)
		resultTitle += fmt.Sprintf(" | 🌟 MVP: %s (%d PTS)", mvpPlayer.Username, mvpPlayer.Score)
	}

	endPayload, _ := json.Marshal(map[string]interface{}{
		"action": "battle_royale_game_over",
		"data": map[string]interface{}{
			"msg":       resultTitle,
			"blueScore": blueTotal,
			"redScore":  redTotal,
		},
	})

	for p := range room.Players {
		select {
		case p.Send <- endPayload:
		default:
		}
		p.Room = nil
		p.InQueue = false
		p.Answered = false
	}

	delete(h.Rooms, room.ID)
	h.Mutex.Unlock()

	h.broadcastLobbyQueues()
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
			Action     string `json:"action"`
			Mode       string `json:"mode"`
			Ready      bool   `json:"ready"`
			CustomName string `json:"customName"`
			Chosen     int    `json:"chosenIdx"`
			SubmitMs   int    `json:"submitMs"`
		}

		if err := json.Unmarshal(message, &req); err != nil {
			continue
		}

		// NGƯỜI CHƠI BẤM TÌM TRẬN THEO CHẾ ĐỘ (1v1, 2v2, 3v3, 5v5)
		if req.Action == "join_queue" {
			p.Hub.Mutex.Lock()
			if p.Room == nil {
				cleanName := strings.TrimSpace(req.CustomName)
				if cleanName != "" && len(cleanName) <= 24 {
					p.Username = cleanName
				}
				mode := strings.ToLower(strings.TrimSpace(req.Mode))
				if _, ok := ModeRequiredPlayers[mode]; !ok {
					mode = "1v1"
				}
				p.SelectedMode = mode
				p.InQueue = req.Ready

				if p.InQueue {
					p.Hub.tryMatchmakingLocked(mode)
				}
			}
			p.Hub.Mutex.Unlock()
			p.Hub.broadcastLobbyQueues()
		}

		// NGƯỜI CHƠI CHỌN ĐÁP ÁN TRONG MÁY CHỦ CỦA MÌNH
		if req.Action == "submit_answer" {
			p.Hub.Mutex.Lock()
			room := p.Room
			if room != nil && room.Active && !p.Answered && room.CurrentQuestion != nil {
				p.Answered = true
				isCorrect := (req.Chosen == room.CurrentQuestion.Ans)
				scoreGain := 0

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

				if p.Hub.checkRoomAllAnsweredLocked(room) {
					select {
					case room.AllAnsweredChan <- room.Round:
					default:
					}
				}
			}
			p.Hub.Mutex.Unlock()

			if room != nil {
				p.Hub.broadcastRoomState(room)
			}
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
		Hub:          hub,
		Conn:         conn,
		Send:         make(chan []byte, 256),
		Username:     username,
		Score:        0,
		SelectedMode: "1v1",
		InQueue:      false,
		Answered:     false,
	}

	hub.Register <- player

	go player.writePump()
	go player.handleIncomingMessages()
}
