package app

import (
	"context"
	"encoding/json"
	"net/http"

	"go.opentelemetry.io/otel"
	"nhooyr.io/websocket"
)

func (app App) OpenWsConnection(writer http.ResponseWriter, req *http.Request) {
	ctx, openWsSpan := otel.Tracer("").Start(req.Context(), "OpenWsConnection")
	Sugar.Info("making ws connection")
	// right now this doesn't handle dealing with the request origin
	conn, err := websocket.Accept(writer, req, nil)
	if err != nil {
		Sugar.Error("upgrade error: ", err)
		writer.WriteHeader(http.StatusInternalServerError)
		return
	}
	defer conn.Close(websocket.StatusInternalError, "")

	openWsSpan.AddEvent("Connection upgraded to WebSocket")
	Sugar.Info("connection ugraded to ws")

	session, err := app.PgStore.Get(req, "session-name")
	if err != nil {
		openWsSpan.RecordError(err)
		Sugar.Error("err getting session name: ", err)
	}
	clientName := session.Values["username"].(string)

	if err != nil {
		openWsSpan.RecordError(err)
		Sugar.Error("could not parse Message struct")
	}
	// TODO: function that retrieves chatrooms user is part of and joins them
	chatUser := User{
		Conn:      conn,
		Id:        clientName,
		Chatrooms: make([]string, 0),
	}
	app.Clients[clientName] = &chatUser
	stmt := "SELECT chatroom FROM users WHERE user = ?;"
	values := []string{"user"}
	query := app.ScyllaDb.Query(stmt, values)
	query.Bind(clientName)

	var chatrooms []string
	err = query.SelectRelease(&chatrooms)
	if err != nil {
		// TODO: Figure out why i'm doing this
		if err.Error() != "" {
			openWsSpan.RecordError(err)
			Sugar.Error("Error finding all chatrooms for user: ", err)
			writer.WriteHeader(http.StatusInternalServerError)
			return
		}
	}
	for _, name := range chatrooms {
		app.Chatrooms[name].addUser(conn, clientName)
	}

	openWsSpan.End()
	tracer := otel.Tracer("")
	for {
		// messageType, message, err := conn.ReadMessage()
		_, message, err := conn.Read(ctx)
		// ctx, span := tracer.Start(ctx, clientName)
		// maybe have unique ID for this user and their connection
		// or maybe use the chatroom derived from the message
		// as the name of the tracer
		ctx, span := tracer.Start(context.Background(), clientName)

		if err != nil {
			span.RecordError(err)
			Sugar.Error("connection closed: ", err)
			break
		}
		// spew.Dump(messageType, message)
		// println(message)
		// userMessage := UserMessage{}
		testMessage := TestMessage{}

		err = json.Unmarshal([]byte(message), &testMessage)
		if err != nil {
			span.RecordError(err)
			Sugar.Error("error json parsing user message: ", err)
			break
		}

		userMessage := IncomingMessage{}
		userMessage.User = chatUser.Id
		userMessage.Message = testMessage.Message
		userMessage.ChatroomName = testMessage.ChatroomName

		// fmt.Println("message type: ", messageType)
		// spew.Dump(userMessage)
		// fmt.Println()
		// spew.Dump(ChatroomChannels)
		// ChatroomChannels[userMessage.ChatroomName] <- userMessage
		app.ChatroomChannels[userMessage.ChatroomName] <- MessageWithCtx{Message: userMessage, Ctx: ctx}
		// if err != nil {
		// 	app.Sugar.Error("could not parse Message struct: ", err)
		// 	break
		// }
		span.End()
	}
	delete(app.Clients, clientName)
}
