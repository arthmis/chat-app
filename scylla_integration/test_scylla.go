package main

import (
	"log"
	"math/rand"
	"time"

	"github.com/gocql/gocql"
	"github.com/scylladb/gocqlx/table"
	"github.com/scylladb/gocqlx/v2"
	"github.com/sony/sonyflake"
)

var messageMetaData = table.Metadata{
	Name:    "messages",
	Columns: []string{"chatroom_id", "chatroom_name", "user_id", "content", "message_id"},
	PartKey: []string{"chatroom_id", "message_id"},
	SortKey: []string{"datetime"},
}
var chatroomTable = table.New(messageMetaData)

type Message struct {
	ChatroomId   uint64
	ChatroomName string
	UserId       string
	Content      string
	MessageId    uint64 `db:"message_id"`
}

// func init() {
// 	rand.Seed(time.Now().UnixNano())
// }

var letterRunes = []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")

func RandStringRunes(n int) string {
	b := make([]rune, n)
	for i := range b {
		b[i] = letterRunes[rand.Intn(len(letterRunes))]
	}
	return string(b)
}

func main() {
	// node, err := snowflake.NewNode(1)
	// if err != nil {
	// 	log.Fatal(err)
	// }
	// id := node.Generate()
	// log.Println("id: ", id)

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
		`CREATE TABLE IF NOT EXISTS messages(
			chatroom_name TEXT,
			user_id TEXT,
			content TEXT,
			message_id bigint,
			PRIMARY KEY (chatroom_name, message_id)
		) WITH CLUSTERING ORDER BY (message_id DESC)`,
	)
	if err != nil {
		log.Fatal("Create chatrooms error:", err)
	}
	node := sonyflake.NewSonyflake(
		sonyflake.Settings{
			StartTime: time.Unix(0, 0),
		},
	)

	for i := 0; i < 10; i += 1 {

		id, err := node.NextID()
		if err != nil {
			log.Fatal(err)
		}

		userMessage := Message{
			ChatroomId:   0,
			ChatroomName: "First Room",
			UserId:       "art",
			Content:      RandStringRunes(10),
			MessageId:    id,
		}
		query := session.Query(chatroomTable.Insert()).BindStruct(userMessage)
		if err := query.ExecRelease(); err != nil {
			log.Fatal("error inserting message:", err)
		}
	}
	for i := 0; i < 10; i += 1 {

		id, err := node.NextID()
		if err != nil {
			log.Fatal(err)
		}

		userMessage := Message{
			ChatroomId:   1,
			ChatroomName: "Second Room",
			UserId:       "art",
			Content:      RandStringRunes(10),
			MessageId:    id,
		}
		query := session.Query(chatroomTable.Insert()).BindStruct(userMessage)
		if err := query.ExecRelease(); err != nil {
			log.Fatal("error inserting message:", err)
		}
	}

	// storedMessage := Message{ChatroomId: 0, ChatroomName: "", UserId: "", Content: "", Datetime: time.Now()}
	// // stmt, names := chatroomTable.SelectBuilder().AllowFiltering().Where(qb.ContainsNamed("user_id", "art")).Limit(1).ToCql()
	// // spew.Dump(stmt)
	// // spew.Dump(names)
	// // query = session.Query(`SELECT * FROM chatroom WHERE user_id=? AND message_text=? LIMIT 1 ALLOW FILTERING `, nil)
	// query := session.Query(
	// 	`SELECT * FROM messages WHERE content=? LIMIT 1 ALLOW FILTERING`,
	// 	nil,
	// )
	// // must Bind values in order to use in statement
	// query.Bind("Hello World")
	// err = query.GetRelease(&storedMessage)
	// if err != nil {
	// 	spew.Dump(err)
	// 	return
	// }
	// spew.Dump(storedMessage)

}
