package config

import (
	"github.com/gorilla/sessions"
)

var Store *sessions.CookieStore

func InitSession(secret string, secure bool) {
	Store = sessions.NewCookieStore([]byte(secret))
	Store.Options = &sessions.Options{
		Path:     "/",
		MaxAge:   28800, // [Security] 8 hours instead of 14 days
		HttpOnly: true,
		Secure:   secure,
		SameSite: 3, // [Security] SameSite=Strict (was Lax)
	}
}
