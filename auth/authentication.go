package auth

import (
	"context"
	"database/sql"
	"fmt"
	"html/template"
	"log"
	"net/http"

	"github.com/antonlindstrom/pgstore"
	"github.com/go-playground/validator"
	"github.com/gorilla/schema"
	"github.com/gorilla/sessions"
	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/stdlib"
	"golang.org/x/crypto/bcrypt"
)

var Decoder = schema.NewDecoder()
var Validate = validator.New()
var Tmpl *template.Template
var Store *pgstore.PGStore
var Db *sql.DB

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
	req.ParseMultipartForm(50000)
	err := Decoder.Decode(&form, req.PostForm)
	if err != nil {
		log.Println(err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	fmt.Printf("%+v\n", form)

	err = Validate.Struct(form)
	if err != nil {
		log.Println(err)
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(form.Password), bcrypt.MinCost)
	if err != nil {
		log.Println(err)
	}
	fmt.Println(string(hash))

	emailExists, usernameExists := checkUserExists(req.Context(), Db, form.Email, form.Username)

	userExists := struct {
		username       string
		email          string
		usernameExists string
		emailExists    string
	}{
		form.Username,
		form.Email,
		"Username already exists",
		"Email already exists",
	}

	fmt.Println(emailExists, usernameExists)
	// TODO COMPLETE THIS AND ACTUALLY IMPLEMENT THE TEMPLATES
	if usernameExists && emailExists {
		w.WriteHeader(http.StatusOK)
		Tmpl.ExecuteTemplate(w, "signup.html", userExists)

		return
	} else if usernameExists {
		userExists.email = ""
		w.WriteHeader(http.StatusOK)
		Tmpl.ExecuteTemplate(w, "signup.html", userExists)

		return
	} else if emailExists {
		userExists.username = ""
		w.WriteHeader(http.StatusOK)
		Tmpl.ExecuteTemplate(w, "signup.html", userExists)

		return
	}

	conn, err := stdlib.AcquireConn(Db)
	if err == nil {
		conn.Exec(
			req.Context(),
			`INSERT INTO Users (email, username, password) VALUES ($1, $2, $3)`,
			form.Email,
			form.Username,
			string(hash),
		)
		stdlib.ReleaseConn(Db, conn)
	} else {
		log.Println("error inserting values: \n", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusAccepted)
	Tmpl.ExecuteTemplate(w, "login.html", nil)
}

func Login(w http.ResponseWriter, req *http.Request) {
	var form UserLogin
	err := req.ParseMultipartForm(50000)
	if err != nil {
		log.Println(err)
	}
	err = Decoder.Decode(&form, req.PostForm)
	if err != nil {
		log.Println(err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	row := Db.QueryRow(
		`SELECT password FROM Users WHERE email=$1`,
		form.Email,
	)
	var password string
	err = row.Scan(&password)
	if err != nil {
		// email is not available
		w.WriteHeader(http.StatusOK)
		Tmpl.ExecuteTemplate(w, "login.html", nil)
		return
	}

	err = bcrypt.CompareHashAndPassword([]byte(password), []byte(form.Password))
	// password did not match
	if err != nil {
		w.WriteHeader(http.StatusOK)
		Tmpl.ExecuteTemplate(w, "login.html", nil)
		return
	}

	session, err := Store.New(req, "session-name")
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		log.Println(err)
		return
	}
	session.Options = &sessions.Options{
		Path:     "/",
		MaxAge:   60 * 60 * 24 * 7,
		HttpOnly: true,
	}

	row = Db.QueryRow(
		`SELECT username FROM Users WHERE email=$1`,
		form.Email,
	)
	var username string
	err = row.Scan(&username)
	session.Values["username"] = username

	err = session.Save(req, w)
	if err != nil {
		log.Println(err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	http.Redirect(w, req, "/chat", http.StatusSeeOther)
}

func checkUserExists(ctx context.Context, dbpool *sql.DB, email string, username string) (emailExists, usernameExists bool) {
	row := dbpool.QueryRow(
		`SELECT email FROM Users WHERE email=$1`,
		email,
	)

	err := row.Scan(&email)
	emailExists = true
	if err == pgx.ErrNoRows {
		emailExists = false
	}
	row = dbpool.QueryRow(
		`SELECT username FROM Users WHERE username=$1`,
		username,
	)

	err = row.Scan(&username)
	usernameExists = true
	if err == pgx.ErrNoRows {
		usernameExists = false
	}

	return emailExists, usernameExists
}

func Logout(w http.ResponseWriter, req *http.Request) {
	session, err := Store.Get(req, "session-name")

	session.Options.MaxAge = -1
	if err = session.Save(req, w); err != nil {
		log.Printf("Error deleting session: %v", err)
	}

	http.Redirect(w, req, "/", http.StatusSeeOther)
}

func UserSession(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		session, err := Store.Get(req, "session-name")
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			log.Println(err)
			return
		}
		if session.IsNew {
			http.Redirect(w, req, "/login", http.StatusSeeOther)
			return
		}
		next.ServeHTTP(w, req)
	})
}
