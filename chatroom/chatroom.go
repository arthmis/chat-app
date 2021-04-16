package chatroom

import (
	"chat/applog"
	"chat/database"
	"chat/validate"
	"database/sql"
	"encoding/json"
	"log"
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/scylladb/gocqlx/v2"
	"github.com/scylladb/gocqlx/v2/table"
	"github.com/sony/sonyflake"
	"go.opentelemetry.io/otel"
	"nhooyr.io/websocket"
)

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
	ChatroomName string
	UserId       string
	Content      string
	MessageId    uint64 `db:"message_id"`
}

type ChatroomUser struct {
	Name string
	Conn *websocket.Conn
}
type Chatroom struct {
	Id      string
	Clients []*ChatroomClient
	// get rid of messages field, not necessary
	Messages []UserMessage
	// Channel       chan UserMessage
	Channel       chan MessageWithCtx
	ScyllaSession *gocqlx.Session
	Snowflake     *sonyflake.Sonyflake
}

var Chatrooms = make(map[string]*Chatroom)

// var ChatroomChannels = make(map[string]chan UserMessage)
var ChatroomChannels = make(map[string]chan MessageWithCtx)
var Snowflake *sonyflake.Sonyflake
var ScyllaSession gocqlx.Session

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
		err := room.saveMessage(message)
		// if there is an error saving the message there should be
		// a way to communicate back to the user that sent the message
		// that it was not sent successfully and should try again
		if err != nil {
			applog.Sugar.Error("error saving message: ", err)
		}
		// bytes, err := json.Marshal(newMessage)
		bytes, err := json.Marshal(message)
		if err != nil {
			span.RecordError(err)
			applog.Sugar.Error(err)
		}
		span.End()
		ctx, span = otel.Tracer("").Start(ctx, "Writing message to users")
		for i := range room.Clients {
			// err = room.Clients[i].Conn.WriteMessage(websocket.TextMessage, bytes)
			err = room.Clients[i].Conn.Write(ctx, websocket.MessageText, bytes)
			if err != nil {
				applog.Sugar.Error("error writing message to user ws connection: ", err)
			}
		}
		span.End()
	}
}

func (room *Chatroom) saveMessage(chatMessage UserMessage) error {
	messageId, err := room.Snowflake.NextID()
	if err != nil {
		applog.Sugar.Error("Error generating sonyflake id: ", err)
		return err
	}
	applog.Sugar.Info("user: ", chatMessage.User)

	message := Message{
		ChatroomName: chatMessage.ChatroomName,
		UserId:       chatMessage.User,
		Content:      chatMessage.Message,
		MessageId:    messageId,
	}
	query := room.ScyllaSession.Query(chatroomTable.Insert()).BindStruct(message)
	err = query.ExecRelease()
	if err != nil {
		applog.Sugar.Error("Error inserting message in database: ", err)
		return err
	}

	return err
}

func NewChatroom() *Chatroom {
	room := new(Chatroom)
	room.Id = ""
	room.Clients = make([]*ChatroomClient, 0)
	room.Messages = make([]UserMessage, 20)
	// room.Channel = make(chan UserMessage)
	room.Channel = make(chan MessageWithCtx)
	return room
}

// TODO think about tracking users and the rooms they are a part of
func Create(writer http.ResponseWriter, req *http.Request) {
	err := req.ParseMultipartForm(1000)
	if err != nil {
		applog.Sugar.Error("error parsing form: ", err)
		writer.WriteHeader(http.StatusInternalServerError)
		return
	}

	session, err := database.PgStore.Get(req, "session-name")
	if err != nil {
		applog.Sugar.Error("error getting session name: ", err)
		writer.WriteHeader(http.StatusInternalServerError)
		return
	}
	roomName := req.FormValue("chatroom_name")
	err = validate.Validate.Var(roomName, "lt=30,gt=3,ascii")
	if err != nil {
		applog.Sugar.Error("chatroom name was not valid: ", err)
		writer.WriteHeader(http.StatusBadRequest)
		return
	}

	username := session.Values["username"].(string)

	room := NewChatroom()
	room.Id = roomName
	room.Snowflake = Snowflake
	room.ScyllaSession = &ScyllaSession

	newRoomForUser := struct {
		User            string
		CurrentChatroom string
		Chatroom        string
	}{
		User:            username,
		CurrentChatroom: roomName,
		Chatroom:        roomName,
	}
	query := room.ScyllaSession.Query(userTable.Insert()).BindStruct(newRoomForUser)
	err = query.ExecRelease()
	if err != nil {
		applog.Sugar.Error("Error inserting new chatroom for user in user table: ", err)
		return
		// return err
	}

	Clients[username].Chatrooms = append(Clients[username].Chatrooms, room.Id)
	user := session.Values["username"].(string)
	client := ChatroomClient{Id: Clients[user].Id, Conn: Clients[user].Conn}
	room.Clients = append(room.Clients, &client)
	ChatroomChannels[room.Id] = room.Channel
	Chatrooms[room.Id] = room

	go room.Run()

	chatroomNameEncoded, err := json.Marshal(room.Id)
	if err != nil {
		applog.Sugar.Error(err)
	}

	writer.WriteHeader(http.StatusAccepted)
	_, err = writer.Write(chatroomNameEncoded)
	if err != nil {
		applog.Sugar.Error("error writing chatroom name in response: ", err)
		return
	}
}

// TODO check to see if the user is actually a part of the chatroom
// before they are allowed to create an invite
func CreateInvite(w http.ResponseWriter, req *http.Request) {
	err := req.ParseForm()
	if err != nil {
		applog.Sugar.Error("error parsing form for create invite: ", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	roomName := req.FormValue("chatroom_name")
	now := time.Now()
	// timeLimit := req.FormValue("invite_timelimit")
	inviteTimeLimit := time.Time{}
	forever := 0.0
	switch timeLimit := req.FormValue("invite_timelimit"); timeLimit {
	case "1 day":
		inviteTimeLimit = now.Add(time.Hour * 24)
	case "1 week":
		inviteTimeLimit = now.Add(time.Hour * 24 * 7)
	case "Forever":
		forever = math.Inf(1)
	default:
		w.WriteHeader(http.StatusBadRequest)
		_, err := w.Write([]byte("Expiry value is not one of the possible choices"))
		if err != nil {
			applog.Sugar.Error(err)
			w.WriteHeader(http.StatusInternalServerError)
		}
		return
	}

	inviteCodeUUID, err := uuid.NewRandom()
	if err != nil {
		applog.Sugar.Error("error creating random uuid: ", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	inviteCode := inviteCodeUUID.String()
	inviteCode = strings.ReplaceAll(inviteCode, "-", "")
	applog.Sugar.Info(inviteCode)
	if forever == math.Inf(1) {
		_, err := database.PgDB.Exec(
			`INSERT INTO Invites (invite, chatroom, expires) VALUES ($1, $2, $3)`,
			inviteCode,
			roomName,
			"infinity",
		)
		if err != nil {
			applog.Sugar.Error("error inserting invite into invites table: ", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
	} else {
		_, err := database.PgDB.Exec(
			`INSERT INTO Invites (invite, chatroom, expires) VALUES ($1, $2, $3)`,
			inviteCode,
			roomName,
			inviteTimeLimit,
		)
		if err != nil {
			applog.Sugar.Error("error inserting invite into invites table: ", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
	}
	encodedInviteCode, err := json.Marshal(inviteCode)
	if err != nil {
		applog.Sugar.Error(err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusAccepted)
	_, err = w.Write(encodedInviteCode)
	if err != nil {
		applog.Sugar.Error("Error writing invite code in response: ", err)
	}

}

// add validation for this endpoint
func Join(writer http.ResponseWriter, req *http.Request) {
	err := req.ParseForm()
	if err != nil {
		applog.Sugar.Error("error parsing form: ", err)
		writer.WriteHeader(http.StatusInternalServerError)
	}

	inviteCode := req.FormValue("invite_code")

	row := database.PgDB.QueryRow(
		`SELECT chatroom, expires FROM Invites WHERE invite=$1`,
		inviteCode,
	)

	var chatroomName string
	// TODO: use this to figure out whether invite is past its expiration
	// before allowing user to use it
	var inviteExpiration string
	err = row.Scan(&chatroomName, &inviteExpiration)
	if err == sql.ErrNoRows {
		applog.Sugar.Error("invite not found: ", err)
		writer.WriteHeader(http.StatusNotFound)
		return
	} else if err != nil {
		applog.Sugar.Error("err scanning row: ", err)
		writer.WriteHeader(http.StatusInternalServerError)
		return
	}

	session, err := database.PgStore.Get(req, "session-name")
	if err != nil {
		applog.Sugar.Error("err getting session name: ", err)
		writer.WriteHeader(http.StatusInternalServerError)
		return
	}

	user := session.Values["username"].(string)
	client := ChatroomClient{Id: Clients[user].Id, Conn: Clients[user].Conn}

	Chatrooms[chatroomName].Clients = append(Chatrooms[chatroomName].Clients, &client)
	// todo add chatroom to user also

	// writer.WriteHeader(http.StatusInternalServerError)
	name, err := json.Marshal(chatroomName)
	if err != nil {
		applog.Sugar.Error(err)
	}

	writer.WriteHeader(http.StatusAccepted)
	_, err = writer.Write(name)
	if err != nil {
		applog.Sugar.Error("error writing chatroom name in response: ", err)
		return
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
