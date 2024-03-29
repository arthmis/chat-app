package app

import (
	"net/http"
	"net/url"
	"strings"
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

		form = url.Values{}
		form.Set("email", "test@gmail.com")
		form.Set("password", "secretpassy")

		_, err = client.PostForm(server.URL+"/api/user/login", form)
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		return nil

	}()

	if err != nil {
		t.Errorf("Received an error when signing up, then logging in a user: %v", err)
	}

	res, err := client.Post(server.URL+"/api/user/logout", "application/x-www-form-urlencoded", strings.NewReader(""))
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	// similar to login, this will redirect and since there is no static file server
	// the client will request the main page but nothing will happen, and the server will
	// respond with 404
	if res.StatusCode != http.StatusNotFound {
		t.Errorf("User art was not created. Received status code %v, wanted %v", res.StatusCode, http.StatusNotFound)
	}

	// checking if session was deleted
	// don't know if this is right way to check if a session was deleted
	session, err := application.PgStore.Get(res.Request, "session-name")
	if err != nil {
		Sugar.Error("err getting session name: ", err)
	}
	if session.ID != "" {
		t.Error("Expected there to be no client.")
	}

}

func TestLogoutWithoutLogin(t *testing.T) {
	server, client, err := serverSetup()
	t.Cleanup(func() {
		server.Close()
		databaseReset()
	})

	res, err := client.Post(server.URL+"/api/user/logout", "application/x-www-form-urlencoded", strings.NewReader(""))
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	// similar to login, this will redirect and since there is no static file server
	// the client will request the main page but nothing will happen, and the server will
	// respond with 404
	if res.StatusCode != http.StatusInternalServerError {
		t.Errorf("User art was somehow logged out although it should be impossible. Received status code %v, wanted %v", res.StatusCode, http.StatusInternalServerError)
	}

	// checking if session was deleted or not there at all
	// don't know if this is right way to check if a session was deleted or not there
	session, err := application.PgStore.Get(res.Request, "session-name")
	if err != nil {
		Sugar.Error("err getting session name: ", err)
	}
	if session.ID != "" {
		t.Error("Expected there to be no client.")
	}

}
