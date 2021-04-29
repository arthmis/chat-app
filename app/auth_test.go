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

func TestLogin(t *testing.T) {
	// func(t *testing.T) {
	// 	addUser()
	// 	t.Cleanup(func() {
	// 		_, err := database.PgDB.Exec(
	// 			`DELETE FROM users;`,
	// 		)
	// 		if err != nil {
	// 			applog.Sugar.Error("error deleting all users: ", err)
	// 		}
	// 	})
	// }(t)

	// form := url.Values{}
	// form.Set("email", "test@gmail.com")
	// form.Set("password", "secretpassy")

	// encodedForm := strings.NewReader(form.Encode())
	// req, err := http.NewRequest(http.MethodPost, "/login", encodedForm)
	// if err != nil {
	// 	t.Fatal("error creating new request: ", err)
	// }
	// req.Header.Add("Content-Type", "application/x-www-form-urlencoded")

	// res := httptest.NewRecorder()
	// router := newRouter()
	// router.ServeHTTP(res, req)

	// status := res.Code
	// if status != http.StatusSeeOther {
	// 	t.Errorf("login endpoint returned wrong status code: got %v want %v\n", status, http.StatusSeeOther)
	// }
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
