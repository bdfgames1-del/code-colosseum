package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"
)

type User struct {
	Username string
	Password string
	IsAdmin  bool
}

type ActivityLog struct {
	Time   string `json:"time"`
	Action string `json:"action"`
	Status string `json:"status"`
}

type PlayerActivity struct {
	Username      string        `json:"username"`
	IsOnline      bool          `json:"isOnline"`
	CurrentAction string        `json:"currentAction"`
	LastActive    string        `json:"lastActive"`
	LastExitTime  string        `json:"lastExitTime"`
	Status        string        `json:"status"`
	TotalScore    int           `json:"totalScore"`
	History       []ActivityLog `json:"history"`
}

const playerDBFile = "player_history_db.json"

var (
	users          = make(map[string]User)
	sessions       = make(map[string]string)
	sessionLock    sync.Mutex
	activeTracking = make(map[string]*PlayerActivity)
	trackLock      sync.Mutex
)

func init() {
	hashedUser, _ := bcrypt.GenerateFromPassword([]byte("123456"), bcrypt.DefaultCost)
	users["chienbinh"] = User{Username: "chienbinh", Password: string(hashedUser), IsAdmin: false}

	hashedAdmin, _ := bcrypt.GenerateFromPassword([]byte("admin@2026"), bcrypt.DefaultCost)
	users["admin"] = User{Username: "admin", Password: string(hashedAdmin), IsAdmin: true}

	loadPlayerDataFromFile()
}

func loadPlayerDataFromFile() {
	trackLock.Lock()
	defer trackLock.Unlock()

	data, err := os.ReadFile(playerDBFile)
	if err != nil {
		return
	}
	_ = json.Unmarshal(data, &activeTracking)
}

func savePlayerDataLocked() {
	data, err := json.MarshalIndent(activeTracking, "", "  ")
	if err == nil {
		_ = os.WriteFile(playerDBFile, data, 0644)
	}
}

func generateToken() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func getSessionUsername(r *http.Request) string {
	cookie, err := r.Cookie("session_token")
	if err != nil {
		return ""
	}
	sessionLock.Lock()
	defer sessionLock.Unlock()
	return sessions[cookie.Value]
}

func AddWinScore(username string, score int) {
	if username == "" || username == "admin" {
		return
	}
	nowStr := time.Now().Format("15:04:05 02/01/2006")

	trackLock.Lock()
	defer trackLock.Unlock()

	p, exists := activeTracking[username]
	if !exists {
		p = &PlayerActivity{
			Username:     username,
			LastExitTime: "Chưa thoát",
			History:      []ActivityLog{},
		}
		activeTracking[username] = p
	}

	p.TotalScore += score
	p.LastActive = nowStr
	entry := ActivityLog{
		Time:   nowStr,
		Action: fmt.Sprintf("Hoàn thành trận Đấu Trường (+%d PTS)", score),
		Status: fmt.Sprintf("Tổng điểm: %d PTS", p.TotalScore),
	}
	p.History = append([]ActivityLog{entry}, p.History...)
	if len(p.History) > 100 {
		p.History = p.History[:100]
	}

	savePlayerDataLocked()
}

func TrackPlayer(username, action, status string) {
	if username == "" || username == "admin" {
		return
	}
	nowStr := time.Now().Format("15:04:05 02/01/2006")

	trackLock.Lock()
	defer trackLock.Unlock()

	p, exists := activeTracking[username]
	if !exists {
		p = &PlayerActivity{
			Username:     username,
			LastExitTime: "Chưa thoát",
			History:      []ActivityLog{},
		}
		activeTracking[username] = p
	}

	p.IsOnline = true
	p.CurrentAction = action
	p.LastActive = nowStr
	p.Status = status

	entry := ActivityLog{
		Time:   nowStr,
		Action: action,
		Status: status,
	}
	p.History = append([]ActivityLog{entry}, p.History...)
	if len(p.History) > 100 {
		p.History = p.History[:100]
	}

	savePlayerDataLocked()
}

func TrackPlayerExit(username, reason string) {
	if username == "" || username == "admin" {
		return
	}
	nowStr := time.Now().Format("15:04:05 02/01/2006")

	trackLock.Lock()
	defer trackLock.Unlock()

	p, exists := activeTracking[username]
	if !exists {
		p = &PlayerActivity{
			Username: username,
			History:  []ActivityLog{},
		}
		activeTracking[username] = p
	}

	p.IsOnline = false
	p.CurrentAction = reason
	p.LastActive = nowStr
	p.LastExitTime = nowStr
	p.Status = "Đã thoát game (Offline)"

	entry := ActivityLog{
		Time:   nowStr,
		Action: reason,
		Status: "Đã thoát trò chơi",
	}
	p.History = append([]ActivityLog{entry}, p.History...)
	if len(p.History) > 100 {
		p.History = p.History[:100]
	}

	savePlayerDataLocked()
}

func autoCheckOfflinePlayers() {
	for {
		time.Sleep(15 * time.Second)
		now := time.Now()

		trackLock.Lock()
		changed := false
		for _, p := range activeTracking {
			if p.IsOnline {
				lastTime, err := time.ParseInLocation("15:04:05 02/01/2006", p.LastActive, time.Local)
				if err == nil && now.Sub(lastTime) > 2*time.Minute {
					nowStr := now.Format("15:04:05 02/01/2006")
					p.IsOnline = false
					p.LastExitTime = nowStr
					p.Status = "Đã thoát game (Offline)"
					p.CurrentAction = "Ngắt kết nối / Đóng trình duyệt"
					p.History = append([]ActivityLog{{
						Time:   nowStr,
						Action: "Hệ thống Go phát hiện người chơi đã đóng tab",
						Status: "Đã thoát trò chơi",
					}}, p.History...)
					if len(p.History) > 100 {
						p.History = p.History[:100]
					}
					changed = true
				}
			}
		}
		if changed {
			savePlayerDataLocked()
		}
		trackLock.Unlock()
	}
}

func trackActivityHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		return
	}
	var req struct {
		Username string `json:"username"`
		Action   string `json:"action"`
		Status   string `json:"status"`
		IsExit   bool   `json:"isExit"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err == nil {
		u := getSessionUsername(r)
		if u == "" {
			u = req.Username
		}
		if u != "" {
			if req.IsExit {
				TrackPlayerExit(u, req.Action)
			} else {
				TrackPlayer(u, req.Action, req.Status)
			}
		}
	}
	w.WriteHeader(http.StatusOK)
}

func adminDataHandler(w http.ResponseWriter, r *http.Request) {
	username := getSessionUsername(r)
	sessionLock.Lock()
	user, exists := users[username]
	sessionLock.Unlock()

	if !exists || !user.IsAdmin {
		http.Error(w, "Truy cập bị từ chối!", http.StatusForbidden)
		return
	}

	trackLock.Lock()
	var list []*PlayerActivity
	for _, act := range activeTracking {
		list = append(list, act)
	}
	trackLock.Unlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(list)
}

func adminPageHandler(w http.ResponseWriter, r *http.Request) {
	username := getSessionUsername(r)
	sessionLock.Lock()
	user, exists := users[username]
	sessionLock.Unlock()

	if !exists || !user.IsAdmin {
		http.Error(w, "Chỉ tài khoản Admin mới có quyền truy cập!", http.StatusForbidden)
		return
	}

	tmpl, err := template.ParseFiles("templates/admin.html")
	if err != nil {
		http.Error(w, "Lỗi nạp templates/admin.html: "+err.Error(), http.StatusInternalServerError)
		return
	}
	tmpl.Execute(w, map[string]interface{}{"Username": username})
}

func homeHandler(w http.ResponseWriter, r *http.Request) {
	username := getSessionUsername(r)
	if username != "" {
		TrackPlayer(username, "Đang xem Trang chủ", "Đang Online")
	}
	tmpl, err := template.ParseFiles("templates/home.html")
	if err != nil {
		http.Redirect(w, r, "/arena", http.StatusSeeOther)
		return
	}
	tmpl.Execute(w, map[string]interface{}{"Username": username})
}

func coursesHandler(w http.ResponseWriter, r *http.Request) {
	username := getSessionUsername(r)
	if username != "" {
		TrackPlayer(username, "Đang xem Kho Bài Học Python", "Đang học lý thuyết")
	}
	tmpl, err := template.ParseFiles("templates/courses.html")
	if err != nil {
		http.Redirect(w, r, "/arena", http.StatusSeeOther)
		return
	}
	tmpl.Execute(w, map[string]interface{}{"Username": username})
}

func arenaSelectHandler(w http.ResponseWriter, r *http.Request) {
	username := getSessionUsername(r)
	TrackPlayer(username, "Đang ở sảnh chọn 3 Bản Đồ", "Đang Online")

	tmpl, err := template.ParseFiles("templates/arena_select.html")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	tmpl.Execute(w, map[string]interface{}{"Username": username})
}

func arenaBattleHandler(w http.ResponseWriter, r *http.Request) {
	username := getSessionUsername(r)
	mapID := r.URL.Query().Get("map")
	TrackPlayer(username, "Vào thực hành Bản đồ Map "+mapID, "Đang giải đố")

	tmpl, err := template.ParseFiles("templates/arena.html")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	tmpl.Execute(w, map[string]interface{}{"Username": username})
}

func speedDuelHandler(w http.ResponseWriter, r *http.Request) {
	username := getSessionUsername(r)
	TrackPlayer(username, "Vào Đấu Trường Kiến Thức Python", "Đang ở sảnh đấu trường")

	tmpl, err := template.ParseFiles("templates/speed_duel.html")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	tmpl.Execute(w, map[string]interface{}{"Username": username})
}

func leaderboardHandler(w http.ResponseWriter, r *http.Request) {
	username := getSessionUsername(r)
	if username != "" {
		TrackPlayer(username, "Đang xem Bảng xếp hạng", "Đang Online")
	}
	tmpl, err := template.ParseFiles("templates/leaderboard.html")
	if err != nil {
		http.Redirect(w, r, "/arena", http.StatusSeeOther)
		return
	}
	tmpl.Execute(w, map[string]interface{}{"Username": username})
}

func registerHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		tmpl, err := template.ParseFiles("templates/register.html")
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		tmpl.Execute(w, nil)
		return
	}

	username := r.FormValue("username")
	password := r.FormValue("password")

	if username == "" || password == "" {
		http.Error(w, "Vui lòng nhập đủ thông tin", http.StatusBadRequest)
		return
	}

	sessionLock.Lock()
	if username == "admin" || users[username].Username != "" {
		sessionLock.Unlock()
		http.Error(w, "Tên tài khoản này đã được sử dụng", http.StatusBadRequest)
		return
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		sessionLock.Unlock()
		http.Error(w, "Lỗi mã hóa", http.StatusInternalServerError)
		return
	}

	users[username] = User{Username: username, Password: string(hashedPassword), IsAdmin: false}
	sessionLock.Unlock()

	TrackPlayer(username, "Vừa đăng ký tài khoản mới", "Tài khoản mới")
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

func loginHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		tmpl, err := template.ParseFiles("templates/login.html")
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		tmpl.Execute(w, nil)
		return
	}

	username := r.FormValue("username")
	password := r.FormValue("password")

	sessionLock.Lock()
	user, exists := users[username]
	sessionLock.Unlock()

	if !exists || bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(password)) != nil {
		http.Error(w, "Sai tài khoản hoặc mật khẩu", http.StatusUnauthorized)
		return
	}

	token := generateToken()
	sessionLock.Lock()
	sessions[token] = username
	sessionLock.Unlock()

	http.SetCookie(w, &http.Cookie{
		Name:    "session_token",
		Value:   token,
		Expires: time.Now().Add(24 * time.Hour),
		Path:    "/",
	})

	if user.IsAdmin {
		http.Redirect(w, r, "/admin", http.StatusSeeOther)
		return
	}

	TrackPlayer(username, "Vừa đăng nhập vào hệ thống", "Đang Online")
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// HÀM ĐĂNG XUẤT CHO CẢ ADMIN VÀ NGƯỜI CHƠI
func logoutHandler(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie("session_token")
	if err == nil {
		sessionLock.Lock()
		username := sessions[cookie.Value]
		delete(sessions, cookie.Value)
		sessionLock.Unlock()

		if username != "" && username != "admin" {
			TrackPlayerExit(username, "Chủ động bấm Đăng xuất khỏi tài khoản")
		}
	}

	// Xóa cookie session_token trên trình duyệt
	http.SetCookie(w, &http.Cookie{
		Name:     "session_token",
		Value:    "",
		Path:     "/",
		Expires:  time.Unix(0, 0),
		MaxAge:   -1,
		HttpOnly: true,
	})

	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

func main() {
	go GlobalArenaHub.Run()
	go autoCheckOfflinePlayers()

	http.HandleFunc("/", homeHandler)
	http.HandleFunc("/courses", coursesHandler)
	http.HandleFunc("/arena", arenaSelectHandler)
	http.HandleFunc("/arena/battle", arenaBattleHandler)
	http.HandleFunc("/arena/speed-duel", speedDuelHandler)
	http.HandleFunc("/ws/arena", func(w http.ResponseWriter, r *http.Request) {
		username := getSessionUsername(r)
		if username == "" {
			username = fmt.Sprintf("Khách #%d", time.Now().Unix()%1000)
		}
		ServeArenaWs(GlobalArenaHub, w, r, username)
	})

	http.HandleFunc("/admin", adminPageHandler)
	http.HandleFunc("/api/admin/active-players", adminDataHandler)
	http.HandleFunc("/api/track-activity", trackActivityHandler)

	http.HandleFunc("/leaderboard", leaderboardHandler)
	http.HandleFunc("/login", loginHandler)
	http.HandleFunc("/register", registerHandler)
	http.HandleFunc("/logout", logoutHandler) // <-- Đường dẫn Đăng xuất

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	log.Printf("🔥 Server đang chạy tại port: %s\n", port)
	if err := http.ListenAndServe(":"+port, nil); err != nil {
		log.Fatal(err)
	}
}
