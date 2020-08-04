package chatroom

import (
	"chat/auth"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/scylladb/gocqlx/table"
	"github.com/scylladb/gocqlx/v2"
	"github.com/sony/sonyflake"
)

var messageMetaData = table.Metadata{
	Name:    "messages",
	Columns: []string{"chatroom_name", "user_id", "content", "message_id"},
	PartKey: []string{"chatroom_name", "message_id"},
	SortKey: []string{"message_id"},
}

var chatroomTable = table.New(messageMetaData)

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
	Id    string
	Users []*User
	// get rid of messages field, not necessary
	Messages      []UserMessage
	Channel       chan UserMessage
	ScyllaSession *gocqlx.Session
	Snowflake     *sonyflake.Sonyflake
}

var chatrooms = make(map[string]*Chatroom)
var ChatroomChannels = make(map[string]chan UserMessage)
var Snowflake *sonyflake.Sonyflake
var ScyllaSession gocqlx.Session

func (room *Chatroom) Run() {
	for {
		newMessage := <-room.Channel
		room.Messages = append(room.Messages, newMessage)
		err := room.saveMessage(newMessage)
		if err != nil {
			log.Println("error saving message: ", err)
		}
		bytes, err := json.Marshal(newMessage)
		if err != nil {
			log.Println(err)
		}
		for i := range room.Users {
			err = room.Users[i].Conn.WriteMessage(websocket.TextMessage, bytes)
			if err != nil {
				log.Println("error writing message to user ws connection: ", err)
			}
		}
	}
}

func (room *Chatroom) saveMessage(chatMessage UserMessage) error {
	messageId, err := room.Snowflake.NextID()
	if err != nil {
		log.Println("Error generating sonyflake id: ", err)
		return err
	}
	log.Println("user: ", chatMessage.User)

	message := Message{
		ChatroomName: chatMessage.ChatroomName,
		UserId:       chatMessage.User,
		Content:      chatMessage.Message,
		MessageId:    messageId,
	}
	query := room.ScyllaSession.Query(chatroomTable.Insert()).BindStruct(message)
	err = query.ExecRelease()
	if err != nil {
		log.Println("Error inserting message in database: ", err)
		return err
	}

	return err
}

func NewChatroom() *Chatroom {
	room := new(Chatroom)
	room.Id = ""
	room.Users = make([]*User, 0)
	room.Messages = make([]UserMessage, 20)
	room.Channel = make(chan UserMessage)
	return room
}

// TODO think about tracking users and the rooms they are a part of
func Create(writer http.ResponseWriter, req *http.Request) {
	err := req.ParseForm()
	if err != nil {
		log.Println("error parsing form: ", err)
		writer.WriteHeader(http.StatusInternalServerError)
		return
	}

	session, err := auth.Store.Get(req, "session-name")
	if err != nil {
		log.Println("error getting session name: ", err)
		writer.WriteHeader(http.StatusInternalServerError)
		return
	}
	roomName := req.FormValue("chatroom_name")
	err = auth.Validate.Var(roomName, "lt=30,gt=3,alphanumeric")
	if err != nil {
		log.Println("chatroom name was not valid: ", err)
		writer.WriteHeader(http.StatusBadRequest)
		return
	}

	room := NewChatroom()
	room.Id = req.FormValue("chatroom_name")
	room.Snowflake = Snowflake
	room.ScyllaSession = &ScyllaSession
	Clients[session.Values["username"].(string)].Chatrooms = append(Clients[session.Values["username"].(string)].Chatrooms, room.Id)
	room.Users = append(room.Users, Clients[session.Values["username"].(string)])
	ChatroomChannels[room.Id] = room.Channel
	chatrooms[room.Id] = room

	go room.Run()

	chatroomNameEncoded, err := json.Marshal(room.Id)
	if err != nil {
		log.Println(err)
	}

	writer.WriteHeader(http.StatusAccepted)
	_, err = writer.Write(chatroomNameEncoded)
	if err != nil {
		log.Println("error writing chatroom name in response: ", err)
		return
	}
}

// TODO check to see if the user is actually a part of the chatroom
// before they are allowed to create an invite
func CreateInvite(w http.ResponseWriter, req *http.Request) {
	err := req.ParseForm()
	if err != nil {
		log.Println("error parsing form for create invite: ", err)
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
			log.Println(err)
			w.WriteHeader(http.StatusInternalServerError)
		}
		return
	}

	inviteCodeUUID, err := uuid.NewRandom()
	if err != nil {
		log.Println("error creating random uuid: ", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	inviteCode := inviteCodeUUID.String()
	inviteCode = strings.ReplaceAll(inviteCode, "-", "")
	fmt.Println(inviteCode)
	if forever == math.Inf(1) {
		_, err := auth.Db.Exec(
			`INSERT INTO Invites (invite, chatroom, expires) VALUES ($1, $2, $3)`,
			inviteCode,
			roomName,
			"infinity",
		)
		if err != nil {
			log.Println("error inserting invite into invites table: ", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
	} else {
		_, err := auth.Db.Exec(
			`INSERT INTO Invites (invite, chatroom, expires) VALUES ($1, $2, $3)`,
			inviteCode,
			roomName,
			inviteTimeLimit,
		)
		if err != nil {
			log.Println("error inserting invite into invites table: ", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
	}
	encodedInviteCode, err := json.Marshal(inviteCode)
	if err != nil {
		log.Println(err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusAccepted)
	_, err = w.Write(encodedInviteCode)
	if err != nil {
		log.Println("Error writing invite code in response: ", err)
	}

}

// add validation for this endpoint
func Join(writer http.ResponseWriter, req *http.Request) {
	err := req.ParseForm()
	if err != nil {
		log.Println("error parsing form: ", err)
		writer.WriteHeader(http.StatusInternalServerError)
	}

	inviteCode := req.FormValue("invite_code")

	row := auth.Db.QueryRow(
		`SELECT chatroom, expires FROM Invites WHERE invite=$1`,
		inviteCode,
	)

	var chatroomName string
	// TODO: use this to figure out whether invite is past its expiration
	// before allowing user to use it
	var inviteExpiration string
	err = row.Scan(&chatroomName, &inviteExpiration)
	if err == sql.ErrNoRows {
		log.Println("invite not found: ", err)
		writer.WriteHeader(http.StatusNotFound)
		return
	} else if err != nil {
		log.Println("err scanning row: ", err)
		writer.WriteHeader(http.StatusInternalServerError)
		return
	}

	session, err := auth.Store.Get(req, "session-name")
	if err != nil {
		log.Println("err getting session name: ", err)
		writer.WriteHeader(http.StatusInternalServerError)
		return
	}

	user := session.Values["username"].(string)

	chatrooms[chatroomName].Users = append(chatrooms[chatroomName].Users, Clients[user])
	// todo add chatroom to user also

	// writer.WriteHeader(http.StatusInternalServerError)
	name, err := json.Marshal(chatroomName)
	if err != nil {
		log.Println(err)
	}

	writer.WriteHeader(http.StatusAccepted)
	_, err = writer.Write(name)
	if err != nil {
		log.Println("error writing chatroom name in response: ", err)
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
