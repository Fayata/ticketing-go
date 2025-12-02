package controllers

import (
	"net/http"
	"ticketing/config"
	"ticketing/services"
	"ticketing/utils"
)

type AuthController struct {
	authService *services.AuthService
}

func NewAuthController(authService *services.AuthService) *AuthController {
	return &AuthController{authService: authService}
}

func (c *AuthController) Login(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		// Gunakan utils.RenderTemplate
		utils.RenderTemplate(w, "login.html", map[string]interface{}{
			"title":      "Login - Portal Ticketing",
			"query_next": r.URL.Query().Get("next"),
		})
		return
	}

	if r.Method == http.MethodPost {
		r.ParseForm()
		username := r.FormValue("username")
		password := r.FormValue("password")
		nextParam := r.FormValue("next")

		user, err := c.authService.Authenticate(username, password)
		if err != nil {
			// Gunakan utils.RenderTemplate
			utils.RenderTemplate(w, "login.html", map[string]interface{}{
				"error":            err.Error(),
				"entered_username": username,
				"query_next":       nextParam,
			})
			return
		}

		// Set Session
		sess, _ := config.Store.Get(r, "session")
		sess.Values["user_id"] = user.ID
		sess.Values["username"] = user.Username
		sess.Save(r, w)

		if nextParam != "" {
			http.Redirect(w, r, nextParam, http.StatusSeeOther)
			return
		}
		http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
	}
}

func (c *AuthController) Register(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		utils.RenderTemplate(w, "register.html", nil)

		return
	}

	if r.Method == http.MethodPost {
		r.ParseForm()
		err := c.authService.RegisterUser(r.FormValue("username"), r.FormValue("email"), r.FormValue("password1"))
		if err != nil {
			utils.RenderTemplate(w, "register.html", map[string]interface{}{ // Gunakan utils.RenderTemplate
				"errors": map[string]string{"register": err.Error()},
			})
			return
		}
		http.Redirect(w, r, "/login?success=Cek+email+untuk+verifikasi", http.StatusSeeOther)
	}
}

func (c *AuthController) VerifyEmail(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	if err := c.authService.VerifyEmail(token); err != nil {
		http.Redirect(w, r, "/login?error=Verifikasi+gagal+atau+token+expired", http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, "/login?success=Email+terverifikasi.+Silakan+login", http.StatusSeeOther)
}

func (c *AuthController) Logout(w http.ResponseWriter, r *http.Request) {
	sess, _ := config.Store.Get(r, "session")
	sess.Options.MaxAge = -1
	sess.Save(r, w)
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}
