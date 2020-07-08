package main

import (
	"log"
	"time"

	"github.com/davecgh/go-spew/spew"
	"github.com/gocql/gocql"
	"github.com/scylladb/gocqlx/table"
	"github.com/scylladb/gocqlx/v2"
)

var messageMetaData = table.Metadata{
	Name:    "chatroom",
	Columns: []string{"user_id", "message_text", "datetime"},
	PartKey: []string{"user_id"},
	SortKey: []string{"datetime"},
}
var chatroomTable = table.New(messageMetaData)

type Message struct {
	UserId      string
	MessageText string
	Datetime    time.Time
}

func main() {

	// this should be the address the node is reachable on
	// for development it is probably the host ip address `127.0.0.1`
	cluster := gocql.NewCluster("127.0.0.1")
	cluster.Keyspace = "chatserver"
	session, err := gocqlx.WrapSession(cluster.CreateSession())
	if err != nil {
		log.Fatal(err)
	}

	defer session.Close()

	err = session.ExecStmt(
		`CREATE KEYSPACE IF NOT EXISTS chatserver WITH replication = {
			'class': 'SimpleStrategy',
			'replication_factor': 1
		}`,
	)

	if err != nil {
		log.Fatal("create keyspace:", err)
	}

	err = session.ExecStmt(
		`CREATE TABLE IF NOT EXISTS chatroom (
			user_id TEXT PRIMARY KEY,
			message_text TEXT,
			datetime TIMESTAMP
		)`,
	)
	if err != nil {
		log.Fatal("Create chatrooms error:", err)
	}

	userMessage := Message{
		UserId:      "art",
		MessageText: "Hello World",
		Datetime:    time.Now(),
	}
	query := session.Query(chatroomTable.Insert()).BindStruct(userMessage)
	if err := query.ExecRelease(); err != nil {
		log.Fatal("error inserting message:", err)
	}

	storedMessage := Message{UserId: "art", MessageText: "", Datetime: time.Now()}
	// query = session.Query(stmt, names).BindStruct(storedMessage)
	// query = session.Query(
	// 	chatroomTable.SelectBuilder().AllowFiltering().Limit(1).Where(qb.ContainsNamed("user_id", "art")).ToCql(),
	// )
	query = session.Query(`SELECT * FROM chatroom WHERE user_id = ? LIMIT 1`, []string{"art"})
	if err := query.Iter().StructOnly().Get(&storedMessage); err != nil {
		spew.Dump(err)
		return
	}
	spew.Dump(storedMessage)

}
