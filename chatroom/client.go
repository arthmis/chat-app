package chatroom

import (
	"chat/auth"
	"encoding/json"
	"fmt"
	"log"
	"net/http"

	"github.com/gorilla/websocket"
)

var Clients = make(map[string]*User)

func OpenWsConnection(writer http.ResponseWriter, req *http.Request) {
	upgrader := websocket.Upgrader{}
	conn, err := upgrader.Upgrade(writer, req, nil)
	if err != nil {
		log.Println("upgrade error: ", err)
	}

	defer conn.Close()

	session, err := auth.Store.Get(req, "session-name")
	if err != nil {
		log.Println("err getting session name: ", err)
	}
	clientName := session.Values["username"].(string)

	// if err != nil {
	// 	log.Println("could not parse Message struct")
	// }
	// TODO: function that retrieves chatrooms user is part of and joins them
	chatUser := User{
		Conn:      conn,
		Id:        clientName,
		Chatrooms: make([]string, 0),
	}
	Clients[clientName] = &chatUser

	for {
		messageType, message, err := conn.ReadMessage()

		if err != nil {
			log.Println("connection closed: ", err)
			break
		}
		userMessage := UserMessage{}

		err = json.Unmarshal([]byte(message), &userMessage)
		if err != nil {
			log.Println("error json parsing user message: ", err)
			break
		}

		userMessage.User = clientName

		fmt.Println("message type: ", messageType)
		// spew.Dump(userMessage)
		fmt.Println()
		ChatroomChannels[userMessage.ChatroomName] <- userMessage
		if err != nil {
			log.Println("could not parse Message struct: ", err)
			break
		}
	}
	delete(Clients, clientName)
}
