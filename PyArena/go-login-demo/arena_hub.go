package main

import (
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

// Cấu trúc câu hỏi thi đấu Siêu Trí Tuệ Python (3-5 giây)
type TriviaQuestion struct {
	ID        int      `json:"id"`
	Question  string   `json:"question"`
	Code      string   `json:"code"`
	Options   []string `json:"options"`
	AnswerIdx int      `json:"answerIdx"`
	TimeLimit int      `json:"timeLimit"`
}

// Danh sách câu hỏi cú pháp chuẩn Go
var TRIVIA_QUESTIONS = []TriviaQuestion{
	{
		ID:        1,
		Question:  "Giá trị của biểu thức sau là gì?",
		Code:      "print(2 ** 3 ** 2)",
		Options:   []string{"64", "512", "128", "256"},
		AnswerIdx: 1,
		TimeLimit: 5,
	},
	{
		ID:        2,
		Question:  "Kết quả của đoạn mã sau là gì?",
		Code:      "print([1, 2, 3] * 2)",
		Options:   []string{"[2, 4, 6]", "[1, 2, 3, 1, 2, 3]", "[1, 2, 3, 2]", "Error"},
		AnswerIdx: 1,
		TimeLimit: 4,
	},
	{
		ID:        3,
		Question:  "Biểu thức sau trả về kiểu dữ liệu gì?",
		Code:      "type(3 / 3)",
		Options:   []string{"<class 'int'>", "<class 'float'>", "<class 'bool'>", "<class 'number'>"},
		AnswerIdx: 1,
		TimeLimit: 3,
	},
	{
		ID:        4,
		Question:  "Kết quả của toán tử logic sau là gì?",
		Code:      "print(bool([] or '0'))",
		Options:   []string{"False", "True", "None", "[]"},
		AnswerIdx: 1,
		TimeLimit: 4,
	},
	{
		ID:        5,
		Question:  "Kết quả của cú pháp slice sau là gì?",
		Code:      "s = 'Python'[::-2]; print(s)",
		Options:   []string{"nhy", "noP", "nhyP", "Pto"},
		AnswerIdx: 1,
		TimeLimit: 5,
	},
	{
		ID:        6,
		Question:  "Giá trị in ra màn hình là bao nhiêu?",
		Code:      "print(sum(range(1, 5)))",
		Options:   []string{"10", "15", "6", "14"},
		AnswerIdx: 0,
		TimeLimit: 4,
	},
	{
		ID:        7,
		Question:  "Hàm sau trả về bao nhiêu phần tử?",
		Code:      "print(len({1, 2, 2, 3, 3, 3}))",
		Options:   []string{"6", "3", "4", "Error"},
		AnswerIdx: 1,
		TimeLimit: 3,
	},
	{
		ID:        8,
		Question:  "Kết quả in ra của phép chia lấy dư là gì?",
		Code:      "print(-7 % 3)",
		Options:   []string{"-1", "2", "1", "-2"},
		AnswerIdx: 1,
		TimeLimit: 4,
	},
	{
		ID:        9,
		Question:  "Giá trị của x sau vòng lặp là gì?",
		Code:      "x = 1\nfor i in range(3): x *= 2\nprint(x)",
		Options:   []string{"6", "8", "4", "16"},
		AnswerIdx: 1,
		TimeLimit: 4,
	},
	{
		ID:        10,
		Question:  "Kết quả của lệnh gán tuple sau là gì?",
		Code:      "a, *b, c = [1, 2, 3, 4, 5]\nprint(b)",
		Options:   []string{"[2, 3, 4]", "(2, 3, 4)", "[3, 4]", "2, 3, 4"},
		AnswerIdx: 0,
		TimeLimit: 5,
	},
}

// Thông tin người chơi
type Player struct {
	Username  string
	Conn      *websocket.Conn
	MapID     int
	Step      int
	Room      *Room
	LastChar  int
	Submitted bool

	Lives     int
	Score     int
	IsAlive   bool
	Answered  bool
	SubmitMs  int64
	MultiRoom *MultiplayerRoom
}

// Phòng đấu Map 1-3 cũ (1v1)
type Room struct {
	ID            string
	MapID         int
	Player1       *Player
	Player2       *Player
	CurrentRound  int
	RoundAnswered bool
	TugPosition   int
	Lock          sync.Mutex
}

// Phòng đấu Đấu Trường Siêu Trí Tuệ (1v1 Solo - 2 Người)
type MultiplayerRoom struct {
	ID            string
	Players       map[*Player]bool
	CurrentQIndex int
	RoundStarted  time.Time
	TimeLimitSec  int
	IsActive      bool
	Lock          sync.Mutex
}

var (
	mapWaitingQueues = make(map[int][]*Player)
	queueLock        sync.Mutex

	globalMultiRoom *MultiplayerRoom
	multiRoomLock   sync.Mutex
)

func sendMsg(p *Player, action string, data map[string]interface{}) {
	if p == nil || p.Conn == nil {
		return
	}
	payload := map[string]interface{}{
		"action": action,
		"data":   data,
	}
	_ = p.Conn.WriteJSON(payload)
}

func wsArenaHandler(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("Lỗi Upgrade:", err)
		return
	}

	mode := r.URL.Query().Get("mode")
	mapIDStr := r.URL.Query().Get("map")
	mapID, _ := strconv.Atoi(mapIDStr)
	if mapID < 1 {
		mapID = 1
	}

	username := "Đấu thủ"
	cookie, err := r.Cookie("session_token")
	if err == nil {
		sessionLock.RLock()
		if u, exists := sessions[cookie.Value]; exists {
			username = u
		}
		sessionLock.RUnlock()
	} else {
		username = fmt.Sprintf("NgườiChơi_%d", rand.Intn(900)+100)
	}

	player := &Player{
		Username: username,
		Conn:     conn,
		MapID:    mapID,
		Step:     10,
		Lives:    3,
		Score:    0,
		IsAlive:  true,
	}

	// Đấu Trường Siêu Trí Tuệ 1v1 (2 Người)
	if mode == "super_mind" {
		joinMultiplayerSuperMind(player)
		for {
			var msg map[string]interface{}
			if err := conn.ReadJSON(&msg); err != nil {
				handleMultiDisconnect(player)
				break
			}
			action, _ := msg["action"].(string)
			if action == "submit_answer" {
				chosenIdx := int(msg["chosenIdx"].(float64))
				clientSubmitMs := int64(msg["submitMs"].(float64))
				handleMultiAnswer(player, chosenIdx, clientSubmitMs)
			}
		}
		return
	}

	// Map 1-3 thông thường
	matchPlayerByMap(player)
	for {
		var msg map[string]interface{}
		if err := conn.ReadJSON(&msg); err != nil {
			handleDisconnect(player)
			break
		}
		action, _ := msg["action"].(string)
		switch action {
		case "submit_golf":
			codeLen := int(msg["codeLen"].(float64))
			handleGolfSubmit(player, codeLen)
		case "answer":
			isCorrect, _ := msg["isCorrect"].(bool)
			handleAnswer(player, isCorrect)
		}
	}
}

// Tham gia phòng đấu 2 người
func joinMultiplayerSuperMind(p *Player) {
	multiRoomLock.Lock()
	defer multiRoomLock.Unlock()

	if globalMultiRoom == nil || globalMultiRoom.IsActive || len(globalMultiRoom.Players) >= 2 {
		globalMultiRoom = &MultiplayerRoom{
			ID:            fmt.Sprintf("Room_%d", time.Now().Unix()),
			Players:       make(map[*Player]bool),
			CurrentQIndex: -1,
			IsActive:      false,
		}
	}

	room := globalMultiRoom
	room.Players[p] = true
	p.MultiRoom = room

	broadcastLobbyState(room)

	// Đủ 2 người chơi là tự động đếm ngược 3 giây vào trận
	if len(room.Players) >= 2 && !room.IsActive {
		room.IsActive = true
		go func() {
			for c := 3; c > 0; c-- {
				broadcastToRoom(room, "countdown_start", map[string]interface{}{"seconds": c})
				time.Sleep(1 * time.Second)
			}
			startNextMultiRound(room)
		}()
	} else if len(room.Players) < 2 && !room.IsActive {
		sendMsg(p, "waiting_players", map[string]interface{}{
			"currentCount": len(room.Players),
			"need":         2,
			"msg":          "Đang tìm kiếm đối thủ thứ 2...",
		})
	}
}

func broadcastLobbyState(r *MultiplayerRoom) {
	r.Lock.Lock()
	defer r.Lock.Unlock()

	var pList []map[string]interface{}
	for p := range r.Players {
		pList = append(pList, map[string]interface{}{
			"username": p.Username,
			"lives":    p.Lives,
			"score":    p.Score,
			"isAlive":  p.IsAlive,
		})
	}

	for p := range r.Players {
		sendMsg(p, "lobby_state", map[string]interface{}{
			"totalPlayers": len(r.Players),
			"players":      pList,
		})
	}
}

func broadcastToRoom(r *MultiplayerRoom, action string, data map[string]interface{}) {
	r.Lock.Lock()
	defer r.Lock.Unlock()
	for p := range r.Players {
		sendMsg(p, action, data)
	}
}

func startNextMultiRound(r *MultiplayerRoom) {
	r.Lock.Lock()
	r.CurrentQIndex++
	if r.CurrentQIndex >= len(TRIVIA_QUESTIONS) {
		r.Lock.Unlock()
		endMultiGame(r, "TẤT CẢ VÒNG THI ĐÃ HOÀN TẤT!")
		return
	}

	q := TRIVIA_QUESTIONS[r.CurrentQIndex]
	r.RoundStarted = time.Now()
	r.TimeLimitSec = q.TimeLimit

	for p := range r.Players {
		p.Answered = false
		p.SubmitMs = 99999
	}
	r.Lock.Unlock()

	qData, _ := json.Marshal(q)
	var qMap map[string]interface{}
	_ = json.Unmarshal(qData, &qMap)
	delete(qMap, "answerIdx")

	broadcastToRoom(r, "new_round_question", map[string]interface{}{
		"round":     r.CurrentQIndex + 1,
		"question":  qMap,
		"timeLimit": q.TimeLimit,
	})

	go func(qIdx int, limit int) {
		time.Sleep(time.Duration(limit)*time.Second + 300*time.Millisecond)

		r.Lock.Lock()
		if r.CurrentQIndex != qIdx {
			r.Lock.Unlock()
			return
		}

		for p := range r.Players {
			if p.IsAlive && !p.Answered {
				p.Lives--
				if p.Lives <= 0 {
					p.IsAlive = false
					sendMsg(p, "player_eliminated", map[string]interface{}{"reason": "Hết máu do quá giờ!"})
				}
			}
		}
		r.Lock.Unlock()

		processRoundResults(r)
	}(r.CurrentQIndex, q.TimeLimit)
}

func handleMultiAnswer(p *Player, chosenIdx int, clientSubmitMs int64) {
	r := p.MultiRoom
	if r == nil || !p.IsAlive || p.Answered {
		return
	}

	r.Lock.Lock()
	p.Answered = true
	q := TRIVIA_QUESTIONS[r.CurrentQIndex]
	elapsedMs := time.Since(r.RoundStarted).Milliseconds()
	if clientSubmitMs > 0 && clientSubmitMs < elapsedMs {
		elapsedMs = clientSubmitMs
	}
	p.SubmitMs = elapsedMs

	if chosenIdx == q.AnswerIdx {
		baseScore := 100
		speedBonus := int(float64(q.TimeLimit*1000-int(elapsedMs)) / 10.0)
		if speedBonus < 0 {
			speedBonus = 0
		}
		p.Score += (baseScore + speedBonus)
		sendMsg(p, "answer_feedback", map[string]interface{}{
			"correct":     true,
			"scoreGained": baseScore + speedBonus,
			"speedMs":     elapsedMs,
		})
	} else {
		p.Lives--
		if p.Lives <= 0 {
			p.IsAlive = false
			sendMsg(p, "player_eliminated", map[string]interface{}{"reason": "Hết máu do trả lời sai!"})
		} else {
			sendMsg(p, "answer_feedback", map[string]interface{}{
				"correct": false,
				"lives":   p.Lives,
			})
		}
	}
	r.Lock.Unlock()

	r.Lock.Lock()
	allAnswered := true
	for pl := range r.Players {
		if pl.IsAlive && !pl.Answered {
			allAnswered = false
			break
		}
	}
	r.Lock.Unlock()

	if allAnswered {
		processRoundResults(r)
	}
}

func processRoundResults(r *MultiplayerRoom) {
	r.Lock.Lock()
	defer r.Lock.Unlock()

	var leaderboard []map[string]interface{}
	aliveCount := 0
	var lastAlivePlayer *Player

	for p := range r.Players {
		if p.IsAlive {
			aliveCount++
			lastAlivePlayer = p
		}
		leaderboard = append(leaderboard, map[string]interface{}{
			"username": p.Username,
			"score":    p.Score,
			"lives":    p.Lives,
			"isAlive":  p.IsAlive,
			"speedMs":  p.SubmitMs,
		})
	}

	for p := range r.Players {
		sendMsg(p, "round_ended", map[string]interface{}{
			"leaderboard": leaderboard,
			"aliveCount":  aliveCount,
		})
	}

	// Phân định thắng thua 1v1 khi còn 1 người sống sót
	if aliveCount <= 1 && len(r.Players) > 1 {
		winnerName := "Không có ai"
		if lastAlivePlayer != nil {
			winnerName = lastAlivePlayer.Username
			AddWinScore(winnerName)
		}
		go endMultiGame(r, fmt.Sprintf("Trận đấu kết thúc! Người chiến thắng là: %s 🏆", winnerName))
		return
	}

	go func() {
		time.Sleep(3 * time.Second)
		startNextMultiRound(r)
	}()
}

func endMultiGame(r *MultiplayerRoom, msg string) {
	r.Lock.Lock()
	defer r.Lock.Unlock()
	for p := range r.Players {
		sendMsg(p, "battle_royale_game_over", map[string]interface{}{
			"msg": msg,
		})
	}
	r.IsActive = false
}

func handleMultiDisconnect(p *Player) {
	r := p.MultiRoom
	if r != nil {
		r.Lock.Lock()
		delete(r.Players, p)
		r.Lock.Unlock()
		broadcastLobbyState(r)
	}
}

// ----------------- CÁC MAP THỰC HÀNH CŨ -----------------

func matchPlayerByMap(p *Player) {
	queueLock.Lock()
	defer queueLock.Unlock()

	queue := mapWaitingQueues[p.MapID]
	if len(queue) > 0 {
		opponent := queue[0]
		mapWaitingQueues[p.MapID] = queue[1:]

		room := &Room{
			MapID:        p.MapID,
			Player1:      opponent,
			Player2:      p,
			CurrentRound: 1,
			TugPosition:  0,
		}
		opponent.Room = room
		p.Room = room

		sendMsg(opponent, "matched", map[string]interface{}{"opponent": p.Username, "isPlayer1": true})
		sendMsg(p, "matched", map[string]interface{}{"opponent": opponent.Username, "isPlayer1": false})

		go func() {
			time.Sleep(1500 * time.Millisecond)
			startNewRound(room)
		}()
	} else {
		mapWaitingQueues[p.MapID] = append(mapWaitingQueues[p.MapID], p)
	}
}

func startNewRound(r *Room) {
	r.Lock.Lock()
	r.RoundAnswered = false
	r.Player1.Submitted = false
	r.Player2.Submitted = false
	r.Lock.Unlock()

	sendMsg(r.Player1, "next_round", map[string]interface{}{"round": r.CurrentRound})
	sendMsg(r.Player2, "next_round", map[string]interface{}{"round": r.CurrentRound})
}

func handleGolfSubmit(p *Player, codeLen int) {
	room := p.Room
	if room == nil {
		return
	}
	p.LastChar = codeLen
	p.Submitted = true
	var opp *Player
	if room.Player1 == p {
		opp = room.Player2
	} else {
		opp = room.Player1
	}

	if p.Submitted && opp.Submitted {
		sendMsg(p, "golf_round_result", map[string]interface{}{"yourLen": p.LastChar, "oppLen": opp.LastChar})
		sendMsg(opp, "golf_round_result", map[string]interface{}{"yourLen": opp.LastChar, "oppLen": p.LastChar})
		room.CurrentRound++
		go func() {
			time.Sleep(2 * time.Second)
			startNewRound(room)
		}()
	}
}

func handleAnswer(p *Player, isCorrect bool) {
	room := p.Room
	if room == nil {
		return
	}
	var opp *Player
	if room.Player1 == p {
		opp = room.Player2
	} else {
		opp = room.Player1
	}
	if isCorrect {
		opp.Step--
	} else {
		p.Step--
	}
	sendMsg(p, "round_result", map[string]interface{}{"yourStep": p.Step, "oppStep": opp.Step})
	sendMsg(opp, "round_result", map[string]interface{}{"yourStep": opp.Step, "oppStep": p.Step})
}

func handleDisconnect(p *Player) {
	queueLock.Lock()
	queue := mapWaitingQueues[p.MapID]
	for i, qp := range queue {
		if qp == p {
			mapWaitingQueues[p.MapID] = append(queue[:i], queue[i+1:]...)
			break
		}
	}
	queueLock.Unlock()
}