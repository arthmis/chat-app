package app

import (
	"database/sql"
	"math"
	"math/rand"
	"time"
)

const (
	Never = 0
	Week  = 7
	Day   = 1
)

type InviteTimeLimit struct {
	limit int32
}

type Invitations struct {
	pg *sql.DB
	// will store some information on client to interface with url generation service
	// when it gets to that point
	// rigth now  this won't hold anything
	// it will generate the invites on demand and store them in a sql database
}

// this will return a new unique invite string
// it will return an error if there was an issue creating the invite
func (self Invitations) createInvite(roomName string, timeLimit InviteTimeLimit) (string, error) {
	inviteCode := generateInvite()

	now := time.Now()
	inviteTimeLimit := time.Time{}
	forever := 0.0
	switch timeLimit.limit {
	case Day:
		inviteTimeLimit = now.Add(time.Hour * 24)
	case Week:
		inviteTimeLimit = now.Add(time.Hour * 24 * 7)
	case Never:
		forever = math.Inf(1)
	}

	if forever == math.Inf(1) {
		_, err := self.pg.Exec(
			`INSERT INTO Invites (invite, chatroom, expires) VALUES ($1, $2, $3)`,
			inviteCode,
			roomName,
			"infinity",
		)
		if err != nil {
			Sugar.Error("error inserting invite into invites table: ", err)
			return "", err
		}
	} else {
		_, err := self.pg.Exec(
			`INSERT INTO Invites (invite, chatroom, expires) VALUES ($1, $2, $3)`,
			inviteCode,
			roomName,
			inviteTimeLimit,
		)
		if err != nil {
			Sugar.Error("error inserting invite into invites table: ", err)
			return "", err
		}
	}

	return inviteCode, nil
}

// this will check if an invite exists and if it does
// which room it belongs to
func (self Invitations) getChatroom(inviteCode string) (string, error) {

	row := self.pg.QueryRow(
		`SELECT chatroom, expires FROM Invites WHERE invite=$1`,
		inviteCode,
	)

	var chatroomName string
	// TODO: use this to figure out whether invite is past its expiration
	// before allowing user to use it
	var inviteExpiration string
	err := row.Scan(&chatroomName, &inviteExpiration)
	if err == sql.ErrNoRows {
		Sugar.Error("invite not found: ", err)
		return "", err
	} else if err != nil {
		Sugar.Error("err scanning row: ", err)
		return "", err
	}

	return chatroomName, nil
}

var letterRunes = []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789")

// I will have to find a truly random sampler, but this should work for now
func generateInvite() string {
	b := make([]rune, 8)
	for i := range b {
		idx := rand.Intn(len(letterRunes))
		b[i] = letterRunes[idx]
	}
	return string(b)
}
