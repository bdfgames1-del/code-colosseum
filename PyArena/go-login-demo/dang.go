package main

import (
	"crypto/rand"
	"encoding/hex"
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
}

var (
	users       = make(map[string]User)
	sessions    = make(map[string]string)
	sessionLock sync.Mutex
)

func init() {
	hashedPwd, _ := bcrypt.GenerateFromPassword([]byte("123456"), bcrypt.DefaultCost)
	users["chienbinh"] = User{Username: "chienbinh", Password: string(hashedPwd)}
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

// Hàm hỗ trợ cộng điểm xếp hạng
func AddWinScore(username string, score int) {
	// Dùng để mở rộng lưu database/leaderboard nếu cần
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

// Bấm "Thực hành bài tập" (/arena) -> Trỏ chuẩn xác về trang 3 Map arena_select.html
func arenaSelectHandler(w http.ResponseWriter, r *http.Request) {
	username := getSessionUsername(r)
	tmpl, err := template.ParseFiles("templates/arena_select.html")
	if err != nil {
		http.Error(w, "Lỗi nạp templates/arena_select.html: "+err.Error(), http.StatusInternalServerError)
		return
	}
	tmpl.Execute(w, map[string]interface{}{"Username": username})
}

// Chơi từng Map 1, 2, 3
func arenaBattleHandler(w http.ResponseWriter, r *http.Request) {
	username := getSessionUsername(r)
	tmpl, err := template.ParseFiles("templates/arena.html")
	if err != nil {
		http.Error(w, "Lỗi nạp templates/arena.html: "+err.Error(), http.StatusInternalServerError)
		return
	}
	tmpl.Execute(w, map[string]interface{}{"Username": username})
}

// Bấm "Đấu trường LIVE" (/arena/speed-duel) -> Nạp speed_duel.html
func speedDuelHandler(w http.ResponseWriter, r *http.Request) {
	username := getSessionUsername(r)
	tmpl, err := template.ParseFiles("templates/speed_duel.html")
	if err != nil {
		http.Error(w, "Lỗi nạp templates/speed_duel.html: "+err.Error(), http.StatusInternalServerError)
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
	if _, exists := users[username]; exists {
		sessionLock.Unlock()
		http.Error(w, "Tài khoản đã tồn tại", http.StatusBadRequest)
		return
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		sessionLock.Unlock()
		http.Error(w, "Lỗi mã hóa", http.StatusInternalServerError)
		return
	}

	users[username] = User{Username: username, Password: string(hashedPassword)}
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

	http.Redirect(w, r, "/arena", http.StatusSeeOther)
}

func main() {
	go GlobalArenaHub.Run()

	http.HandleFunc("/", homeHandler)
	http.HandleFunc("/courses", coursesHandler)

	// Route Thực hành bài tập trỏ chuẩn xác vào danh sách 3 Map
	http.HandleFunc("/arena", arenaSelectHandler)
	http.HandleFunc("/arena/battle", arenaBattleHandler)

	// Route Đấu trường 1v1
	http.HandleFunc("/arena/speed-duel", speedDuelHandler)
	http.HandleFunc("/ws/arena", func(w http.ResponseWriter, r *http.Request) {
		username := getSessionUsername(r)
		if username == "" {
			username = fmt.Sprintf("Đấu thủ #%d", time.Now().Unix()%1000)
		}
		ServeArenaWs(GlobalArenaHub, w, r, username)
	})

	http.HandleFunc("/leaderboard", leaderboardHandler)
	http.HandleFunc("/login", loginHandler)
	http.HandleFunc("/register", registerHandler)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	log.Printf("🔥 CodeColosseum Server đang chạy tại port: %s\n", port)
	if err := http.ListenAndServe(":"+port, nil); err != nil {
		log.Fatal("Lỗi khởi động Server: ", err)
	}
}
