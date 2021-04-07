package chatroom

import (
	"chat/app"
	"chat/database"
	"encoding/json"
	"net/http"

	"github.com/davecgh/go-spew/spew"
	"github.com/gorilla/websocket"
)

type UserMessage struct {
	Message string
	// MessageType  string
	User         string
	ChatroomName string
}

type TestMessage struct {
	ChatroomName string
	Message      string
}

type User struct {
	Conn      *websocket.Conn
	Id        string
	Chatrooms []string
}

type ChatroomClient struct {
	Conn *websocket.Conn
	Id   string
}

type UserChatrooms struct {
	User            string
	CurrentChatroom string
	Chatroom        string
}

func GetUserInfo(writer http.ResponseWriter, req *http.Request) {

	session, err := database.PgStore.Get(req, "session-name")
	if err != nil {
		app.Sugar.Error("error getting session name: ", err)
		writer.WriteHeader(http.StatusInternalServerError)
		return
	}

	username := session.Values["username"].(string)
	app.Sugar.Info(username)
	stmt := "SELECT chatroom FROM users WHERE user = ?;"
	values := []string{"user"}
	query := ScyllaSession.Query(stmt, values)
	query.Bind(username)

	var chatrooms []string
	err = query.SelectRelease(&chatrooms)
	if err != nil {
		if err.Error() != "" {
			app.Sugar.Error("Error finding all chatrooms for user: ", err)
			writer.WriteHeader(http.StatusInternalServerError)
			return
		}
		// return err
	}

	stmt = "SELECT current_chatroom FROM users WHERE user = ? LIMIT 1;"
	values = []string{"user"}
	query = ScyllaSession.Query(stmt, values)
	query.Bind(username)

	currentRoom := ""
	err = query.GetRelease(&currentRoom)
	if err != nil {
		if err.Error() != "not found" {
			app.Sugar.Error("Error getting current chatroom for user: ", err)
			writer.WriteHeader(http.StatusInternalServerError)
			return
		}
		// return err
	}

	type GetChatrooms struct {
		User        string   `json:"name"`
		Chatrooms   []string `json:"chatrooms"`
		CurrentRoom string   `json:"current_room"`
	}

	rowsJson, err := json.Marshal(GetChatrooms{User: username, Chatrooms: chatrooms, CurrentRoom: currentRoom})
	if err != nil {
		app.Sugar.Error("Error marshalling row data: ", err)
	}

	writer.WriteHeader(http.StatusOK)
	writer.Write(rowsJson)
}

func GetRoomMessages(w http.ResponseWriter, req *http.Request) {
	session, err := database.PgStore.Get(req, "session-name")
	if err != nil {
		app.Sugar.Error("error getting session name: ", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	err = req.ParseForm()
	if err != nil {
		app.Sugar.Error("err parsing form data: ", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	room_name := req.PostFormValue("chatroom_name")

	username := session.Values["username"].(string)
	app.Sugar.Info(room_name)
	app.Sugar.Info(username)
	stmt := "SELECT content FROM messages WHERE chatroom_name = ?;"
	values := []string{"chatroom_name"}
	query := ScyllaSession.Query(stmt, values)
	query.Bind(room_name)

	room_messages := []string{}
	// err = query.GetRelease(&room_messages)
	err = query.SelectRelease(&room_messages)
	if err != nil {
		if err.Error() != "not found" {
			app.Sugar.Error("Error getting current chatroom for user: ", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		// return err
	}
	spew.Dump(room_messages)
	rowsJson, err := json.Marshal(room_messages)
	if err != nil {
		app.Sugar.Error("Error marshalling row data: ", err)
	}

	w.WriteHeader(http.StatusOK)
	w.Write(rowsJson)
}
