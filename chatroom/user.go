package chatroom

import "github.com/gorilla/websocket"

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
