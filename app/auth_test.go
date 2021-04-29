package app

import (
	"net/http"
	"net/url"
	"testing"
)

func TestSignup(t *testing.T) {
	server, client, err := serverSetup()
	t.Cleanup(func() {
		server.Close()
		databaseReset()
	})

	form := url.Values{}
	form.Set("email", "test@gmail.com")
	form.Set("username", "art")
	form.Set("password", "secretpassy")
	form.Set("confirmPassword", "secretpassy")

	res, err := client.PostForm(server.URL+"/api/user/signup", form)
	if err != nil {
		t.Fatalf("err: %v", err)
	}

	if res.StatusCode != http.StatusCreated {
		t.Errorf("User art was not created. Received status code %v, wanted %v", res.StatusCode, http.StatusCreated)
	}
}

func TestLoginWithSignup(t *testing.T) {
	server, client, err := serverSetup()
	t.Cleanup(func() {
		server.Close()
		databaseReset()
	})

	err = func() error { // Signs up a user
		form := url.Values{}
		form.Set("email", "test@gmail.com")
		form.Set("username", "art")
		form.Set("password", "secretpassy")
		form.Set("confirmPassword", "secretpassy")

		_, err := client.PostForm(server.URL+"/api/user/signup", form)
		if err != nil {
			t.Fatalf("err: %v", err)
			return err
		}
		return nil

	}()
	if err != nil {
		t.Errorf("Received an error when signing up a user: %v", err)
	}

	form := url.Values{}
	form.Set("email", "test@gmail.com")
	form.Set("password", "secretpassy")

	res, err := client.PostForm(server.URL+"/api/user/login", form)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	// TODO: ok for now this will return a 404 when successful because it redirects
	// then request the chat page but that isn't served by the server, if it were using nginx
	// then it wouldn't be an issue, or if it it was serving static files
	if res.StatusCode != http.StatusNotFound {
		t.Errorf("User art was not logged in. Received status code %v, wanted %v", res.StatusCode, http.StatusNotFound)
	}

	// checking if session was created for art
	session, err := application.PgStore.Get(res.Request, "session-name")
	if err != nil {
		Sugar.Error("err getting session name: ", err)
	}

	clientName := session.Values["username"].(string)
	if clientName != "art" {
		t.Error("Expected client name to be art.")
	}
}

func TestLoginWithoutSignup(t *testing.T) {
	server, client, err := serverSetup()
	t.Cleanup(func() {
		server.Close()
		databaseReset()
	})

	form := url.Values{}
	form.Set("email", "test@gmail.com")
	form.Set("password", "secretpassy")

	res, err := client.PostForm(server.URL+"/api/user/login", form)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if res.StatusCode != http.StatusOK {
		t.Errorf("User art was not created. Received status code %v, wanted %v", res.StatusCode, http.StatusOK)
	}
}

func TestLogout(t *testing.T) {
	// func(t *testing.T) {
	// 	t.Cleanup(func() {
	// 		_, err := database.PgDB.Exec(
	// 			`DELETE FROM users;`,
	// 		)
	// 		if err != nil {
	// 			applog.Sugar.Error("error deleting all users: ", err)
	// 		}
	// 	})
	// }(t)
	// // server := httptest.NewServer(newRouter())
	// server := httptest.NewUnstartedServer(newRouter())
	// server.Config = &http.Server{
	// 	Addr: "http://localhost:8000",
	// }
	// server.Start()
	// defer server.Close()

	// client := server.Client()

	// form := url.Values{}
	// form.Set("email", "test@gmail.com")
	// form.Set("username", "art")
	// form.Set("password", "secretpassy")
	// form.Set("confirmPassword", "secretpassy")
	// res, err := client.PostForm("/signup", form)
	// fmt.Println(err)
	// fmt.Println(res)

	// form = url.Values{}
	// form.Set("email", "test@gmail.com")
	// form.Set("password", "secretpassy")
	// res, _ = client.PostForm("/login", form)
	// applog.Sugar.Error(res)

	// // req, _ := http.NewRequest(http.MethodPost, "/logout", strings.NewReader(""))
	// // res, _ := client.Do(req)
	// // res, _ := client.Post("/logout", "text/plain", strings.NewReader(""))
	// form = url.Values{}
	// res, _ = client.PostForm("/logout", form)
	// // status := res.
	// spew.Dump(res)

}
