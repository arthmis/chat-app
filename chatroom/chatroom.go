package chatroom

import (
	"encoding/json"
	"log"

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
type UserMessage struct {
	Message      string
	MessageType  string
	User         string
	ChatroomName string
}

type User struct {
	Conn      *websocket.Conn
	Id        string
	Chatrooms []string
}

type ChatroomUser struct {
	Name string
	Conn *websocket.Conn
}
type Chatroom struct {
	Id            string
	Users         []*User
	Messages      []UserMessage
	Channel       chan UserMessage
	ScyllaSession *gocqlx.Session
	Snowflake     *sonyflake.Sonyflake
}

func (room *Chatroom) Run() {
	for {
		newMessage := <-room.Channel
		room.Messages = append(room.Messages, newMessage)
		err := room.saveMessage(newMessage)
		if err != nil {
			log.Println(err)
		}
		bytes, err := json.Marshal(newMessage)
		if err != nil {
			log.Println(err)
		}
		for i := range room.Users {
			room.Users[i].Conn.WriteMessage(websocket.TextMessage, bytes)
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
