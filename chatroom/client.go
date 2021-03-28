package chatroom

import (
	"chat/database"
	"encoding/json"
	"log"
	"net/http"

	"github.com/davecgh/go-spew/spew"
	"github.com/gorilla/websocket"
)

var Clients = make(map[string]*User)

func OpenWsConnection(writer http.ResponseWriter, req *http.Request) {
	println("making connection")
	upgrader := websocket.Upgrader{}
	conn, err := upgrader.Upgrade(writer, req, nil)
	if err != nil {
		log.Println("upgrade error: ", err)
	}
	println("connection ugraded")

	defer conn.Close()

	session, err := database.PgStore.Get(req, "session-name")
	if err != nil {
		log.Println("err getting session name: ", err)
	}
	clientName := session.Values["username"].(string)

	if err != nil {
		log.Println("could not parse Message struct")
	}
	// TODO: function that retrieves chatrooms user is part of and joins them
	chatUser := User{
		Conn:      conn,
		Id:        clientName,
		Chatrooms: make([]string, 0),
	}
	Clients[clientName] = &chatUser

	for {
		// messageType, message, err := conn.ReadMessage()
		_, message, err := conn.ReadMessage()

		if err != nil {
			log.Println("connection closed: ", err)
			break
		}
		// spew.Dump(messageType, message)
		// println(message)
		// userMessage := UserMessage{}
		testMessage := TestMessage{}

		err = json.Unmarshal([]byte(message), &testMessage)
		if err != nil {
			log.Println("error json parsing user message: ", err)
			break
		}

		userMessage := UserMessage{}
		userMessage.User = chatUser.Id
		userMessage.Message = testMessage.Message
		userMessage.ChatroomName = testMessage.ChatroomName

		// fmt.Println("message type: ", messageType)
		// spew.Dump(userMessage)
		// fmt.Println()
		spew.Dump(ChatroomChannels)
		ChatroomChannels[userMessage.ChatroomName] <- userMessage
		// if err != nil {
		// 	log.Println("could not parse Message struct: ", err)
		// 	break
		// }
	}
	delete(Clients, clientName)
}
