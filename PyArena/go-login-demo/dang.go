package main

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"
)

type UserAccount struct {
	Username     string
	PasswordHash string
	EloScore     int
	CreatedAt    time.Time
}

var (
	users       = make(map[string]*UserAccount)
	usersLock   sync.RWMutex
	sessions    = make(map[string]string)
	sessionLock sync.RWMutex
)

func init() {
	hashedPwd, _ := bcrypt.GenerateFromPassword([]byte("123456"), bcrypt.DefaultCost)
	users["admin"] = &UserAccount{
		Username:     "admin",
		PasswordHash: string(hashedPwd),
		EloScore:     1250,
		CreatedAt:    time.Now(),
	}
}

func AddWinScore(username string) {
	usersLock.Lock()
	defer usersLock.Unlock()
	if u, exists := users[username]; exists {
		u.EloScore += 25
	}
}

func generateSecureToken() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func getCurrentUser(r *http.Request) string {
	cookie, err := r.Cookie("session_token")
	if err != nil {
		return ""
	}
	sessionLock.RLock()
	defer sessionLock.RUnlock()
	return sessions[cookie.Value]
}

func main() {
	fs := http.FileServer(http.Dir("static"))
	http.Handle("/static/", http.StripPrefix("/static/", fs))

	// 1. Trang chủ (templates/home.html)
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		tmpl, err := template.ParseFiles("templates/home.html")
		if err != nil {
			http.Error(w, "Lỗi đọc file templates/home.html: "+err.Error(), http.StatusInternalServerError)
			return
		}
		tmpl.Execute(w, map[string]interface{}{
			"Username": getCurrentUser(r),
		})
	})

	// 2. Đăng nhập
	http.HandleFunc("/login", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			tmpl, err := template.ParseFiles("templates/login.html")
			if err != nil {
				fmt.Fprint(w, `<h2>Đăng nhập</h2><form method="POST"><input name="username" placeholder="Tên đăng nhập"><input type="password" name="password"><button type="submit">Đăng nhập</button></form>`)
				return
			}
			tmpl.Execute(w, nil)
			return
		}

		if r.Method == http.MethodPost {
			username := r.FormValue("username")
			password := r.FormValue("password")

			usersLock.RLock()
			user, exists := users[username]
			usersLock.RUnlock()

			if !exists || bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)) != nil {
				http.Error(w, "Sai tài khoản hoặc mật khẩu!", http.StatusUnauthorized)
				return
			}

			token := generateSecureToken()
			sessionLock.Lock()
			sessions[token] = username
			sessionLock.Unlock()

			http.SetCookie(w, &http.Cookie{
				Name:     "session_token",
				Value:    token,
				Path:     "/",
				HttpOnly: true,
				Expires:  time.Now().Add(24 * time.Hour),
			})

			http.Redirect(w, r, "/arena", http.StatusSeeOther)
		}
	})

	// 3. Đăng ký
	http.HandleFunc("/register", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			tmpl, err := template.ParseFiles("templates/register.html")
			if err != nil {
				fmt.Fprint(w, `<h2>Đăng ký</h2><form method="POST"><input name="username" placeholder="Tên đăng nhập"><input type="password" name="password"><button type="submit">Đăng ký</button></form>`)
				return
			}
			tmpl.Execute(w, nil)
			return
		}

		if r.Method == http.MethodPost {
			username := r.FormValue("username")
			password := r.FormValue("password")

			usersLock.Lock()
			if _, exists := users[username]; exists {
				usersLock.Unlock()
				http.Error(w, "Tên tài khoản đã tồn tại!", http.StatusBadRequest)
				return
			}

			hashedPwd, _ := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
			users[username] = &UserAccount{
				Username:     username,
				PasswordHash: string(hashedPwd),
				EloScore:     1000,
				CreatedAt:    time.Now(),
			}
			usersLock.Unlock()

			http.Redirect(w, r, "/login", http.StatusSeeOther)
		}
	})

	// 4. Đăng xuất
	http.HandleFunc("/logout", func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie("session_token")
		if err == nil {
			sessionLock.Lock()
			delete(sessions, cookie.Value)
			sessionLock.Unlock()
		}
		http.SetCookie(w, &http.Cookie{
			Name:     "session_token",
			Value:    "",
			Path:     "/",
			HttpOnly: true,
			MaxAge:   -1,
		})
		http.Redirect(w, r, "/", http.StatusSeeOther)
	})

	// 5. Danh mục bài học
	http.HandleFunc("/courses", func(w http.ResponseWriter, r *http.Request) {
		tmpl, err := template.ParseFiles("templates/courses.html")
		if err != nil {
			http.Redirect(w, r, "/arena", http.StatusSeeOther)
			return
		}
		tmpl.Execute(w, map[string]interface{}{
			"Username": getCurrentUser(r),
		})
	})

	// 6. Chi tiết bài học
	http.HandleFunc("/lesson", func(w http.ResponseWriter, r *http.Request) {
		tmpl, err := template.ParseFiles("templates/lesson.html")
		if err != nil {
			http.Redirect(w, r, "/courses", http.StatusSeeOther)
			return
		}
		tmpl.Execute(w, map[string]interface{}{
			"Username": getCurrentUser(r),
		})
	})

	// 7. Bảng xếp hạng Elo
	http.HandleFunc("/leaderboard", func(w http.ResponseWriter, r *http.Request) {
		tmpl, err := template.ParseFiles("templates/leaderboard.html")
		if err != nil {
			http.Redirect(w, r, "/arena", http.StatusSeeOther)
			return
		}
		tmpl.Execute(w, map[string]interface{}{
			"Username": getCurrentUser(r),
		})
	})

	// 8. Thực Hành Bài Tập (Các Map 1->10)
	http.HandleFunc("/arena", func(w http.ResponseWriter, r *http.Request) {
		tmpl, err := template.ParseFiles("templates/arena_select.html")
		if err != nil {
			http.Error(w, "Không tìm thấy templates/arena_select.html", http.StatusInternalServerError)
			return
		}
		tmpl.Execute(w, map[string]interface{}{
			"Username": getCurrentUser(r),
		})
	})

	// 9. Phòng thi đấu Map 1, 2, 3 (/arena/battle?map=X)
	http.HandleFunc("/arena/battle", func(w http.ResponseWriter, r *http.Request) {
		tmpl, err := template.ParseFiles("templates/arena.html")
		if err != nil {
			http.Error(w, "Không tìm thấy templates/arena.html", http.StatusInternalServerError)
			return
		}
		tmpl.Execute(w, map[string]interface{}{
			"Username": getCurrentUser(r),
		})
	})

	// 10. ĐẤU TRƯỜNG TRỰC TIẾP (Speed Coding Duel)
	http.HandleFunc("/arena/speed-duel", func(w http.ResponseWriter, r *http.Request) {
		tmpl, err := template.ParseFiles("templates/speed_duel.html")
		if err != nil {
			http.Error(w, "Không tìm thấy templates/speed_duel.html", http.StatusInternalServerError)
			return
		}
		tmpl.Execute(w, map[string]interface{}{
			"Username": getCurrentUser(r),
		})
	})

	// 11. WebSocket Handler
	http.HandleFunc("/ws/arena", wsArenaHandler)

	port := ":8080"
	fmt.Printf("🚀 Server đang chạy tại http://localhost%s\n", port)
	if err := http.ListenAndServe(port, nil); err != nil {
		log.Fatal(err)
	}
}
