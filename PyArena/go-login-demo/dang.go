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
	users    = make(map[string]User)
	sessions = make(map[string]string) // sessionToken -> username
	mu       sync.Mutex
)

func init() {
	// Tài khoản mẫu ban đầu để test
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
	mu.Lock()
	defer mu.Unlock()
	return sessions[cookie.Value]
}

// ----------------- HANDLERS CÁC TRANG -----------------

// Trang chủ
func homeHandler(w http.ResponseWriter, r *http.Request) {
	username := getSessionUsername(r)
	tmpl, err := template.ParseFiles("templates/home.html")
	if err != nil {
		http.Redirect(w, r, "/arena", http.StatusSeeOther)
		return
	}
	tmpl.Execute(w, map[string]interface{}{"Username": username})
}

// Trang danh sách bài học
func coursesHandler(w http.ResponseWriter, r *http.Request) {
	username := getSessionUsername(r)
	tmpl, err := template.ParseFiles("templates/courses.html")
	if err != nil {
		http.Redirect(w, r, "/arena", http.StatusSeeOther)
		return
	}
	tmpl.Execute(w, map[string]interface{}{"Username": username})
}

// Trang chọn 3 Map Thử Thách (/arena) -> SỬA CHUẨN XÁC VÀO arena_select.html
func arenaSelectHandler(w http.ResponseWriter, r *http.Request) {
	username := getSessionUsername(r)
	tmpl, err := template.ParseFiles("templates/arena_select.html")
	if err != nil {
		http.Error(w, "Lỗi nạp templates/arena_select.html: "+err.Error(), http.StatusInternalServerError)
		return
	}
	tmpl.Execute(w, map[string]interface{}{"Username": username})
}

// Trang chiến đấu trong 3 Map (/arena/battle?map=1/2/3) -> Nạp arena.html
func arenaBattleHandler(w http.ResponseWriter, r *http.Request) {
	username := getSessionUsername(r)
	tmpl, err := template.ParseFiles("templates/arena.html")
	if err != nil {
		http.Error(w, "Lỗi nạp templates/arena.html: "+err.Error(), http.StatusInternalServerError)
		return
	}
	tmpl.Execute(w, map[string]interface{}{"Username": username})
}

// Trang Đấu Trường Siêu Trí Tuệ 1v1 (/arena/speed-duel) -> Nạp speed_duel.html
func speedDuelHandler(w http.ResponseWriter, r *http.Request) {
	username := getSessionUsername(r)
	tmpl, err := template.ParseFiles("templates/speed_duel.html")
	if err != nil {
		http.Error(w, "Lỗi nạp templates/speed_duel.html: "+err.Error(), http.StatusInternalServerError)
		return
	}
	tmpl.Execute(w, map[string]interface{}{"Username": username})
}

// Trang Bảng xếp hạng
func leaderboardHandler(w http.ResponseWriter, r *http.Request) {
	username := getSessionUsername(r)
	tmpl, err := template.ParseFiles("templates/leaderboard.html")
	if err != nil {
		http.Redirect(w, r, "/arena", http.StatusSeeOther)
		return
	}
	tmpl.Execute(w, map[string]interface{}{"Username": username})
}

// Đăng ký
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

	mu.Lock()
	if _, exists := users[username]; exists {
		mu.Unlock()
		http.Error(w, "Tài khoản đã tồn tại", http.StatusBadRequest)
		return
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		mu.Unlock()
		http.Error(w, "Lỗi mã hóa", http.StatusInternalServerError)
		return
	}

	users[username] = User{Username: username, Password: string(hashedPassword)}
	mu.Unlock()

	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

// Đăng nhập
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

	mu.Lock()
	user, exists := users[username]
	mu.Unlock()

	if !exists || bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(password)) != nil {
		http.Error(w, "Sai tài khoản hoặc mật khẩu", http.StatusUnauthorized)
		return
	}

	token := generateToken()
	mu.Lock()
	sessions[token] = username
	mu.Unlock()

	http.SetCookie(w, &http.Cookie{
		Name:    "session_token",
		Value:   token,
		Expires: time.Now().Add(24 * time.Hour),
		Path:    "/",
	})

	http.Redirect(w, r, "/arena", http.StatusSeeOther)
}

func main() {
	// Khởi tạo Hub WebSocket cho Đấu trường 1v1
	go GlobalArenaHub.Run()

	// Định tuyến URL chính xác
	http.HandleFunc("/", homeHandler)
	http.HandleFunc("/courses", coursesHandler)

	// THỰC HÀNH BÀI TẬP: TRỎ ĐÚNG VÀO BẢNG CHỌN 3 MAP
	http.HandleFunc("/arena", arenaSelectHandler)
	http.HandleFunc("/arena/battle", arenaBattleHandler)

	// ĐẤU TRƯỜNG 1V1 LIVE
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
