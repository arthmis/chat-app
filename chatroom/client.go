package chatroom

import (
	"chat/app"
	"chat/database"
	"encoding/json"
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
		app.Sugar.Error("upgrade error: ", err)
	}
	println("connection ugraded")

	defer conn.Close()

	session, err := database.PgStore.Get(req, "session-name")
	if err != nil {
		app.Sugar.Error("err getting session name: ", err)
	}
	clientName := session.Values["username"].(string)

	if err != nil {
		app.Sugar.Error("could not parse Message struct")
	}
	// TODO: function that retrieves chatrooms user is part of and joins them
	chatUser := User{
		Conn:      conn,
		Id:        clientName,
		Chatrooms: make([]string, 0),
	}
	Clients[clientName] = &chatUser
	stmt := "SELECT chatroom FROM users WHERE user = ?;"
	values := []string{"user"}
	query := ScyllaSession.Query(stmt, values)
	query.Bind(clientName)

	var chatrooms []string
	err = query.SelectRelease(&chatrooms)
	if err != nil {
		// TODO: Figure out why i'm doing this
		if err.Error() != "" {
			app.Sugar.Error("Error finding all chatrooms for user: ", err)
			writer.WriteHeader(http.StatusInternalServerError)
			return
		}
	}
	for _, name := range chatrooms {
		Chatrooms[name].addUser(conn, clientName)
	}

	for {
		// messageType, message, err := conn.ReadMessage()
		_, message, err := conn.ReadMessage()

		if err != nil {
			app.Sugar.Error("connection closed: ", err)
			break
		}
		// spew.Dump(messageType, message)
		// println(message)
		// userMessage := UserMessage{}
		testMessage := TestMessage{}

		err = json.Unmarshal([]byte(message), &testMessage)
		if err != nil {
			app.Sugar.Error("error json parsing user message: ", err)
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
		// 	app.Sugar.Error("could not parse Message struct: ", err)
		// 	break
		// }
	}
	delete(Clients, clientName)
}
