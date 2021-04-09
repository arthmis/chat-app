package auth

import (
	// "chat/chatroom"

	"chat/applog"
	"chat/chatroom"
	"chat/database"
	"chat/validate"
	"context"
	"database/sql"
	"html/template"
	"net/http"

	"github.com/davecgh/go-spew/spew"
	"github.com/gorilla/schema"
	"github.com/gorilla/sessions"
	"golang.org/x/crypto/bcrypt"
)

var Decoder = schema.NewDecoder()
var Tmpl *template.Template

type UserSignup struct {
	Email           string `form:"email" validate:"required,email,max=50"`
	Username        string `form:"username" validate:"required,min=3,max=30"`
	Password        string `form:"password" validate:"required,eqfield=ConfirmPassword,min=8,max=50"`
	ConfirmPassword string `form:"confirmPassword" validate:"required,min=8,max=50"`
}

type UserLogin struct {
	Email    string `form:"email" validate:"required,email,max=50"`
	Password string `form:"password" validate:"required,min=8,max=50"`
}

func Signup(w http.ResponseWriter, req *http.Request) {
	var form UserSignup
	err := req.ParseForm()
	if err != nil {
		applog.Sugar.Error("Error parsing form: ", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	err = Decoder.Decode(&form, req.PostForm)
	if err != nil {
		applog.Sugar.Error("err decoding form in signup: ", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	err = validate.Validate.Struct(form)
	if err != nil {
		applog.Sugar.Error("err validating form in signup: ", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(form.Password), bcrypt.MinCost)
	if err != nil {
		applog.Sugar.Error("err generating from password: ", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	emailExists, usernameExists := checkUserExists(req.Context(), form.Email, form.Username)

	userExists := struct {
		Username       string
		Email          string
		UsernameExists string
		EmailExists    string
	}{
		form.Username,
		form.Email,
		"Username already exists",
		"Email already exists",
	}

	if usernameExists && emailExists {
		w.WriteHeader(http.StatusOK)
		err = Tmpl.ExecuteTemplate(w, "signup.html", userExists)
		if err != nil {
			applog.Sugar.Error("error executing template: ", err)
		}

		return
	} else if usernameExists {
		userExists.EmailExists = ""
		w.WriteHeader(http.StatusOK)
		err = Tmpl.ExecuteTemplate(w, "signup.html", userExists)
		if err != nil {
			applog.Sugar.Error("error executing template: ", err)
		}

		return
	} else if emailExists {
		userExists.UsernameExists = ""
		w.WriteHeader(http.StatusOK)
		err = Tmpl.ExecuteTemplate(w, "signup.html", userExists)
		if err != nil {
			applog.Sugar.Error("error executing template: ", err)
		}

		return
	}

	if err == nil {
		_, err = database.PgDB.Exec(
			`INSERT INTO Users (email, username, password) VALUES ($1, $2, $3)`,
			form.Email,
			form.Username,
			string(hash),
		)
		if err != nil {
			applog.Sugar.Error("error inserting new user: ", err)
		}
	} else {
		applog.Sugar.Error("error inserting values: \n", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	chatroom.Clients[form.Username] = &chatroom.User{}

	w.WriteHeader(http.StatusCreated)
	err = Tmpl.ExecuteTemplate(w, "login.html", nil)
	if err != nil {
		applog.Sugar.Error("error executing template: ", err)
	}
}

func Login(w http.ResponseWriter, req *http.Request) {
	var form UserLogin
	err := req.ParseForm()
	if err != nil {
		applog.Sugar.Error("err parsing form data: ", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	err = Decoder.Decode(&form, req.PostForm)
	if err != nil {
		applog.Sugar.Error("err decoding post form: ", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	loginSuccess := struct {
		Email        string
		ErrorMessage string
	}{
		form.Email,
		"Email or Password is incorrect",
	}

	row := database.PgDB.QueryRow(
		`SELECT password FROM Users WHERE email=$1`,
		form.Email,
	)

	var password string
	err = row.Scan(&password)
	if err == sql.ErrNoRows {
		w.WriteHeader(http.StatusOK)

		err = Tmpl.ExecuteTemplate(w, "login.html", loginSuccess)
		if err != nil {
			applog.Sugar.Error("error executing template: ", err)
		}
		return
	} else if err != nil {
		applog.Sugar.Error("err getting password hash: ", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	err = bcrypt.CompareHashAndPassword([]byte(password), []byte(form.Password))
	// password did not match
	if err != nil {
		w.WriteHeader(http.StatusOK)
		err = Tmpl.ExecuteTemplate(w, "login.html", loginSuccess)
		if err != nil {
			applog.Sugar.Error("error executing template: ", err)
		}
		return
	}

	session, err := database.PgStore.New(req, "session-name")
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		applog.Sugar.Error("error creating new unsaved session: ", err)
		return
	}
	session.Options = &sessions.Options{
		Path: "/",
		// in seconds
		MaxAge:   60 * 5 * 60,
		Secure:   false,
		HttpOnly: false,
		SameSite: 4,
	}

	// todo: combine this with search for password to get username and password
	row = database.PgDB.QueryRow(
		`SELECT username FROM Users WHERE email=$1`,
		form.Email,
	)
	var username string
	err = row.Scan(&username)
	if err != nil {
		applog.Sugar.Error("err scanning row: ", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	session.Values["username"] = username

	err = session.Save(req, w)
	if err != nil {
		applog.Sugar.Error("error saving session to db: ", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	http.Redirect(w, req, "/chat", http.StatusSeeOther)
}

func checkUserExists(ctx context.Context, email string, username string) (emailExists, usernameExists bool) {
	row := database.PgDB.QueryRow(
		`SELECT email FROM Users WHERE email=$1`,
		email,
	)
	var test string
	err := row.Scan(&test)
	emailExists = true
	if err != nil {
		emailExists = false
	}
	row = database.PgDB.QueryRow(
		`SELECT username FROM Users WHERE username=$1`,
		username,
	)

	err = row.Scan(&username)
	usernameExists = true
	if err != nil {
		usernameExists = false
	}

	return emailExists, usernameExists
}

func Logout(w http.ResponseWriter, req *http.Request) {
	session, err := database.PgStore.Get(req, "session-name")
	if err != nil {
		applog.Sugar.Error("err getting session name: ", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	session.Options.MaxAge = -1
	err = session.Save(req, w)
	if err != nil {
		applog.Sugar.Error("Error deleting session: ", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	http.Redirect(w, req, "/", http.StatusSeeOther)
}

func UserSession(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		applog.Sugar.Info("authenticating")
		session, err := database.PgStore.Get(req, "session-name")
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			applog.Sugar.Error("Could not get session: ", err)
			return
		}

		username := session.Values["username"].(string)
		applog.Sugar.Info(username)

		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			applog.Sugar.Error("error getting session: ", err)
			return
		}
		if session.IsNew {
			http.Redirect(w, req, "/login", http.StatusSeeOther)
			return
		}
		next.ServeHTTP(w, req)
	})
}

func LogRequest(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		// spew.Dump(req)
		spew.Dump(req.Header)
		next.ServeHTTP(w, req)
	})
}
