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

type PlayerActivity struct {
	Username      string `json:"username"`
	CurrentAction string `json:"currentAction"`
	LastActive    string `json:"lastActive"`
	Status        string `json:"status"`
}

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
	log.Printf("[Score] Đấu thủ %s nhận được +%d điểm chiến thắng!\n", username, score)
}

func TrackPlayer(username, action, status string) {
	if username == "" || username == "admin" {
		return
	}
	trackLock.Lock()
	defer trackLock.Unlock()
	activeTracking[username] = &PlayerActivity{
		Username:      username,
		CurrentAction: action,
		LastActive:    time.Now().Format("15:04:05 02/01/2006"),
		Status:        status,
	}
}

func trackActivityHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		return
	}
	var req struct {
		Action string `json:"action"`
		Status string `json:"status"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err == nil {
		u := getSessionUsername(r)
		if u != "" {
			TrackPlayer(u, req.Action, req.Status)
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
	tmpl, err := template.ParseFiles("templates/home.html")
	if err != nil {
		http.Redirect(w, r, "/arena", http.StatusSeeOther)
		return
	}
	tmpl.Execute(w, map[string]interface{}{"Username": username})
}

func coursesHandler(w http.ResponseWriter, r *http.Request) {
	username := getSessionUsername(r)
	tmpl, err := template.ParseFiles("templates/courses.html")
	if err != nil {
		http.Redirect(w, r, "/arena", http.StatusSeeOther)
		return
	}
	tmpl.Execute(w, map[string]interface{}{"Username": username})
}

func arenaSelectHandler(w http.ResponseWriter, r *http.Request) {
	username := getSessionUsername(r)
	TrackPlayer(username, "Đang ở sảnh chọn Map", "Online")

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
	TrackPlayer(username, "Đang làm bài Map "+mapID, "Đang giải đố")

	tmpl, err := template.ParseFiles("templates/arena.html")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	tmpl.Execute(w, map[string]interface{}{"Username": username})
}

func speedDuelHandler(w http.ResponseWriter, r *http.Request) {
	username := getSessionUsername(r)
	TrackPlayer(username, "Đang vào Đấu trường 1v1", "Chờ / Thi đấu")

	tmpl, err := template.ParseFiles("templates/speed_duel.html")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	tmpl.Execute(w, map[string]interface{}{"Username": username})
}

func leaderboardHandler(w http.ResponseWriter, r *http.Request) {
	username := getSessionUsername(r)
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

	http.Redirect(w, r, "/arena", http.StatusSeeOther)
}

func main() {
	go GlobalArenaHub.Run()

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

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	log.Printf("🔥 Server đang chạy tại port: %s\n", port)
	if err := http.ListenAndServe(":"+port, nil); err != nil {
		log.Fatal(err)
	}
}