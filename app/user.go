package app

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/scylladb/gocqlx/v2"
	"github.com/sony/sonyflake"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
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

func getUserChatrooms(
	ctx context.Context,
	session gocqlx.Session,
	username string) ([]string, error) {

	stmt := "SELECT chatroom FROM users WHERE user = ?;"
	values := []string{"user"}
	query := session.Query(stmt, values)
	query.Bind(username)

	var chatrooms []string
	err := query.Select(&chatrooms)
	if err != nil {
		if err.Error() != "" {
			Sugar.Error("Error finding all chatrooms for user: ", err)
		}
		return chatrooms, err
	}

	return chatrooms, nil
}

func getUserCurrentRoom(ctx context.Context, session gocqlx.Session, username string) (string, error) {
	stmt := "SELECT current_chatroom FROM users WHERE user = ? LIMIT 1;"
	values := []string{"user"}
	query := session.Query(stmt, values)
	query.Bind(username)

	var currentRoom string
	err := query.Get(&currentRoom)
	if err != nil {
		return currentRoom, err
	}

	return currentRoom, nil

}

func (app App) GetUserInfo(writer http.ResponseWriter, req *http.Request) {
	ctx := req.Context()
	ctx, span := otel.Tracer("").Start(req.Context(), "GetUserInfo")
	defer span.End()

	Sugar.Info("Getting user chatrooms")

	session, err := app.PgStore.Get(req, "session-name")
	if err != nil {
		span.RecordError(err)
		Sugar.Error("error getting session name: ", err)
		span.SetStatus(codes.Error, "error getting session name")
		writer.WriteHeader(http.StatusInternalServerError)
		return
	}
	Sugar.Info(session)

	username := session.Values["username"].(string)
	Sugar.Info(username)

	var chatrooms []string
	chatrooms, err = getUserChatrooms(ctx, app.ScyllaDb, username)
	if err != nil {
		span.RecordError(err)
		if err.Error() == "not found" {
			Sugar.Infof("Chatrooms for user were not found: %v", err)
		} else {
			Sugar.Errorf("Error getting user chatrooms: %v", err)
			span.RecordError(err)
			span.SetStatus(codes.Error, "error getting user chatrooms")
			writer.WriteHeader(http.StatusInternalServerError)
			return
		}
	}

	var currentRoom string
	currentRoom, err = getUserCurrentRoom(ctx, app.ScyllaDb, username)
	if err != nil {
		span.RecordError(err)
		if err.Error() == "not found" {
			Sugar.Infof("User current chatroom not found: %v", err)
		} else {
			Sugar.Errorf("Error getting current user chatroom: %v", err)
			span.RecordError(err)
			span.SetStatus(codes.Error, "error getting current user chatroom")
			writer.WriteHeader(http.StatusInternalServerError)
			return
		}
	}

	type GetChatrooms struct {
		User        string   `json:"name"`
		Chatrooms   []string `json:"chatrooms"`
		CurrentRoom string   `json:"current_room"`
	}

	rowsJson, err := json.Marshal(GetChatrooms{User: username, Chatrooms: chatrooms, CurrentRoom: currentRoom})
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "error marhaslling row data")
		Sugar.Error("Error marshalling row data: ", err)
		writer.WriteHeader(http.StatusInternalServerError)
		return
	}

	span.SetStatus(codes.Ok, "")
	writer.WriteHeader(http.StatusOK)
	writer.Write(rowsJson)
}

func (app App) GetRoomMessages(w http.ResponseWriter, req *http.Request) {
	_, span := otel.Tracer("").Start(req.Context(), "GetRoomMessages")
	defer span.End()

	session, err := app.PgStore.Get(req, "session-name")
	if err != nil {
		Sugar.Error("error getting session name: ", err)
		span.RecordError(err)
		span.SetStatus(codes.Error, "error getting session name")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	err = req.ParseForm()
	if err != nil {
		Sugar.Error("err parsing form data: ", err)
		span.RecordError(err)
		span.SetStatus(codes.Error, "error parsing form")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	roomName := req.PostFormValue("chatroom_name")
	if roomName == "" {
		room_messages := []string{}
		rowsJson, err := json.Marshal(room_messages)
		if err != nil {
			Sugar.Error("Error marshalling row data: ", err)
		}
		w.Write(rowsJson)
		span.SetStatus(codes.Ok, "")
		w.WriteHeader(http.StatusOK)
		return
	}

	username := session.Values["username"].(string)
	Sugar.Info(roomName)
	Sugar.Info(username)
	stmt := "SELECT * FROM messages WHERE chatroom_name = ?;"
	values := []string{"chatroom_name"}
	query := app.ScyllaDb.Query(stmt, values)
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
		Sugar.Error("Error closing iterator for chatroom messages: ", err)
		span.RecordError(err)
		span.SetStatus(codes.Error, "Error closing interator for chatroom messages")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	rowsJson, err := json.Marshal(roomMessages)
	if err != nil {
		Sugar.Error("Error marshalling row data: ", err)
		span.RecordError(err)
		span.SetStatus(codes.Error, "Error marshalling messages into Json")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	span.SetStatus(codes.Ok, "")
	w.WriteHeader(http.StatusOK)
	w.Write(rowsJson)
}
