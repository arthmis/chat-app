package chatroom

import (
	"encoding/json"
	"fmt"
	"log"

	"github.com/gorilla/websocket"
)

type Message struct {
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
	Id       string
	Users    []*User
	Messages []Message
	Channel  chan Message
}

func (room *Chatroom) Run() {
	for {
		newMessage := <-room.Channel
		fmt.Printf("%v", newMessage)
		room.Messages = append(room.Messages, newMessage)
		bytes, err := json.Marshal(newMessage)
		if err != nil {
			log.Println(err)
		}
		for i := range room.Users {
			room.Users[i].Conn.WriteMessage(websocket.TextMessage, bytes)
		}
	}
}

func NewChatroom() *Chatroom {
	room := new(Chatroom)
	room.Id = ""
	room.Users = make([]*User, 0)
	room.Messages = make([]Message, 20)
	room.Channel = make(chan Message)
	return room
}
