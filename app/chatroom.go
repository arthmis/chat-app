package app

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/scylladb/gocqlx/v2"
	"github.com/scylladb/gocqlx/v2/table"
	"github.com/sony/sonyflake"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	"nhooyr.io/websocket"
)

const webUrl = "http://localhost:8000"

var messageMetaData = table.Metadata{
	Name:    "messages",
	Columns: []string{"chatroom_name", "user_id", "content", "message_id"},
	PartKey: []string{"chatroom_name", "message_id"},
	SortKey: []string{"message_id"},
}
var userMetaData = table.Metadata{
	Name:    "users",
	Columns: []string{"user", "current_chatroom", "chatroom"},
	PartKey: []string{"user", "chatroom"},
	SortKey: []string{"chatroom"},
}

var chatroomTable = table.New(messageMetaData)
var userTable = table.New(userMetaData)

type Message struct {
	ChatroomName string `db:"chatroom_name"`
	UserId       string `db:"user_id"`
	Content      string `db:"content"`
	MessageId    uint64 `db:"message_id"`
}

type OutgoingMessage struct {
	ChatroomName string
	UserId       string
	Content      string
	Timestamp    string
}

type ChatroomUser struct {
	Name string
	Conn *websocket.Conn
}
type Chatroom struct {
	Id      string
	Clients []*ChatroomClient
	// get rid of messages field, not necessary
	Messages []IncomingMessage
	// Channel       chan UserMessage
	Channel       chan MessageWithCtx
	ScyllaSession *gocqlx.Session
	Snowflake     *sonyflake.Sonyflake
}

func (room *Chatroom) addUser(conn *websocket.Conn, user string) {
	client := ChatroomClient{Conn: conn, Id: user}
	room.Clients = append(room.Clients, &client)
}
func (room *Chatroom) Run() {
	// ctx := context.Background()
	for {
		newMessage := <-room.Channel
		ctx := newMessage.Ctx
		message := newMessage.Message
		_, span := otel.Tracer("").Start(ctx, "Saving message")
		// room.Messages = append(room.Messages, newMessage)
		// err := room.saveMessage(newMessage)
		savedMsg, err := room.saveMessage(message)
		// if there is an error saving the message there should be
		// a way to communicate back to the user that sent the message
		// that it was not sent successfully and should try again
		if err != nil {
			Sugar.Error("error saving message: ", err)
		}
		// bytes, err := json.Marshal(newMessage)

		snowFlakeParts := sonyflake.Decompose(savedMsg.MessageId)
		msgTime := snowFlakeParts["time"]

		outMessage := OutgoingMessage{
			ChatroomName: savedMsg.ChatroomName,
			UserId:       savedMsg.UserId,
			Content:      savedMsg.Content,
			Timestamp:    time.Unix(int64(msgTime/100), 0).Format(time.RFC3339),
		}

		savedMsg.MessageId = msgTime

		bytes, err := json.Marshal(outMessage)
		if err != nil {
			span.RecordError(err)
			Sugar.Error(err)
		}
		span.End()
		ctx, span = otel.Tracer("").Start(ctx, "Writing message to users")
		for i := range room.Clients {
			// err = room.Clients[i].Conn.WriteMessage(websocket.TextMessage, bytes)
			err = room.Clients[i].Conn.Write(ctx, websocket.MessageText, bytes)
			if err != nil {
				Sugar.Error("error writing message to user ws connection: ", err)
			}
		}
		span.End()
	}
}

func (room *Chatroom) saveMessage(chatMessage IncomingMessage) (Message, error) {
	messageId, err := room.Snowflake.NextID()
	if err != nil {
		Sugar.Error("Error generating sonyflake id: ", err)
		return Message{}, err
	}
	Sugar.Info("user: ", chatMessage.User)

	message := Message{
		ChatroomName: chatMessage.ChatroomName,
		UserId:       chatMessage.User,
		Content:      chatMessage.Message,
		MessageId:    messageId,
	}
	query := room.ScyllaSession.Query(chatroomTable.Insert()).BindStruct(message)
	err = query.ExecRelease()
	if err != nil {
		Sugar.Error("Error inserting message in database: ", err)
		return Message{}, err
	}

	return message, err
}

func NewChatroom() *Chatroom {
	room := new(Chatroom)
	room.Id = ""
	room.Clients = make([]*ChatroomClient, 0)
	room.Messages = make([]IncomingMessage, 20)
	room.Channel = make(chan MessageWithCtx)
	return room
}

// TODO think about tracking users and the rooms they are a part of
func (app App) Create(writer http.ResponseWriter, req *http.Request) {
	_, span := otel.Tracer("").Start(req.Context(), "CreateRoom")
	defer span.End()

	err := req.ParseForm()
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "error parsing form")
		Sugar.Error("error parsing form: ", err)
		writer.WriteHeader(http.StatusInternalServerError)
		return
	}

	roomName := req.FormValue("chatroom_name")
	err = Validate.Var(roomName, "lt=30,gt=3,ascii")
	if err != nil {
		Sugar.Error("chatroom name was not valid: ", err)
		span.RecordError(err)
		span.SetStatus(codes.Ok, "chatroom name was not valid")
		writer.WriteHeader(http.StatusBadRequest)
		return
	}

	session, err := app.PgStore.Get(req, "session-name")
	if err != nil {
		Sugar.Error("error getting session name: ", err)
		span.RecordError(err)
		span.SetStatus(codes.Error, "error getting session name")
		writer.WriteHeader(http.StatusInternalServerError)
		return
	}

	if session.ID == "" {
		Sugar.Error("Session was empty. Session was not found")
		span.RecordError(err)
		span.SetStatus(codes.Error, "session was empty")
		writer.WriteHeader(http.StatusInternalServerError)
		return
	}

	username := session.Values["username"].(string)

	room := NewChatroom()
	room.Id = roomName
	room.Snowflake = app.Snowflake
	room.ScyllaSession = &app.ScyllaDb

	newRoomForUser := struct {
		User            string
		CurrentChatroom string
		Chatroom        string
	}{
		User:            username,
		CurrentChatroom: roomName,
		Chatroom:        roomName,
	}

	// TODO: assume these ccan fail
	query := room.ScyllaSession.Query(userTable.Insert()).BindStruct(newRoomForUser)
	err = query.ExecRelease()
	if err != nil {
		Sugar.Error("Error inserting new chatroom for user in user table: ", err)
		span.RecordError(err)
		span.SetStatus(codes.Error, "error inserting new chatroom for user in user table")
		writer.WriteHeader(http.StatusInternalServerError)
		return
	}

	// TODO: assume these ccan fail and I will have to roll back the above scylla insert and
	// PG insert
	// query = room.ScyllaSession.Query(chatroomTable.Insert()).BindStruct(newRoomForUser)
	// err = query.ExecRelease()
	// if err != nil {
	// 	Sugar.Error("Error inserting new chatroom into messages table: ", err)
	// 	writer.WriteHeader(http.StatusInternalServerError)
	// 	return
	// }

	// TODO: assume these ccan fail and I will have to roll back the above scylla insert
	_, err = app.Pg.Exec(
		`INSERT INTO Rooms (name) VALUES ($1)`,
		roomName,
	)
	if err != nil {
		Sugar.Error("error inserting new chatroom into Rooms table: ", err)
		span.RecordError(err)
		span.SetStatus(codes.Error, "error inserting new chatroom into Rooms table")
		writer.WriteHeader(http.StatusInternalServerError)
		return
	}

	app.Clients[username].Chatrooms = append(app.Clients[username].Chatrooms, room.Id)
	user := session.Values["username"].(string)
	client := ChatroomClient{Id: app.Clients[user].Id, Conn: app.Clients[user].Conn}
	room.Clients = append(room.Clients, &client)
	app.ChatroomChannels[room.Id] = room.Channel
	app.Chatrooms[room.Id] = room

	go room.Run()

	chatroomNameEncoded, err := json.Marshal(room.Id)
	if err != nil {
		Sugar.Error(err)
		span.RecordError(err)
		span.SetStatus(codes.Error, "error mashalling room.Id")
	}

	writer.WriteHeader(http.StatusCreated)
	span.SetStatus(codes.Ok, "room was created")
	_, err = writer.Write(chatroomNameEncoded)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "Error writing encoded chatroom name in response")
		Sugar.Error("error writing chatroom name in response: ", err)
		writer.WriteHeader(http.StatusInternalServerError)
		return
	}
}

// TODO check to see if the user is actually a part of the chatroom
// before they are allowed to create an invite
func (app App) CreateInvite(w http.ResponseWriter, req *http.Request) {
	_, span := otel.Tracer("").Start(req.Context(), "CreateInvite")
	defer span.End()

	err := req.ParseForm()
	if err != nil {
		Sugar.Error("error parsing form for create invite: ", err)
		span.RecordError(err)
		span.SetStatus(codes.Error, "error parsing form for create invite")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	roomName := req.FormValue("chatroom_name")

	var inviteTimeLimit InviteTimeLimit
	switch timeLimit := req.FormValue("invite_timelimit"); timeLimit {
	case "1 day":
		inviteTimeLimit = InviteTimeLimit{
			limit: 1,
		}
	case "1 week":
		inviteTimeLimit = InviteTimeLimit{
			limit: 7,
		}
	case "Forever":
		inviteTimeLimit = InviteTimeLimit{
			// this will represent infinity
			limit: 0,
		}
	default:
		span.AddEvent("InviteBadRequest")
		span.SetStatus(codes.Ok, fmt.Sprintf("bad invite: %v", timeLimit))
		w.WriteHeader(http.StatusBadRequest)
		_, err := w.Write([]byte("Expiry value is not one of the possible choices"))
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, "error writing response")
			Sugar.Error(req.FormValue("invite_timelimit"), err)
			w.WriteHeader(http.StatusInternalServerError)
		}
		return
	}

	inviteCode, err := app.Invitations.createInvite(roomName, inviteTimeLimit)
	if err != nil {
		Sugar.Error(err)
		span.RecordError(err)
		span.SetStatus(codes.Error, "invite code not successfully created")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	encodedInviteCode, err := json.Marshal(webUrl + "/room/join/" + inviteCode)
	if err != nil {
		Sugar.Error(err)
		span.RecordError(err)
		span.SetStatus(codes.Error, "marshalling failed")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusCreated)
	span.SetStatus(codes.Ok, "invite code created")

	_, err = w.Write(encodedInviteCode)
	if err != nil {
		Sugar.Error("Error writing invite code in response: ", err)
		span.RecordError(err)
		span.SetStatus(codes.Error, "marshalling failed")
	}

}

func (app App) Join(writer http.ResponseWriter, req *http.Request) {
	_, span := otel.Tracer("").Start(req.Context(), "JoinRoom")
	defer span.End()

	inviteCode := strings.Split(req.URL.String(), "/api/room/join/")[1]
	chatroomName, err := app.Invitations.getChatroom(inviteCode)
	// TODO: I will need to handle multiple errors here
	// inviteNoteFound
	// DatabaseError
	if err != nil {
		Sugar.Error(err)
		span.RecordError(err)
		span.SetStatus(codes.Error, "")
		writer.WriteHeader(http.StatusInternalServerError)
		return
	}
	Sugar.Info(inviteCode, chatroomName)

	session, err := app.PgStore.Get(req, "session-name")
	if err != nil {
		Sugar.Error("err getting session name: ", err)
		span.RecordError(err)
		span.SetStatus(codes.Error, "error getting session name")
		writer.WriteHeader(http.StatusInternalServerError)
		return
	}

	user := session.Values["username"].(string)
	client := ChatroomClient{Id: app.Clients[user].Id, Conn: app.Clients[user].Conn}

	app.Chatrooms[chatroomName].Clients = append(app.Chatrooms[chatroomName].Clients, &client)
	// todo add chatroom to user also

	// writer.WriteHeader(http.StatusInternalServerError)
	name, err := json.Marshal(chatroomName)
	if err != nil {
		Sugar.Error(err)
		span.RecordError(err)
		span.SetStatus(codes.Error, "error masharlling chatroom name")
		writer.WriteHeader(http.StatusInternalServerError)
		return
	}

	writer.WriteHeader(http.StatusAccepted)
	span.SetStatus(codes.Ok, "successfully joined chatroom")
	_, err = writer.Write(name)
	if err != nil {
		Sugar.Error("error writing chatroom name in response: ", err)
		span.RecordError(err)
		writer.WriteHeader(http.StatusInternalServerError)
		span.SetStatus(codes.Error, "error writing chatroom name in response")
	}
}

// TODO add channel that would allow breaking out of this function
// might be necessary
func RemoveExpiredInvites(db *sql.DB, interval time.Duration) {
	ticker := time.NewTicker(interval)

	defer func() {
		ticker.Stop()
	}()

	for {
		select {

		case <-ticker.C:
			_, err := db.Exec("DELETE FROM Invites WHERE expires < now()")
			if err != nil {
				log.Printf("Unable to delete invites: %v", err)
			}
		}
	}
}
