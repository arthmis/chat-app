package chatroom

import (
	"chat/applog"
	"chat/database"
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/sony/sonyflake"
	"nhooyr.io/websocket"
)

type IncomingMessage struct {
	Message string
	// MessageType  string
	User         string
	ChatroomName string
}

type MessageWithCtx struct {
	Message IncomingMessage
	Ctx     context.Context
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

func getUserChatrooms(ctx context.Context, username string) ([]string, error) {
	stmt := "SELECT chatroom FROM users WHERE user = ?;"
	values := []string{"user"}
	query := ScyllaSession.Query(stmt, values)
	query.Bind(username)

	var chatrooms []string
	err := query.Select(&chatrooms)
	if err != nil {
		if err.Error() != "" {
			applog.Sugar.Error("Error finding all chatrooms for user: ", err)
		}
		return chatrooms, err
	}

	return chatrooms, nil
}

func getUserCurrentRoom(ctx context.Context, username string) (string, error) {
	stmt := "SELECT current_chatroom FROM users WHERE user = ? LIMIT 1;"
	values := []string{"user"}
	query := ScyllaSession.Query(stmt, values)
	query.Bind(username)

	var currentRoom string
	err := query.Get(&currentRoom)
	if err != nil {
		if err.Error() != "not found" {
			applog.Sugar.Error("Error getting current chatroom for user: ", err)
		}
		return currentRoom, err
	}

	return currentRoom, nil

}

func GetUserInfo(writer http.ResponseWriter, req *http.Request) {
	ctx := req.Context()
	applog.Sugar.Info("Getting user chatrooms")

	session, err := database.PgStore.Get(req, "session-name")
	if err != nil {
		applog.Sugar.Error("error getting session name: ", err)
		writer.WriteHeader(http.StatusInternalServerError)
		return
	}
	applog.Sugar.Info(session)

	username := session.Values["username"].(string)
	applog.Sugar.Info(username)

	var chatrooms []string
	chatrooms, err = getUserChatrooms(ctx, username)
	// TODO: maybe think about checking if user doesn't exist
	// and return a more appropriate error?
	if err != nil {
		writer.WriteHeader(http.StatusInternalServerError)
		return
	}

	var currentRoom string
	currentRoom, err = getUserCurrentRoom(ctx, username)
	// TODO: maybe think about checking if user doesn't exist
	// and return a more appropriate error?
	if err != nil {
		writer.WriteHeader(http.StatusInternalServerError)
		return
	}

	type GetChatrooms struct {
		User        string   `json:"name"`
		Chatrooms   []string `json:"chatrooms"`
		CurrentRoom string   `json:"current_room"`
	}

	rowsJson, err := json.Marshal(GetChatrooms{User: username, Chatrooms: chatrooms, CurrentRoom: currentRoom})
	if err != nil {
		applog.Sugar.Error("Error marshalling row data: ", err)
	}

	writer.WriteHeader(http.StatusOK)
	writer.Write(rowsJson)
}

func GetRoomMessages(w http.ResponseWriter, req *http.Request) {
	session, err := database.PgStore.Get(req, "session-name")
	if err != nil {
		applog.Sugar.Error("error getting session name: ", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	err = req.ParseForm()
	if err != nil {
		applog.Sugar.Error("err parsing form data: ", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	roomName := req.PostFormValue("chatroom_name")
	if roomName == "" {
		room_messages := []string{}
		rowsJson, err := json.Marshal(room_messages)
		if err != nil {
			applog.Sugar.Error("Error marshalling row data: ", err)
		}
		w.Write(rowsJson)
		w.WriteHeader(http.StatusOK)
		return
	}

	username := session.Values["username"].(string)
	applog.Sugar.Info(roomName)
	applog.Sugar.Info(username)
	stmt := "SELECT * FROM messages WHERE chatroom_name = ?;"
	values := []string{"chatroom_name"}
	query := ScyllaSession.Query(stmt, values)
	iter := query.Bind(roomName).Iter()

	roomMessages := []OutgoingMessage{}
	var message Message
	var outMessage OutgoingMessage
	for iter.StructScan(&message) {
		msgTime := sonyflake.Decompose(message.MessageId)["time"]
		outMessage.ChatroomName = message.ChatroomName
		outMessage.UserId = message.UserId
		outMessage.Content = message.Content
		// sonyflake time is in units of 10 milliseconds
		// divide by 100 to get the correct amount of seconds
		outMessage.Timestamp = time.Unix(int64(msgTime/100), 0).Format(time.RFC3339)

		roomMessages = append(roomMessages, outMessage)
	}
	if err := iter.Close(); err != nil {
		applog.Sugar.Error("Error closing iterato for chatroom messages: ", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	rowsJson, err := json.Marshal(roomMessages)
	if err != nil {
		applog.Sugar.Error("Error marshalling row data: ", err)
	}

	w.WriteHeader(http.StatusOK)
	w.Write(rowsJson)
}
